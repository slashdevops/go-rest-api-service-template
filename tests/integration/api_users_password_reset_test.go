//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"testing"
	"uuid"

	"github.com/stretchr/testify/require"
)

var usersPasswordResetEndpoint = newAPIEndpoint(http.MethodPost, "/users/{user_id}/password/reset")

// TestUserPasswordResetByAdmin covers the route that replaced the password
// field on PUT /users/{id}: an administrator can only ask for a reset link to
// be emailed, never set the password.
func TestUserPasswordResetByAdmin(t *testing.T) {
	ctx := t.Context()
	admin := getAdminUserTokens(t)
	auth := map[string]string{"Authorization": "Bearer " + admin.AccessToken}

	email := fmt.Sprintf("reset-target-%s@example.com", uuid.NewV7().String()[:8])
	userID := createUserInDB(t, "Reset", "Target", email, "ThisIsApassw0rd.,")
	enableUserByEmailFromDB(t, email)
	t.Cleanup(func() { deleteUserByIDFromDB(t, userID) })

	t.Run("an_existing_account_gets_202", func(t *testing.T) {
		resp, err := sendHTTPRequest(t, ctx, usersPasswordResetEndpoint.RewriteSlugs(userID.String()), nil, auth)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusAccepted, resp.StatusCode, readResponseBody(t, resp))
	})

	t.Run("a_missing_account_is_404", func(t *testing.T) {
		resp, err := sendHTTPRequest(t, ctx, usersPasswordResetEndpoint.RewriteSlugs(uuid.NewV7().String()), nil, auth)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusNotFound, resp.StatusCode, readResponseBody(t, resp))
	})

	t.Run("a_malformed_id_is_400", func(t *testing.T) {
		resp, err := sendHTTPRequest(t, ctx, usersPasswordResetEndpoint.RewriteSlugs("not-a-uuid"), nil, auth)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, readResponseBody(t, resp))
	})
}
