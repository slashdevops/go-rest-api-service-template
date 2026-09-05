//go:build integration

package integration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
)

var (
	tokenLifetimesGetEndpoint    = newAPIEndpoint(http.MethodGet, "/auth/token_lifetimes")
	tokenLifetimesUpdateEndpoint = newAPIEndpoint(http.MethodPut, "/auth/token_lifetimes")
)

// The value under test is GLOBAL: every other test in this suite logs in and
// gets tokens with whatever lifetimes are stored. Every test that changes it
// restores the seeded defaults in a cleanup, with context.Background() because
// the test context is cancelled before cleanups run.
func restoreTokenLifetimes(t *testing.T, accessToken string) {
	t.Helper()

	hdr := map[string]string{"Authorization": "Bearer " + accessToken}

	resp, err := sendHTTPRequest(t, context.Background(), tokenLifetimesUpdateEndpoint, map[string]any{
		"access_token_duration":  "5m",
		"refresh_token_duration": "24h",
	}, hdr)
	if err != nil {
		t.Logf("could not restore the token lifetimes: %v; the next test that logs in will see the changed values", err)

		return
	}

	resp.Body.Close()
}

func getTokenLifetimes(t *testing.T, accessToken string) payload.TokenLifetimesResponse {
	t.Helper()

	hdr := map[string]string{"Authorization": "Bearer " + accessToken}

	resp, err := sendHTTPRequest(t, t.Context(), tokenLifetimesGetEndpoint, nil, hdr)
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", readResponseBody(t, resp))

	var got payload.TokenLifetimesResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))

	return got
}

func putTokenLifetimes(t *testing.T, accessToken, access, refresh string) (*http.Response, string) {
	t.Helper()

	hdr := map[string]string{"Authorization": "Bearer " + accessToken}

	resp, err := sendHTTPRequest(t, t.Context(), tokenLifetimesUpdateEndpoint, map[string]any{
		"access_token_duration":  access,
		"refresh_token_duration": refresh,
	}, hdr)
	require.NoError(t, err)

	return resp, readResponseBody(t, resp)
}

// jwtExpiresIn decodes exp from an unverified JWT. The suite has no key; it
// only needs the number the API wrote.
func jwtExpiresIn(t *testing.T, token string) time.Duration {
	t.Helper()

	parts := strings.Split(token, ".")
	require.Len(t, parts, 3, "not a JWT")

	var claims struct {
		Exp int64 `json:"exp"`
		Iat int64 `json:"iat"`
	}

	require.NoError(t, json.Unmarshal(base64URLDecode(t, parts[1]), &claims))
	require.NotZero(t, claims.Exp, "exp is required")

	return time.Duration(claims.Exp-claims.Iat) * time.Second
}

func base64URLDecode(t *testing.T, segment string) []byte {
	t.Helper()

	out, err := base64.RawURLEncoding.DecodeString(segment)
	require.NoError(t, err, "JWT segment is not base64url")

	return out
}

// GET returns the stored values with the bounds and defaults a client
// validates against, so nothing in a client has to hardcode a number.
func TestTokenLifetimesGetCarriesBoundsAndDefaults(t *testing.T) {
	tokens := getAdminUserTokens(t)

	got := getTokenLifetimes(t, tokens.AccessToken)

	assert.Equal(t, "2m0s", got.Bounds.AccessTokenDuration.Min)
	assert.Equal(t, "48h0m0s", got.Bounds.AccessTokenDuration.Max)
	assert.Equal(t, "12h0m0s", got.Bounds.RefreshTokenDuration.Min)
	assert.Equal(t, "168h0m0s", got.Bounds.RefreshTokenDuration.Max)
	assert.Equal(t, "5m0s", got.Defaults.AccessTokenDuration)
	assert.Equal(t, "24h0m0s", got.Defaults.RefreshTokenDuration)
	assert.NotEmpty(t, got.AccessTokenDuration)
	assert.NotEmpty(t, got.RefreshTokenDuration)
}

// The whole point: a PUT changes what the NEXT login is issued with, without a
// restart, and the change is visible on the same replica at once.
func TestTokenLifetimesUpdateAppliesToTheNextLogin(t *testing.T) {
	admin := getAdminUserTokens(t)
	t.Cleanup(func() { restoreTokenLifetimes(t, admin.AccessToken) })

	resp, body := putTokenLifetimes(t, admin.AccessToken, "10m", "72h")
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)

	var updated payload.TokenLifetimesResponse
	require.NoError(t, json.Unmarshal([]byte(body), &updated))
	assert.Equal(t, "10m0s", updated.AccessTokenDuration)
	assert.Equal(t, "72h0m0s", updated.RefreshTokenDuration)
	require.NotNil(t, updated.UpdatedBy, "a change must be attributed to the caller")
	assert.Equal(t, admin.UserID, *updated.UpdatedBy)

	t.Run("get_agrees_with_the_put", func(t *testing.T) {
		got := getTokenLifetimes(t, admin.AccessToken)
		assert.Equal(t, "10m0s", got.AccessTokenDuration)
		assert.Equal(t, "72h0m0s", got.RefreshTokenDuration)
	})

	t.Run("the_next_login_carries_the_new_lifetimes", func(t *testing.T) {
		_, login := loginAs(t)

		assert.Equal(t, 10*time.Minute, jwtExpiresIn(t, login.AccessToken), "access token exp must follow the new lifetime")
		assert.Equal(t, 72*time.Hour, jwtExpiresIn(t, login.RefreshToken), "refresh token exp must follow the new lifetime")
	})

	t.Run("a_token_already_issued_keeps_its_expiry", func(t *testing.T) {
		// The admin logged in under the previous lifetimes, before the PUT.
		// Nothing about the change may touch a token already signed.
		assert.Equal(t, 5*time.Minute, jwtExpiresIn(t, admin.AccessToken))
	})
}

// A refresh mints a new ACCESS token under the current lifetime, but the
// refresh token carries the expiry the session started with -- rotation never
// renews it. The refresh lifetime applies at the next login only.
func TestTokenLifetimesRefreshRenewsTheAccessTokenOnly(t *testing.T) {
	admin := getAdminUserTokens(t)
	t.Cleanup(func() { restoreTokenLifetimes(t, admin.AccessToken) })

	// A session started under the defaults.
	_, login := loginAs(t)
	sessionExpiry := jwtExpiresIn(t, login.RefreshToken)
	require.Equal(t, 24*time.Hour, sessionExpiry)

	resp, body := putTokenLifetimes(t, admin.AccessToken, "15m", "96h")
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)

	refreshed, refreshedBody := refresh(t, login.RefreshToken)
	defer refreshed.Body.Close()

	require.Equal(t, http.StatusOK, refreshed.StatusCode, "body: %s", refreshedBody)

	var out payload.RefreshTokenResponse
	require.NoError(t, json.Unmarshal([]byte(refreshedBody), &out))

	assert.Equal(t, 15*time.Minute, jwtExpiresIn(t, out.AccessToken), "the new access token takes the current lifetime")

	// The rotated refresh token expires when the ORIGINAL would have, so its
	// remaining life is at most the 24h it started with -- never the 96h that
	// a login started now would get.
	remaining := jwtExpiresIn(t, out.RefreshToken)
	assert.LessOrEqual(t, remaining, 24*time.Hour, "rotation must carry the session's original expiry, not renew it")
	assert.Greater(t, remaining, 23*time.Hour)
}

// Every rejection names its field and writes nothing. The valid body these
// mutate is 10m / 72h, so a validator that refused everything could not pass
// the test above.
func TestTokenLifetimesRejects(t *testing.T) {
	admin := getAdminUserTokens(t)
	t.Cleanup(func() { restoreTokenLifetimes(t, admin.AccessToken) })

	cases := []struct {
		name    string
		access  string
		refresh string
		field   string
	}{
		{"access_below_minimum", "1m59s", "72h", "access_token_duration"},
		{"access_above_maximum", "48h1s", "168h", "access_token_duration"},
		{"refresh_below_minimum", "10m", "11h59m59s", "refresh_token_duration"},
		{"refresh_above_maximum", "10m", "168h1s", "refresh_token_duration"},
		{"refresh_equal_to_access", "24h", "24h", "refresh_token_duration"},
		{"refresh_shorter_than_access", "48h", "12h", "refresh_token_duration"},
		{"access_not_a_duration", "ten minutes", "72h", "access_token_duration"},
		{"refresh_not_a_duration", "10m", "3 days", "refresh_token_duration"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := putTokenLifetimes(t, admin.AccessToken, tc.access, tc.refresh)
			resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "body: %s", body)
			assert.Contains(t, body, tc.field, "the rejection must name the field")
		})
	}

	// And after all of that, nothing changed.
	got := getTokenLifetimes(t, admin.AccessToken)
	assert.Equal(t, "5m0s", got.AccessTokenDuration)
	assert.Equal(t, "24h0m0s", got.RefreshTokenDuration)
}

// The two boundaries are inclusive, and the widest pair is accepted.
func TestTokenLifetimesAcceptsTheBoundaries(t *testing.T) {
	admin := getAdminUserTokens(t)
	t.Cleanup(func() { restoreTokenLifetimes(t, admin.AccessToken) })

	for _, tc := range []struct{ access, refresh string }{
		{"2m", "12h"},
		{"48h", "168h"},
	} {
		resp, body := putTokenLifetimes(t, admin.AccessToken, tc.access, tc.refresh)
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "%s / %s: %s", tc.access, tc.refresh, body)
	}
}

func TestTokenLifetimesRequiresAuthentication(t *testing.T) {
	resp, err := sendHTTPRequest(t, t.Context(), tokenLifetimesGetEndpoint, nil, nil)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// An ordinary authenticated user holds no policy on the resource: the seeded
// AuthenticatedUser role does not cover /auth/token_lifetimes.
func TestTokenLifetimesRequiresAuthorization(t *testing.T) {
	_, login := loginAs(t)
	hdr := map[string]string{"Authorization": "Bearer " + login.AccessToken}

	resp, err := sendHTTPRequest(t, t.Context(), tokenLifetimesGetEndpoint, nil, hdr)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "body: %s", readResponseBody(t, resp))

	put, body := putTokenLifetimes(t, login.AccessToken, "10m", "72h")
	put.Body.Close()
	assert.Equal(t, http.StatusForbidden, put.StatusCode, "body: %s", body)
}

// The case the revoked_tokens.token_type column exists for. With the old
// horizon-based mirror a lifetime raised past the reload window silently
// missed revocations; a logout under a 48h access token must still refuse the
// access token, immediately on the replica that served the logout and by type
// on every other.
func TestTokenLifetimesLogoutStillRevokesUnderARaisedAccessLifetime(t *testing.T) {
	admin := getAdminUserTokens(t)
	t.Cleanup(func() { restoreTokenLifetimes(t, admin.AccessToken) })

	resp, body := putTokenLifetimes(t, admin.AccessToken, "48h", "168h")
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)

	_, login := loginAs(t)
	require.Equal(t, 48*time.Hour, jwtExpiresIn(t, login.AccessToken))

	hdr := map[string]string{"Authorization": "Bearer " + login.AccessToken}

	before, err := sendHTTPRequest(t, t.Context(), meAuthzEndpoint, nil, hdr)
	require.NoError(t, err)
	before.Body.Close()
	require.Equal(t, http.StatusOK, before.StatusCode, "the token must work before logout: %s", readResponseBody(t, before))

	out := logout(t, login.AccessToken, map[string]any{"refresh_token": login.RefreshToken})
	out.Body.Close()
	require.Equal(t, http.StatusOK, out.StatusCode)

	after, err := sendHTTPRequest(t, t.Context(), meAuthzEndpoint, nil, hdr)
	require.NoError(t, err)

	defer after.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, after.StatusCode,
		"a logged-out 48h access token must be refused; if it is not, the revocation mirror is no longer selecting by type")
}
