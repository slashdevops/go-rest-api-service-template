//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
)

// TestAuthRefreshRevalidatesTheAccount covers the gap where a refresh token
// outlived the account it was issued for.
//
// RefreshAccessToken used to rebuild an identity from the token's own claims
// and never read the user, so nothing that happened to the account in between
// could take effect. Disabling someone stopped them logging in but not
// refreshing — they kept minting fresh access tokens for the whole refresh
// lifetime, 30 days under the dev configuration. Reproduced against the running
// API before the fix:
//
//	login with correct password    -> 401  "user is disabled"
//	existing access token, GET /me -> 200
//	POST /auth/refresh             -> 200  minted a NEW token, valid 86400s
//
// The account is disabled through the API rather than with direct SQL on
// purpose: the user read is cached, and it is the service's own invalidation
// that makes the change visible. An out-of-band UPDATE would not be seen until
// the entry expired, which is a property of the cache, not of this check.
func TestAuthRefreshRevalidatesTheAccount(t *testing.T) {
	t.Run("disabled_account_cannot_refresh", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		require.NotEmpty(t, adminToken, "Admin token should not be empty")

		adminHeader := map[string]string{
			"Authorization": "Bearer " + adminToken.AccessToken,
		}

		// 1. A user who can log in.
		firstName, lastName, email := generateUserData(t)
		password := generatePassword(t)

		userID := createUserInDB(t, firstName, lastName, email, password)
		t.Cleanup(func() { deleteUserByEmailFromDB(t, email) })

		enableUserByEmailFromDB(t, email)
		assignRoleToUserInDB(t, userID, "AuthenticatedUser")

		loginResponse, err := sendHTTPRequest(t, ctx, authLoginEndpoint, map[string]any{
			"email":    email,
			"password": password,
		})
		require.NoError(t, err, "Error sending login request")
		defer loginResponse.Body.Close()

		require.Equal(t, http.StatusOK, loginResponse.StatusCode,
			"Expected login to succeed. Got %d. Message: %s", loginResponse.StatusCode, readResponseBody(t, loginResponse))

		login, err := parserResponseBody[payload.LoginUserResponse](t, loginResponse)
		require.NoError(t, err, "Error parsing login response")
		require.NotEmpty(t, login.RefreshToken, "Expected a refresh token")

		refreshBody := map[string]any{"refresh_token": login.RefreshToken}
		refreshHeader := map[string]string{"Authorization": "Bearer " + login.RefreshToken}

		// 2. The refresh token works while the account is live. Without this the
		//    test would pass even if refresh were broken outright.
		beforeResponse, err := sendHTTPRequest(t, ctx, authRefreshEndpoint, refreshBody, refreshHeader)
		require.NoError(t, err, "Error sending refresh request")
		defer beforeResponse.Body.Close()

		require.Equal(t, http.StatusOK, beforeResponse.StatusCode,
			"Expected refresh to succeed for a live account. Got %d. Message: %s", beforeResponse.StatusCode, readResponseBody(t, beforeResponse))

		// 3. Disable the account through the API, which invalidates its cache entry.
		disableEndpoint := usersUpdateEndpoint.Clone().RewriteSlugs(userID.String())

		disableResponse, err := sendHTTPRequest(t, ctx, disableEndpoint, map[string]any{
			"first_name": firstName,
			"last_name":  lastName,
			"disabled":   true,
		}, adminHeader)
		require.NoError(t, err, "Error sending disable request")
		defer disableResponse.Body.Close()

		require.Equal(t, http.StatusOK, disableResponse.StatusCode,
			"Expected the account to be disabled. Got %d. Message: %s", disableResponse.StatusCode, readResponseBody(t, disableResponse))

		// 4. The same refresh token must now be refused.
		afterResponse, err := sendHTTPRequest(t, ctx, authRefreshEndpoint, refreshBody, refreshHeader)
		require.NoError(t, err, "Error sending refresh request")
		defer afterResponse.Body.Close()

		body := readResponseBody(t, afterResponse)

		assert.Equal(t, http.StatusUnauthorized, afterResponse.StatusCode,
			"A disabled account must not be able to mint a new access token. Got %d. Message: %s", afterResponse.StatusCode, body)
		assert.NotEqual(t, http.StatusInternalServerError, afterResponse.StatusCode,
			"Rejecting a disabled account is not a server fault; InvalidRefreshTokenError must map to 401")
		assert.Contains(t, body, "disabled", "The rejection should say why")
	})

	t.Run("deleted_account_cannot_refresh", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		firstName, lastName, email := generateUserData(t)
		password := generatePassword(t)

		userID := createUserInDB(t, firstName, lastName, email, password)
		enableUserByEmailFromDB(t, email)
		assignRoleToUserInDB(t, userID, "AuthenticatedUser")

		loginResponse, err := sendHTTPRequest(t, ctx, authLoginEndpoint, map[string]any{
			"email":    email,
			"password": password,
		})
		require.NoError(t, err, "Error sending login request")
		defer loginResponse.Body.Close()

		require.Equal(t, http.StatusOK, loginResponse.StatusCode,
			"Expected login to succeed. Got %d. Message: %s", loginResponse.StatusCode, readResponseBody(t, loginResponse))

		login, err := parserResponseBody[payload.LoginUserResponse](t, loginResponse)
		require.NoError(t, err, "Error parsing login response")

		// Removed from under the token. CheckAuthz would eventually deny this
		// user for lack of roles, but the refresh endpoint has to reject it on
		// its own — it is what issues the credential the rest of the API trusts.
		deleteUserByEmailFromDB(t, email)

		afterResponse, err := sendHTTPRequest(t, ctx, authRefreshEndpoint,
			map[string]any{"refresh_token": login.RefreshToken},
			map[string]string{"Authorization": "Bearer " + login.RefreshToken})
		require.NoError(t, err, "Error sending refresh request")
		defer afterResponse.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, afterResponse.StatusCode,
			"A deleted account must not be able to mint a new access token. Got %d. Message: %s",
			afterResponse.StatusCode, readResponseBody(t, afterResponse))
	})
}
