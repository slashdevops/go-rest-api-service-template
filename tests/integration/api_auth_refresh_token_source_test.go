//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// refreshWith calls /auth/refresh with an explicit header token and an explicit
// body token, so a test can make the two disagree.
func refreshWith(t *testing.T, headerToken string, body map[string]any) (*http.Response, string) {
	t.Helper()

	response, err := sendHTTPRequest(t, t.Context(), authRefreshEndpoint, body,
		map[string]string{"Authorization": "Bearer " + headerToken})
	require.NoError(t, err, "Error sending refresh request")

	return response, readResponseBody(t, response)
}

// TestAuthRefreshUsesTheValidatedToken covers A-10: the refresh endpoint had two
// sources of truth for which token it was acting on, and used the one that had
// not been validated.
//
// CheckRefreshToken verifies the token in the Authorization header and puts its
// claims on the context. The handler then decoded a SECOND token out of the
// request body and refreshed that one instead. Nothing checked that the two
// were the same token, so the token a request was authorised with and the token
// it spent could differ.
func TestAuthRefreshUsesTheValidatedToken(t *testing.T) {
	t.Run("a_body_token_that_disagrees_with_the_header_is_refused", func(t *testing.T) {
		t.Parallel()

		_, caller := loginAs(t)
		_, other := loginAs(t)

		// Authorised as one account, asking to spend another account's token.
		// Answering this by quietly picking one of them is how the two got out
		// of step in the first place.
		response, body := refreshWith(t, caller.RefreshToken, map[string]any{
			"refresh_token": other.RefreshToken,
		})
		require.NoError(t, response.Body.Close())

		assert.Equal(t, http.StatusBadRequest, response.StatusCode,
			"a request authorised with one token and spending another must be refused. Got %d. Message: %s",
			response.StatusCode, body)

		// And neither session may have been touched by the attempt.
		for name, token := range map[string]string{
			"caller": caller.RefreshToken,
			"other":  other.RefreshToken,
		} {
			after, afterBody := refresh(t, token)
			assert.Equal(t, http.StatusOK, after.StatusCode,
				"the %s session must survive a refused refresh. Got %d. Message: %s", name, after.StatusCode, afterBody)
			require.NoError(t, after.Body.Close())
		}
	})

	t.Run("the_header_token_alone_is_enough", func(t *testing.T) {
		t.Parallel()

		_, login := loginAs(t)

		// The body is optional: the token that gets spent is the one the
		// middleware verified.
		response, body := refreshWith(t, login.RefreshToken, nil)
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode,
			"a refresh with no body must succeed. Got %d. Message: %s", response.StatusCode, body)
	})

	t.Run("header_and_body_carrying_the_same_token_still_works", func(t *testing.T) {
		t.Parallel()

		// The shape every existing client sends, including the frontend.
		// Breaking it would be a two-repo outage.
		_, login := loginAs(t)

		response, body := refreshWith(t, login.RefreshToken, map[string]any{
			"refresh_token": login.RefreshToken,
		})
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode,
			"the header-and-body shape must keep working. Got %d. Message: %s", response.StatusCode, body)
	})
}
