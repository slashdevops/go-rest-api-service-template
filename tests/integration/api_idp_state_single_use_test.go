//go:build integration

package integration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	idpLoginEndpoint    = newAPIEndpoint(http.MethodGet, "/auth/idp/{idp_id}/login")
	idpCallbackEndpoint = newAPIEndpoint(http.MethodGet, "/auth/idp/{idp_id}/callback")
)

// createIDP registers an identity provider and returns its id.
func createIDP(t *testing.T, accessToken string) string {
	t.Helper()

	header := map[string]string{"Authorization": "Bearer " + accessToken}
	idpID := mustUUIDString(t)

	response, err := sendHTTPRequest(t, t.Context(), idpsCreateEndpoint, map[string]any{
		"id": idpID,
		// The Github kind: an oidc provider would fetch its discovery document
		// when the flow starts, and this test is about the state, not the network.
		"idp_type_id":   getIDPTypeFromDBByName(t, "Github").ID,
		"name":          generateRandomName(t, "StateIDP"),
		"description":   "created by the state replay test",
		"callback_url":  "http://localhost:8080/api/v1/auth/idp/callback",
		"logo":          "https://example.com/logo.png",
		"client_id":     "test-client-id",
		"client_secret": "test-client-secret",
	}, header)
	require.NoError(t, err, "Error creating the IDP")
	require.Equal(t, http.StatusCreated, response.StatusCode,
		"Expected the IDP to be created. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))
	require.NoError(t, response.Body.Close())

	t.Cleanup(func() {
		// A fresh context: t.Context() is already cancelled by the time cleanup
		// runs, so the request would fail before it was sent.
		resp, err := sendHTTPRequest(t, context.Background(), idpsDeleteEndpoint.Clone().RewriteSlugs(idpID), nil, header)
		if err == nil {
			_ = resp.Body.Close()
		}
	})

	return idpID
}

// stateFromLoginRedirect starts a sign-in flow and returns the state the
// service put in the redirect it hands the browser.
func stateFromLoginRedirect(t *testing.T, idpID string) string {
	t.Helper()

	response, err := sendHTTPRequest(t, t.Context(), idpLoginEndpoint.Clone().RewriteSlugs(idpID), nil)
	require.NoError(t, err, "Error starting the IDP login flow")
	defer response.Body.Close()

	// The endpoint hands back the provider URL for the caller to follow rather
	// than redirecting itself, so the state is in the body.
	var out struct {
		RedirectURL string `json:"redirect_url"`
	}

	body := readResponseBody(t, response)
	require.NoError(t, json.Unmarshal([]byte(body), &out),
		"Expected a redirect_url. Got %d. Message: %s", response.StatusCode, body)

	parsed, err := url.Parse(out.RedirectURL)
	require.NoError(t, err, "the redirect must be a URL")

	state := parsed.Query().Get("state")
	require.NotEmpty(t, state, "the redirect must carry a state")

	return state
}

// jtiOf reads the jti out of a token without verifying it.
func jtiOf(t *testing.T, token string) string {
	t.Helper()

	parts := strings.Split(token, ".")
	require.Len(t, parts, 3, "expected a three-part JWT")

	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err, "Error decoding the JWT payload")

	var claims map[string]any
	require.NoError(t, json.Unmarshal(raw, &claims), "Error parsing the JWT payload")

	jti, _ := claims["jti"].(string)

	return jti
}

// TestIDPStateIsSingleUse covers G-02: the OAuth state was never spent.
//
// The state is the only thing binding a callback to the redirect that started
// it. It was verified — signature, token_type, event, idp — and then left
// usable for the rest of its life, so the same state could be presented again
// and again: a callback URL captured from a log, a Referer or browser history
// stayed live until it expired.
func TestIDPStateIsSingleUse(t *testing.T) {
	t.Run("a_state_is_accepted_once_and_refused_after", func(t *testing.T) {
		t.Parallel()

		admin := getAdminUserTokens(t)
		idpID := createIDP(t, admin.AccessToken)

		state := stateFromLoginRedirect(t, idpID)
		jti := jtiOf(t, state)
		require.NotEmpty(t, jti, "the state must carry a jti; without one it cannot be spent")

		callback := func() (int, string) {
			endpoint := idpCallbackEndpoint.Clone().RewriteSlugs(idpID)
			endpoint.requestURL.RawQuery = url.Values{
				"state": {state},
				"code":  {"a-code-the-provider-will-not-honour"},
			}.Encode()

			response, err := sendHTTPRequest(t, t.Context(), endpoint, nil)
			require.NoError(t, err, "Error calling the callback")

			body := readResponseBody(t, response)
			require.NoError(t, response.Body.Close())

			return response.StatusCode, body
		}

		// The first callback gets past the state and fails at the code
		// exchange, because the code is not one the provider issued. That is
		// the point: the state was spent on the way through.
		firstStatus, firstBody := callback()
		assert.NotContains(t, firstBody, "state is not valid",
			"the first use of a state must be accepted. Got %d. Message: %s", firstStatus, firstBody)

		// It is spent whether or not the exchange succeeded. A state that
		// survives a failed exchange is a state an attacker can retry.
		assert.True(t, isTokenRevokedInDB(t, jti),
			"the state's jti must be recorded as spent")

		secondStatus, secondBody := callback()
		assert.Contains(t, secondBody, "state is not valid",
			"a replayed state must be refused. Got %d. Message: %s", secondStatus, secondBody)

		// Neither answer may carry the oauth2 library's words. The first one
		// used to come back as `oauth2: "invalid_client" "The OAuth client was
		// not found."` -- a dependency's wording published as this API's
		// contract, and the provider's view of how our client is registered.
		for _, body := range []string{firstBody, secondBody} {
			assert.NotContains(t, body, "oauth2:",
				"the provider library's error text must not reach the caller: %s", body)
			assert.NotContains(t, body, "invalid_client",
				"the provider's view of our client registration must not reach the caller: %s", body)
		}
	})

	t.Run("two_flows_do_not_interfere", func(t *testing.T) {
		t.Parallel()

		// Spending one state must not affect another: they are separate
		// sign-in attempts, and a shared failure would break every concurrent
		// login.
		admin := getAdminUserTokens(t)
		idpID := createIDP(t, admin.AccessToken)

		first := stateFromLoginRedirect(t, idpID)
		second := stateFromLoginRedirect(t, idpID)

		assert.NotEqual(t, jtiOf(t, first), jtiOf(t, second),
			"each flow must get its own state, or one login would spend another's")
	})
}
