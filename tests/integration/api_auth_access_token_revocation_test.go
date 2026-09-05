//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// meWith calls the cheapest authenticated endpoint with the given token.
func meWith(t *testing.T, accessToken string) (*http.Response, string) {
	t.Helper()

	response, err := sendHTTPRequest(t, t.Context(), meGetEndpoint, nil,
		map[string]string{"Authorization": "Bearer " + accessToken})
	require.NoError(t, err)

	return response, readResponseBody(t, response)
}

// adminLogin creates an admin and logs in. /me is used as the probe because it
// is the cheapest endpoint that proves a token is being accepted, and the
// AuthenticatedUser role cannot reach it. Which role is used is irrelevant to
// what these tests assert.
func adminLogin(t *testing.T) (accessToken, refreshToken string) {
	t.Helper()

	login := getAdminUserTokens(t)
	t.Cleanup(func() { deleteUserByIDFromDB(t, login.UserID) })

	return login.AccessToken, login.RefreshToken
}

// TestLogoutRevokesTheAccessToken covers the residue #354 deliberately left.
//
// Logout revoked the refresh token, so the session could not be extended — but
// the access token it was called with kept working for the rest of its
// lifetime. Reproduced against the running API before this change:
//
//	before logout, GET /me           -> 200
//	DELETE /auth/logout              -> 200
//	AFTER logout, same access token  -> 200 with the full profile
func TestLogoutRevokesTheAccessToken(t *testing.T) {
	t.Parallel()

	t.Run("the_access_token_stops_working", func(t *testing.T) {
		t.Parallel()

		accessToken, refreshToken := adminLogin(t)

		before, _ := meWith(t, accessToken)
		require.Equal(t, http.StatusOK, before.StatusCode,
			"the access token must work before logout, or this test proves nothing")
		require.NoError(t, before.Body.Close())

		out := logout(t, accessToken, map[string]any{"refresh_token": refreshToken})
		require.Equal(t, http.StatusOK, out.StatusCode,
			"Expected logout to succeed. Got %d. Message: %s", out.StatusCode, readResponseBody(t, out))
		require.NoError(t, out.Body.Close())

		after, body := meWith(t, accessToken)
		defer after.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, after.StatusCode,
			"a logged-out access token must not still authenticate. Got %d. Message: %s", after.StatusCode, body)
	})

	t.Run("the_rejection_carries_a_code_a_client_can_branch_on", func(t *testing.T) {
		t.Parallel()

		accessToken, refreshToken := adminLogin(t)

		out := logout(t, accessToken, map[string]any{"refresh_token": refreshToken})
		require.Equal(t, http.StatusOK, out.StatusCode)
		require.NoError(t, out.Body.Close())

		after, body := meWith(t, accessToken)
		defer after.Body.Close()

		require.Equal(t, http.StatusUnauthorized, after.StatusCode)

		// The whole reason the code field exists. A client that cannot tell
		// "expired, refresh and retry" from "revoked, stop" will refresh-and-
		// retry into a refresh token that was revoked in the same breath,
		// burning two more requests to reach the same place. It must not be
		// asked to match on prose.
		assert.Contains(t, body, `"code":"`+domain.CodeTokenRevoked+`"`,
			"the rejection must carry the machine-readable code. Body: %s", body)
	})

	t.Run("one_session_ending_does_not_end_another", func(t *testing.T) {
		t.Parallel()

		victimToken, _ := adminLogin(t)
		otherToken, otherRefresh := adminLogin(t)

		out := logout(t, otherToken, map[string]any{"refresh_token": otherRefresh})
		require.Equal(t, http.StatusOK, out.StatusCode)
		require.NoError(t, out.Body.Close())

		// A denylist keyed on the token, not on the user. If this ever fails,
		// the revocation has become "sign out everywhere" without anybody
		// deciding that it should be.
		response, body := meWith(t, victimToken)
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode,
			"another session's logout must not end this one. Got %d. Message: %s", response.StatusCode, body)
	})

	t.Run("logging_out_twice_still_succeeds", func(t *testing.T) {
		t.Parallel()

		accessToken, refreshToken := adminLogin(t)

		// The endpoint that revokes must not be gated on the token not being
		// revoked, or the second of two tabs logging out at once is refused.
		for i := range 2 {
			out := logout(t, accessToken, map[string]any{"refresh_token": refreshToken})
			assert.Equal(t, http.StatusOK, out.StatusCode,
				"logout %d should succeed even though the first one revoked this very token. Got %d. Message: %s",
				i+1, out.StatusCode, readResponseBody(t, out))
			require.NoError(t, out.Body.Close())
		}
	})
}
