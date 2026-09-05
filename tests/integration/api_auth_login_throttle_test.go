//go:build integration

package integration

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
)

// attemptLogin posts credentials and returns the status and body.
//
// forwardedFor, when set, spoofs the source address. It is what makes the point
// of this whole test: the per-IP limiter can be walked around by varying it, so
// the per-account throttle has to hold without it.
func attemptLogin(t *testing.T, email, password, forwardedFor string) (*http.Response, string) {
	t.Helper()

	headers := map[string]string{}
	if forwardedFor != "" {
		headers["X-Forwarded-For"] = forwardedFor
	}

	response, err := sendHTTPRequest(t, t.Context(), authLoginEndpoint, map[string]any{
		"email":    email,
		"password": password,
	}, headers)
	require.NoError(t, err, "Error sending login request")

	return response, readResponseBody(t, response)
}

// TestAuthLoginThrottle covers the half of the brute-force problem the per-IP
// rate limiter cannot reach.
//
// The IP limiter bounds how fast one source can call the API. Spread the same
// guesses over enough addresses and each one stays under its own limit while
// the account underneath is hammered — and before the trusted-proxy fix, a
// single client could manufacture those addresses with a header. Guessing has
// to be bounded per account as well as per source.
func TestAuthLoginThrottle(t *testing.T) {
	t.Run("failed_attempts_are_refused_after_the_budget_is_spent", func(t *testing.T) {
		t.Parallel()

		firstName, lastName, email := generateUserData(t)
		password := generatePassword(t)

		userID := createUserInDB(t, firstName, lastName, email, password)
		t.Cleanup(func() { deleteUserByEmailFromDB(t, email) })

		enableUserByEmailFromDB(t, email)
		assignRoleToUserInDB(t, userID, "AuthenticatedUser")

		// Each attempt spoofs a DIFFERENT source, so the per-IP limiter never
		// sees the same caller twice and cannot be what refuses these.
		var (
			unauthorized int
			throttled    int
			lastBody     string
			retryAfter   string
		)

		for i := range 12 {
			response, body := attemptLogin(t, email, "ThisIsAWrongPw0rd.,", "198.51.100."+strconv.Itoa(i+1))

			switch response.StatusCode {
			case http.StatusUnauthorized:
				unauthorized++
			case http.StatusTooManyRequests:
				throttled++
				lastBody = body
				retryAfter = response.Header.Get("Retry-After")
			}

			require.NoError(t, response.Body.Close())
		}

		assert.Positive(t, throttled,
			"12 wrong-password attempts from 12 different sources must eventually be refused; got %d×401 and %d×429",
			unauthorized, throttled)
		assert.Less(t, unauthorized, 12,
			"if every attempt was evaluated the account is not throttled at all")

		if throttled > 0 {
			seconds, err := strconv.Atoi(retryAfter)
			assert.NoError(t, err, "Retry-After must be an integer number of seconds, got %q", retryAfter)
			assert.Positive(t, seconds, "Retry-After must be positive; a client reading 0 would retry straight into another refusal")

			assert.NotContains(t, lastBody, email,
				"the throttle response must not echo the address back")
		}
	})

	t.Run("a_correct_password_still_works_after_a_few_typos", func(t *testing.T) {
		t.Parallel()

		firstName, lastName, email := generateUserData(t)
		password := generatePassword(t)

		userID := createUserInDB(t, firstName, lastName, email, password)
		t.Cleanup(func() { deleteUserByEmailFromDB(t, email) })

		enableUserByEmailFromDB(t, email)
		assignRoleToUserInDB(t, userID, "AuthenticatedUser")

		// Two mistypes, well inside the default budget of five.
		for range 2 {
			response, _ := attemptLogin(t, email, "ThisIsAWrongPw0rd.,", "")
			require.Equal(t, http.StatusUnauthorized, response.StatusCode)
			require.NoError(t, response.Body.Close())
		}

		response, body := attemptLogin(t, email, password, "")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode,
			"a correct password after a couple of typos must still work. Got %d. Message: %s", response.StatusCode, body)

		// And the success must have handed the budget back, so the next few
		// mistakes are evaluated rather than refused outright.
		for i := range 3 {
			next, nextBody := attemptLogin(t, email, "ThisIsAWrongPw0rd.,", "")
			assert.Equal(t, http.StatusUnauthorized, next.StatusCode,
				"attempt %d after a success should be evaluated, not throttled. Message: %s", i+1, nextBody)
			require.NoError(t, next.Body.Close())
		}
	})

	t.Run("an_unknown_address_is_throttled_like_a_real_one", func(t *testing.T) {
		t.Parallel()

		// No account is created. If unknown addresses were exempt, the
		// difference in behaviour would answer "does this address have an
		// account?" — which is the question the throttle exists to make
		// expensive to ask.
		_, _, email := generateUserData(t)

		var throttled int

		for i := range 12 {
			response, _ := attemptLogin(t, email, "ThisIsAWrongPw0rd.,", "203.0.113."+strconv.Itoa(i+1))
			if response.StatusCode == http.StatusTooManyRequests {
				throttled++
			}
			require.NoError(t, response.Body.Close())
		}

		assert.Positive(t, throttled,
			"an address with no account must consume budget exactly like one that has an account")
	})

	t.Run("one_account_being_guessed_does_not_lock_out_another", func(t *testing.T) {
		t.Parallel()

		// Victim: burn its budget.
		_, _, victimEmail := generateUserData(t)
		for range 12 {
			response, _ := attemptLogin(t, victimEmail, "ThisIsAWrongPw0rd.,", "")
			require.NoError(t, response.Body.Close())
		}

		// Bystander: a real account, untouched, must still be able to log in.
		firstName, lastName, email := generateUserData(t)
		password := generatePassword(t)

		userID := createUserInDB(t, firstName, lastName, email, password)
		t.Cleanup(func() { deleteUserByEmailFromDB(t, email) })

		enableUserByEmailFromDB(t, email)
		assignRoleToUserInDB(t, userID, "AuthenticatedUser")

		response, body := attemptLogin(t, email, password, "")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode,
			"throttling one account must not affect any other. Got %d. Message: %s", response.StatusCode, body)

		login, err := parserResponseBody[payload.LoginUserResponse](t, response)
		require.NoError(t, err)
		assert.NotEmpty(t, login.AccessToken)
	})
}
