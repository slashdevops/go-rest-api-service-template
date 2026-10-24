//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
)

var idpsAccountsEndpoint = newAPIEndpoint(http.MethodGet, "/auth/idps/{idp_id}/identities")

// TestAProvidersAccountsAreListed: the 409 of a provider's delete says
// "delete those accounts or make them local accounts first", and no route
// said which accounts. GET /auth/idps/{idp_id}/identities lists the accounts
// linked to a provider, each with when it was linked and only_way_in, the
// delete's own rule: true for exactly the accounts the 409 is about. Removing
// what it names lets the delete through.
func TestAProvidersAccountsAreListed(t *testing.T) {
	ctx := t.Context()

	administrator := getAdminUserTokens(t)
	t.Cleanup(func() { deleteUserByIDFromDB(t, administrator.UserID) })

	admin := map[string]string{"Authorization": "Bearer " + administrator.AccessToken}

	provider := createOIDCIDP(t, administrator.AccessToken, newFakeProvider(t), false)
	elsewhere := createOIDCIDP(t, administrator.AccessToken, newFakeProvider(t), false)

	// An account with identities at the providers named, and a password or not.
	account := func(t *testing.T, local bool, idps ...string) uuid.UUID {
		t.Helper()

		firstName, lastName, email := generateUserData(t)
		id := createUserInDB(t, firstName, lastName, email, generatePassword(t))
		t.Cleanup(func() { deleteUserByIDFromDB(t, id) })

		for _, idp := range idps {
			_, err := testDBPool.Exec(ctx, `INSERT INTO users_identities (users_id, idps_id, subject, email) VALUES ($1, $2, $3, $4)`,
				id, idp, "sub-"+uuid.NewV7().String(), email)
			require.NoError(t, err)
		}

		_, err := testDBPool.Exec(ctx, `UPDATE users SET local_account = $2 WHERE id = $1`, id, local)
		require.NoError(t, err)

		return id
	}

	onlyHere := account(t, false, provider)
	alsoElsewhere := account(t, false, provider, elsewhere)
	withAPassword := account(t, true, provider)
	account(t, false, elsewhere) // linked to the other provider only: not listed

	list := func(t *testing.T, idp string, query map[string]string) (int, payload.ListIDPAccountsResponse, string) {
		t.Helper()

		endpoint := idpsAccountsEndpoint.Clone().RewriteSlugs(idp)
		for k, v := range query {
			endpoint.SetQueryParam(k, v)
		}

		response, err := sendHTTPRequest(t, context.WithoutCancel(ctx), endpoint, nil, admin)
		require.NoError(t, err)

		text := readResponseBody(t, response)
		response.Body.Close()

		var page payload.ListIDPAccountsResponse
		if response.StatusCode == http.StatusOK {
			require.NoError(t, json.Unmarshal([]byte(text), &page), text)
		}

		return response.StatusCode, page, text
	}

	t.Run("the_accounts_and_which_are_the_409s", func(t *testing.T) {
		status, page, text := list(t, provider, nil)
		require.Equal(t, http.StatusOK, status, text)

		got := map[uuid.UUID]bool{}
		for _, item := range page.Items {
			got[item.ID] = item.OnlyWayIn
			assert.False(t, item.LinkedAt.IsZero(), "linked_at of %s", item.ID)
		}

		assert.Equal(t, map[uuid.UUID]bool{onlyHere: true, alsoElsewhere: false, withAPassword: false}, got)
		assert.NotContains(t, text, "subject", "a subject is not shown")
	})

	t.Run("filtered_and_narrowed_by_the_accounts_fields", func(t *testing.T) {
		status, page, text := list(t, provider, map[string]string{"filter": "local_account = false", "fields": "id,email"})
		require.Equal(t, http.StatusOK, status, text)

		ids := []uuid.UUID{}
		for _, item := range page.Items {
			ids = append(ids, item.ID)
			assert.Empty(t, item.FirstName, "a field that was not asked for")
			assert.False(t, item.LinkedAt.IsZero(), "linked_at is always returned")
		}

		assert.ElementsMatch(t, []uuid.UUID{onlyHere, alsoElsewhere}, ids)
	})

	t.Run("paged", func(t *testing.T) {
		seen := map[uuid.UUID]bool{}

		status, page, text := list(t, provider, map[string]string{"limit": "2"})
		require.Equal(t, http.StatusOK, status, text)
		require.Len(t, page.Items, 2, text)
		require.NotEmpty(t, page.Paginator.NextToken, text)

		for _, item := range page.Items {
			seen[item.ID] = true
		}

		status, page, text = list(t, provider, map[string]string{"limit": "2", "next_token": page.Paginator.NextToken})
		require.Equal(t, http.StatusOK, status, text)
		require.Len(t, page.Items, 1, text)

		seen[page.Items[0].ID] = true
		assert.Equal(t, map[uuid.UUID]bool{onlyHere: true, alsoElsewhere: true, withAPassword: true}, seen)
	})

	t.Run("a_provider_that_is_not_there_is_a_404", func(t *testing.T) {
		status, _, text := list(t, uuid.NewV7().String(), nil)
		assert.Equal(t, http.StatusNotFound, status, text)
	})

	t.Run("a_provider_id_that_is_no_id_is_a_400", func(t *testing.T) {
		status, _, text := list(t, "not-an-id", nil)
		assert.Equal(t, http.StatusBadRequest, status, text)
	})

	// The 409's remedy: what the list names, and then the delete.
	t.Run("removing_what_it_names_lets_the_delete_through", func(t *testing.T) {
		del := func() (int, string) {
			response, err := sendHTTPRequest(t, context.WithoutCancel(ctx), idpsDeleteEndpoint.Clone().RewriteSlugs(provider), nil, admin)
			require.NoError(t, err)

			defer response.Body.Close()

			return response.StatusCode, readResponseBody(t, response)
		}

		status, text := del()
		require.Equal(t, http.StatusConflict, status, text)

		_, page, _ := list(t, provider, nil)
		for _, item := range page.Items {
			if item.OnlyWayIn {
				_, err := testDBPool.Exec(context.Background(), `UPDATE users SET local_account = TRUE WHERE id = $1`, item.ID)
				require.NoError(t, err)
			}
		}

		status, text = del()
		assert.Equal(t, http.StatusOK, status, text)
	})
}
