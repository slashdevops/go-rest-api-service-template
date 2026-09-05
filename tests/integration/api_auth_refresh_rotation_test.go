//go:build integration

package integration

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
	"github.com/slashdevops/go-rest-api-service-template/internal/config"
)

// jwtClaims decodes a token's payload without verifying it. The service has
// already vouched for anything these tests are handed; what is wanted here is
// the jti and exp, which are otherwise invisible from the outside.
func jwtClaims(t *testing.T, token string) map[string]any {
	t.Helper()

	parts := strings.Split(token, ".")
	require.Len(t, parts, 3, "Expected a three-part JWT")

	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err, "Error decoding the JWT payload")

	var claims map[string]any
	require.NoError(t, json.Unmarshal(raw, &claims), "Error parsing the JWT payload")

	return claims
}

func refreshOK(t *testing.T, token string) payload.RefreshTokenResponse {
	t.Helper()

	response, body := refresh(t, token)
	defer response.Body.Close()

	require.Equal(t, http.StatusOK, response.StatusCode,
		"Expected the refresh to succeed. Got %d. Message: %s", response.StatusCode, body)

	out, err := parserResponseBody[payload.RefreshTokenResponse](t, response)
	require.NoError(t, err, "Error parsing the refresh response")

	return out
}

// TestAuthRefreshRotation covers A-08: refresh tokens were never rotated.
//
// RefreshAccessToken returned the token it was given, so one credential minted
// at login stayed valid for its whole life no matter how often it was used.
// A copy of it was indistinguishable from the original — there was nothing in
// the exchange that could ever reveal that two parties held the same token.
//
// Rotation makes each refresh token single-use, and the denylist records what
// replaced it. That record is what turns a second use into evidence.
func TestAuthRefreshRotation(t *testing.T) {
	t.Run("refresh_issues_a_new_refresh_token", func(t *testing.T) {
		t.Parallel()

		_, login := loginAs(t)

		rotated := refreshOK(t, login.RefreshToken)

		assert.NotEqual(t, login.RefreshToken, rotated.RefreshToken,
			"the refresh token must be replaced, not handed back")
		assert.NotEqual(t,
			jwtClaims(t, login.RefreshToken)["jti"], jwtClaims(t, rotated.RefreshToken)["jti"],
			"the successor must be a new link in the chain, not a re-signing of the same jti")

		// The token that came back has to be usable, or rotation has simply
		// broken refresh.
		next := refreshOK(t, rotated.RefreshToken)
		assert.NotEmpty(t, next.AccessToken, "the rotated token must mint access tokens")
	})

	t.Run("rotation_does_not_extend_the_session", func(t *testing.T) {
		t.Parallel()

		_, login := loginAs(t)

		// The wait is what gives this test its teeth. A token seconds old and a
		// freshly renewed one expire at almost the same instant, so comparing
		// them straight after login cannot tell a carried-over expiry from a
		// renewed one — the assertion holds either way and proves nothing.
		// Letting the original age first separates the two answers by the time
		// waited.
		const age = 6 * time.Second

		time.Sleep(age)

		rotated := refreshOK(t, login.RefreshToken)

		// Every link carries the expiry of the token that started the chain.
		// Renewing it on each refresh would make a session that is merely
		// active immortal, which is a decision about how long people stay
		// logged in and not one rotation should make on its way past.
		before := jwtClaims(t, login.RefreshToken)["exp"].(float64)
		after := jwtClaims(t, rotated.RefreshToken)["exp"].(float64)

		assert.InDelta(t, before, after, age.Seconds()/2,
			"the rotated token must expire when the original would have, not %v later", age)
	})

	t.Run("a_retry_inside_the_grace_window_returns_the_same_successor", func(t *testing.T) {
		t.Parallel()

		_, login := loginAs(t)

		first := refreshOK(t, login.RefreshToken)

		// The same token again, immediately. This is what a client that never
		// received the first answer does — a dropped response, a crash before
		// the new token was stored, two requests refreshing at once. None of
		// them is a theft, and ending the session over one would make the
		// alarm useless by firing it constantly.
		retry := refreshOK(t, login.RefreshToken)

		assert.Equal(t,
			jwtClaims(t, first.RefreshToken)["jti"], jwtClaims(t, retry.RefreshToken)["jti"],
			"a retry must be answered with the successor that was already issued, not a second one")

		// And the successor must still work: the retry must not have disturbed
		// the session it was trying to continue.
		next := refreshOK(t, first.RefreshToken)
		assert.NotEmpty(t, next.AccessToken, "the session must survive a retried refresh")
	})

	t.Run("replaying_a_rotated_token_after_the_grace_window_ends_the_session", func(t *testing.T) {
		t.Parallel()

		_, login := loginAs(t)

		live := refreshOK(t, login.RefreshToken)

		// Past the grace window a second presentation is no longer explicable
		// as a retry: the legitimate client moved on to the successor long ago,
		// so whoever is holding this one copied it.
		//
		// This sleeps because the window is the only thing separating a replay
		// from a retry — there is no other signal, which is the whole reason
		// the grace is a duration rather than a flag.
		time.Sleep(config.DefaultAuthnRefreshTokenRotationGrace + 2*time.Second)

		replayed, replayedBody := refresh(t, login.RefreshToken)
		require.NoError(t, replayed.Body.Close())

		assert.Equal(t, http.StatusUnauthorized, replayed.StatusCode,
			"a replayed refresh token must be refused. Got %d. Message: %s", replayed.StatusCode, replayedBody)

		// The point of detection: the token the legitimate client is holding
		// dies too. Nothing in the request says which party is which, so the
		// only safe answer is to end the chain for both — the alternative
		// leaves whoever stole it with a working session.
		after, afterBody := refresh(t, live.RefreshToken)
		defer after.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, after.StatusCode,
			"detecting a replay must end the whole chain, including the live token. Got %d. Message: %s",
			after.StatusCode, afterBody)
	})

	t.Run("logging_out_with_an_already_rotated_token_still_ends_the_session", func(t *testing.T) {
		t.Parallel()

		_, login := loginAs(t)

		live := refreshOK(t, login.RefreshToken)

		// The client logs out with the token it started the session with, not
		// the one it is currently on. Revoking that link alone is a no-op — it
		// was already spent — and would answer 200 while the session carried
		// on, which is the bug logout was fixed for in the first place.
		out := logout(t, login.AccessToken, map[string]any{"refresh_token": login.RefreshToken})
		require.Equal(t, http.StatusOK, out.StatusCode,
			"Expected logout to succeed. Got %d. Message: %s", out.StatusCode, readResponseBody(t, out))
		require.NoError(t, out.Body.Close())

		after, afterBody := refresh(t, live.RefreshToken)
		defer after.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, after.StatusCode,
			"logout must end the session at the live end of the chain. Got %d. Message: %s",
			after.StatusCode, afterBody)
	})
}
