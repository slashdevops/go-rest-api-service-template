//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
)

// loginAs creates an enabled user with the AuthenticatedUser role and logs in.
func loginAs(t *testing.T) (email string, login payload.LoginUserResponse) {
	t.Helper()

	firstName, lastName, email := generateUserData(t)
	password := generatePassword(t)

	userID := createUserInDB(t, firstName, lastName, email, password)
	t.Cleanup(func() { deleteUserByEmailFromDB(t, email) })

	enableUserByEmailFromDB(t, email)
	assignRoleToUserInDB(t, userID, "AuthenticatedUser")

	response, err := sendHTTPRequest(t, t.Context(), authLoginEndpoint, map[string]any{
		"email":    email,
		"password": password,
	})
	require.NoError(t, err, "Error sending login request")
	defer response.Body.Close()

	require.Equal(t, http.StatusOK, response.StatusCode,
		"Expected login to succeed. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

	login, err = parserResponseBody[payload.LoginUserResponse](t, response)
	require.NoError(t, err, "Error parsing login response")

	return email, login
}

func refresh(t *testing.T, refreshToken string) (*http.Response, string) {
	t.Helper()

	response, err := sendHTTPRequest(t, t.Context(), authRefreshEndpoint,
		map[string]any{"refresh_token": refreshToken},
		map[string]string{"Authorization": "Bearer " + refreshToken})
	require.NoError(t, err, "Error sending refresh request")

	return response, readResponseBody(t, response)
}

func logout(t *testing.T, accessToken string, body map[string]any) *http.Response {
	t.Helper()

	response, err := sendHTTPRequest(t, t.Context(), authLogoutEndpoint, body,
		map[string]string{"Authorization": "Bearer " + accessToken})
	require.NoError(t, err, "Error sending logout request")

	return response
}

// TestAuthLogoutRevokesTheRefreshToken covers the finding that logout said it
// had ended a session and had not.
//
// LogoutUser invalidated two cache entries and returned 200 while both tokens
// kept working; there was no revocation anywhere in the service, and the
// refresh endpoint's own swagger documented a "revoked" 401 that could not
// happen. Reproduced against the running API before the fix:
//
//	before logout, GET /me            -> 200
//	DELETE /auth/logout               -> 200
//	AFTER logout, same access token   -> 200
//	AFTER logout, refresh still works -> 200
func TestAuthLogoutRevokesTheRefreshToken(t *testing.T) {
	t.Run("the_refresh_token_stops_working", func(t *testing.T) {
		t.Parallel()

		_, login := loginAs(t)

		// Without this the test would pass against a refresh endpoint that was
		// broken outright.
		before, body := refresh(t, login.RefreshToken)
		require.Equal(t, http.StatusOK, before.StatusCode,
			"Expected refresh to work before logout. Got %d. Message: %s", before.StatusCode, body)
		require.NoError(t, before.Body.Close())

		out := logout(t, login.AccessToken, map[string]any{"refresh_token": login.RefreshToken})
		require.Equal(t, http.StatusOK, out.StatusCode,
			"Expected logout to succeed. Got %d. Message: %s", out.StatusCode, readResponseBody(t, out))
		require.NoError(t, out.Body.Close())

		after, afterBody := refresh(t, login.RefreshToken)
		defer after.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, after.StatusCode,
			"A logged-out refresh token must not mint new access tokens. Got %d. Message: %s", after.StatusCode, afterBody)
		assert.Contains(t, afterBody, "revoked", "the rejection should say why")
	})

	t.Run("logging_out_twice_is_not_an_error", func(t *testing.T) {
		t.Parallel()

		_, login := loginAs(t)

		for i := range 2 {
			out := logout(t, login.AccessToken, map[string]any{"refresh_token": login.RefreshToken})
			assert.Equal(t, http.StatusOK, out.StatusCode,
				"logout %d should succeed; two tabs logging out at once is normal. Got %d. Message: %s",
				i+1, out.StatusCode, readResponseBody(t, out))
			require.NoError(t, out.Body.Close())
		}
	})

	t.Run("logout_without_a_refresh_token_still_succeeds", func(t *testing.T) {
		t.Parallel()

		_, login := loginAs(t)

		// The endpoint has always been callable with only an access token, and
		// refusing now would break every existing caller. It cannot end the
		// session, which the service logs — but it must not fail.
		out := logout(t, login.AccessToken, nil)
		defer out.Body.Close()

		assert.Equal(t, http.StatusOK, out.StatusCode,
			"logout with no body must still succeed. Got %d. Message: %s", out.StatusCode, readResponseBody(t, out))
	})

	t.Run("an_unreadable_refresh_token_is_refused_not_reported_as_success", func(t *testing.T) {
		t.Parallel()

		_, login := loginAs(t)

		// Answering 200 here would repeat the bug this endpoint was fixed for:
		// the caller's real session is still live, and they have been told it
		// ended. An expired token is different — that session really is over —
		// but a token we cannot read tells us nothing.
		out := logout(t, login.AccessToken, map[string]any{"refresh_token": "not.a.token"})
		outBody := readResponseBody(t, out)
		require.NoError(t, out.Body.Close())

		assert.Equal(t, http.StatusBadRequest, out.StatusCode,
			"an unreadable refresh token must be refused. Got %d. Message: %s", out.StatusCode, outBody)

		// And the real session must be untouched — refusing is not the same as
		// revoking something at random.
		after, afterBody := refresh(t, login.RefreshToken)
		defer after.Body.Close()

		assert.Equal(t, http.StatusOK, after.StatusCode,
			"the session must survive a failed logout. Got %d. Message: %s", after.StatusCode, afterBody)
	})

	t.Run("one_user_cannot_revoke_another_users_token", func(t *testing.T) {
		t.Parallel()

		_, attacker := loginAs(t)
		_, victim := loginAs(t)

		// The attacker is an ordinary logged-in user presenting their own access
		// token, but somebody else's refresh token. Accepting that would be a
		// denial of service against any account whose refresh token leaked into
		// a log or a referrer.
		out := logout(t, attacker.AccessToken, map[string]any{"refresh_token": victim.RefreshToken})
		outBody := readResponseBody(t, out)
		require.NoError(t, out.Body.Close())

		assert.NotEqual(t, http.StatusOK, out.StatusCode,
			"revoking a token belonging to another account must be refused. Got %d. Message: %s", out.StatusCode, outBody)

		// And the victim's session must be untouched.
		after, afterBody := refresh(t, victim.RefreshToken)
		defer after.Body.Close()

		assert.Equal(t, http.StatusOK, after.StatusCode,
			"the victim's refresh token must still work. Got %d. Message: %s", after.StatusCode, afterBody)
	})

	t.Run("logging_out_does_not_affect_another_session", func(t *testing.T) {
		t.Parallel()

		// Two sessions for the same account, as two devices would be.
		firstName, lastName, email := generateUserData(t)
		password := generatePassword(t)

		userID := createUserInDB(t, firstName, lastName, email, password)
		t.Cleanup(func() { deleteUserByEmailFromDB(t, email) })
		enableUserByEmailFromDB(t, email)
		assignRoleToUserInDB(t, userID, "AuthenticatedUser")

		sessions := make([]payload.LoginUserResponse, 0, 2)

		for range 2 {
			response, err := sendHTTPRequest(t, t.Context(), authLoginEndpoint, map[string]any{
				"email":    email,
				"password": password,
			})
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, response.StatusCode)

			session, err := parserResponseBody[payload.LoginUserResponse](t, response)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())

			sessions = append(sessions, session)
		}

		out := logout(t, sessions[0].AccessToken, map[string]any{"refresh_token": sessions[0].RefreshToken})
		require.Equal(t, http.StatusOK, out.StatusCode)
		require.NoError(t, out.Body.Close())

		// Revocation is per token, not per user: signing out of one device must
		// not sign you out of the other.
		after, afterBody := refresh(t, sessions[1].RefreshToken)
		defer after.Body.Close()

		assert.Equal(t, http.StatusOK, after.StatusCode,
			"the other session must survive. Got %d. Message: %s", after.StatusCode, afterBody)
	})
}
