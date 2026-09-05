//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// TestAuthLoginFailuresAreIndistinguishable closes the enumeration oracle.
//
// Login answered 401 for every failure, which is right, but the body carried the
// underlying domain error and the three cases read differently. Against the
// running API before the fix:
//
//	unknown email   -> "user: not found: User not found with email: nobody@example.com"
//	wrong password  -> "invalid password: invalid password"
//	disabled acct   -> "invalid user status: user is disabled"
//
// The unknown-address case even echoed the probed address back, which turns one
// request into a definitive answer about whether someone has an account. The
// per-account throttle makes asking expensive; this makes the answer useless.
func TestAuthLoginFailuresAreIndistinguishable(t *testing.T) {
	t.Run("unknown_wrong_password_and_disabled_all_answer_alike", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		require.NotEmpty(t, adminToken)

		adminHeader := map[string]string{"Authorization": "Bearer " + adminToken.AccessToken}

		// A real, enabled account.
		firstName, lastName, liveEmail := generateUserData(t)
		livePassword := generatePassword(t)

		liveID := createUserInDB(t, firstName, lastName, liveEmail, livePassword)
		t.Cleanup(func() { deleteUserByEmailFromDB(t, liveEmail) })
		enableUserByEmailFromDB(t, liveEmail)
		assignRoleToUserInDB(t, liveID, "AuthenticatedUser")

		// A real account that has been disabled.
		dFirst, dLast, disabledEmail := generateUserData(t)
		disabledPassword := generatePassword(t)

		disabledID := createUserInDB(t, dFirst, dLast, disabledEmail, disabledPassword)
		t.Cleanup(func() { deleteUserByEmailFromDB(t, disabledEmail) })
		enableUserByEmailFromDB(t, disabledEmail)
		assignRoleToUserInDB(t, disabledID, "AuthenticatedUser")

		disableResponse, err := sendHTTPRequest(t, ctx,
			usersUpdateEndpoint.Clone().RewriteSlugs(disabledID.String()),
			map[string]any{"first_name": dFirst, "last_name": dLast, "disabled": true},
			adminHeader)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, disableResponse.StatusCode,
			"Expected the account to be disabled. Got %d. Message: %s", disableResponse.StatusCode, readResponseBody(t, disableResponse))
		require.NoError(t, disableResponse.Body.Close())

		// An address with no account at all.
		_, _, unknownEmail := generateUserData(t)

		cases := []struct {
			name     string
			email    string
			password string
		}{
			{"unknown_address", unknownEmail, generatePassword(t)},
			{"wrong_password", liveEmail, generatePassword(t)},
			{"disabled_account", disabledEmail, disabledPassword},
		}

		bodies := make(map[string]string, len(cases))

		for _, tc := range cases {
			response, body := attemptLogin(t, tc.email, tc.password, "")

			assert.Equal(t, http.StatusUnauthorized, response.StatusCode,
				"%s should answer 401. Got %d. Message: %s", tc.name, response.StatusCode, body)
			assert.NotContains(t, body, tc.email,
				"%s must not echo the address that was probed", tc.name)

			message, err := messageOf(body)
			require.NoError(t, err, "%s: could not read the response body: %s", tc.name, body)

			assert.Equal(t, domain.AuthnInvalidCredentials, message,
				"%s must answer with the one opaque message", tc.name)

			bodies[tc.name] = message
			require.NoError(t, response.Body.Close())
		}

		assert.Equal(t, bodies["unknown_address"], bodies["wrong_password"],
			"an address with no account must be indistinguishable from a wrong password")
		assert.Equal(t, bodies["wrong_password"], bodies["disabled_account"],
			"a disabled account must be indistinguishable from a wrong password")
	})

	t.Run("a_correct_password_still_logs_in", func(t *testing.T) {
		t.Parallel()

		// The uniform failure is only worth anything if success still works —
		// an easy thing to break while collapsing five branches into one.
		firstName, lastName, email := generateUserData(t)
		password := generatePassword(t)

		userID := createUserInDB(t, firstName, lastName, email, password)
		t.Cleanup(func() { deleteUserByEmailFromDB(t, email) })
		enableUserByEmailFromDB(t, email)
		assignRoleToUserInDB(t, userID, "AuthenticatedUser")

		response, body := attemptLogin(t, email, password, "")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode,
			"Expected login to succeed. Got %d. Message: %s", response.StatusCode, body)
	})
}
