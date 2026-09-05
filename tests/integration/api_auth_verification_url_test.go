//go:build integration

package integration

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVerificationTokenIsNotInTheURL covers A-12: the account-verification token
// travelled as a path segment, GET /auth/verify/{token}.
//
// Measured against the running API before the fix, one request wrote the token
// into the service's own log twice:
//
//	url=/auth/verify/eyJhbGciOiJFUzI1NiJ9.THIS_IS_A_VERIFICATION_TOKEN.sig
//	path=/auth/verify/eyJhbGciOiJFUzI1NiJ9.THIS_IS_A_VERIFICATION_TOKEN.sig
//
// A live credential, in the application log, on every verification — and in the
// browser history and Referer of whoever clicked the link. The password-reset
// flow already avoided this; verification did not.
func TestVerificationTokenIsNotInTheURL(t *testing.T) {
	t.Run("the_email_links_to_the_page_and_carries_the_token_as_a_query_parameter", func(t *testing.T) {
		t.Parallel()

		firstName, lastName, email := generateUserData(t)
		password := generatePassword(t)

		response, err := sendHTTPRequest(t, t.Context(), authRegisterEndpoint, map[string]any{
			"id":         mustUUIDString(t),
			"first_name": firstName,
			"last_name":  lastName,
			"email":      email,
			"password":   password,
		})
		require.NoError(t, err, "Error registering the user")
		require.Equal(t, http.StatusCreated, response.StatusCode,
			"Expected the user to be created. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
		require.NoError(t, response.Body.Close())

		t.Cleanup(func() { deleteUserByEmailFromDB(t, email) })

		time.Sleep(500 * time.Millisecond)

		link := getVerifyLinkFromEmail(t, verifyEmailAddress, email)

		parsed, err := url.Parse(link)
		require.NoError(t, err, "the verification link must be a URL")

		token := parsed.Query().Get("token")
		require.NotEmpty(t, token, "the link must carry the token as a query parameter")

		assert.NotContains(t, parsed.Path, token,
			"the token must not be part of the URL path, where the request log records it")
		assert.Equal(t, "/verify", parsed.Path,
			"the link must point at the page, not at an API route that logs what it is given")

		// And the token must actually work when handed over the way the page
		// hands it over, or this has traded a leak for a broken flow.
		confirmed, err := confirmVerification(t, token)
		require.NoError(t, err, "Error confirming the verification")
		defer confirmed.Body.Close()

		assert.Equal(t, http.StatusOK, confirmed.StatusCode,
			"the token from the email must verify the account. Got %d. Message: %s",
			confirmed.StatusCode, readResponseBody(t, confirmed))
	})

	t.Run("the_old_path_route_is_gone", func(t *testing.T) {
		t.Parallel()

		// Leaving it registered would leave the leak: anything still calling it
		// would keep writing tokens to the log.
		endpoint := newAPIEndpoint(http.MethodGet, "/auth/verify/some.jwt.token")

		response, err := sendHTTPRequest(t, t.Context(), endpoint, nil)
		require.NoError(t, err, "Error sending the request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusNotFound, response.StatusCode,
			"GET /auth/verify/{token} must no longer be routed. Got %d", response.StatusCode)
	})

	t.Run("a_token_of_another_class_is_refused", func(t *testing.T) {
		t.Parallel()

		// The endpoint takes a bearer token, so the token_type claim is what
		// stops an access token being spent as a verification token.
		admin := getAdminUserTokens(t)

		response, err := confirmVerification(t, admin.AccessToken)
		require.NoError(t, err, "Error sending the request")

		body := readResponseBody(t, response)
		require.NoError(t, response.Body.Close())

		assert.Equal(t, http.StatusUnauthorized, response.StatusCode,
			"an access token must not verify an account. Got %d. Message: %s", response.StatusCode, body)
		assert.True(t, strings.Contains(body, "email_verification") || strings.Contains(body, "Invalid"),
			"the rejection should be about the token, got: %s", body)
	})
}
