//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
)

var authRecoverEndpoint = newAPIEndpoint(http.MethodPost, "/auth/password/recover")

// recover asks for a password-recovery email and returns what the caller sees.
func requestRecovery(t *testing.T, email string) (int, string) {
	t.Helper()

	response, err := sendHTTPRequest(t, t.Context(), authRecoverEndpoint, map[string]any{"email": email})
	require.NoError(t, err, "Error requesting password recovery")

	body, err := parserResponseBody[payload.HTTPMessage](t, response)
	require.NoError(t, err, "Error parsing the response")
	require.NoError(t, response.Body.Close())

	return response.StatusCode, body.Message
}

// TestPasswordRecoveryAnswersTheSameWay covers the half of A-06 that was left
// behind: login was made uniform, password recovery was not.
//
// It was a better account oracle than login had ever been -- no password to
// guess and no throttle in the way. Measured against the running API before
// this fix:
//
//	no such address       -> 500, with the probed address echoed back
//	a local account       -> 200 "Password recovery email sent"
//	an IdP-backed account -> 500 "user is not a local account"
//
// So one unauthenticated request told a caller whether an address had an
// account, and whether that account signs in through an identity provider --
// which is exactly what someone choosing a target wants to know.
func TestPasswordRecoveryAnswersTheSameWay(t *testing.T) {
	t.Parallel()

	// A local account that can genuinely be recovered: the baseline every other
	// case has to be indistinguishable from.
	firstName, lastName, localEmail := generateUserData(t)
	createUserInDB(t, firstName, lastName, localEmail, generatePassword(t))
	t.Cleanup(func() { deleteUserByEmailFromDB(t, localEmail) })
	enableUserByEmailFromDB(t, localEmail)

	// An account that signs in through an identity provider, so it has no
	// password to recover.
	idpFirst, idpLast, idpEmail := generateUserData(t)
	createUserInDB(t, idpFirst, idpLast, idpEmail, generatePassword(t))
	t.Cleanup(func() { deleteUserByEmailFromDB(t, idpEmail) })
	setLocalAccountInDB(t, idpEmail, false)

	// A disabled account. createUserInDB leaves an account disabled until it is
	// verified, which is what makes this the ordinary unverified case too.
	disFirst, disLast, disabledEmail := generateUserData(t)
	createUserInDB(t, disFirst, disLast, disabledEmail, generatePassword(t))
	t.Cleanup(func() { deleteUserByEmailFromDB(t, disabledEmail) })

	// Short enough to be a VALID address that simply has no account. A longer
	// one is rejected by validation instead (the limit is 50 characters), which
	// is a different path and would make this test pass for the wrong reason.
	_, _, unknownEmail := generateUserData(t)

	baseStatus, baseMessage := requestRecovery(t, localEmail)
	require.Equal(t, http.StatusOK, baseStatus,
		"a recoverable account must succeed, or this test proves nothing. Message: %s", baseMessage)

	for name, email := range map[string]string{
		"an address with no account":        unknownEmail,
		"an account that uses an IdP":       idpEmail,
		"an account that has been disabled": disabledEmail,
	} {
		t.Run(name, func(t *testing.T) {
			status, message := requestRecovery(t, email)

			assert.Equal(t, baseStatus, status,
				"%s must answer with the same status as a recoverable account", name)
			assert.Equal(t, baseMessage, message,
				"%s must answer with the same message as a recoverable account", name)

			// And it must never hand the probed address back, which is how the
			// unknown-address case used to confirm itself.
			assert.NotContains(t, message, email,
				"the response must not echo the address that was probed")
		})
	}
}

// TestPasswordRecoveryIsBoundedPerAddress covers the other half: recovery was
// reachable at the per-IP limiter's rate and no slower.
//
// That left enumeration at speed, and a way to send a great deal of mail to
// somebody else's address. The budget is keyed on the address that was
// submitted, before anything is looked up, so an address with no account is
// throttled exactly like a real one.
func TestPasswordRecoveryIsBoundedPerAddress(t *testing.T) {
	t.Parallel()

	firstName, lastName, email := generateUserData(t)
	password := generatePassword(t)

	userID := createUserInDB(t, firstName, lastName, email, password)
	t.Cleanup(func() { deleteUserByEmailFromDB(t, email) })
	enableUserByEmailFromDB(t, email)
	assignRoleToUserInDB(t, userID, "AuthenticatedUser")

	// Ask repeatedly until the budget is refused. The exact ceiling is
	// configuration; that it HAS one is the behaviour.
	var (
		refused    bool
		refusedAt  int
		retryAfter string
	)

	for i := 1; i <= 25; i++ {
		response, err := sendHTTPRequest(t, t.Context(), authRecoverEndpoint, map[string]any{"email": email})
		require.NoError(t, err, "Error requesting password recovery")

		status := response.StatusCode
		retryAfter = response.Header.Get("Retry-After")
		require.NoError(t, response.Body.Close())

		if status == http.StatusTooManyRequests {
			refused, refusedAt = true, i

			break
		}

		require.Equal(t, http.StatusOK, status, "request %d should either succeed or be throttled", i)
	}

	require.True(t, refused,
		"recovery must be bounded per address; 25 requests for one address were all accepted")
	assert.NotEmpty(t, retryAfter, "a throttled response must say when to try again")

	t.Logf("recovery for one address was refused at request %d", refusedAt)

	// And the budget must be its OWN. Spending recovery must not lock the same
	// address out of signing in: that would turn a mild abuse control into a
	// denial of service anyone could trigger against any address they know.
	loginResponse, err := sendHTTPRequest(t, t.Context(), authLoginEndpoint, map[string]any{
		"email":    email,
		"password": password,
	})
	require.NoError(t, err, "Error logging in")

	loginBody := readResponseBody(t, loginResponse)
	require.NoError(t, loginResponse.Body.Close())

	assert.Equal(t, http.StatusOK, loginResponse.StatusCode,
		"spending the recovery budget must not affect signing in. Got %d. Message: %s",
		loginResponse.StatusCode, loginBody)
}
