//go:build integration

package integration

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
)

// A fake OpenID Connect provider the RUNNING SERVICE can reach: discovery,
// JWKS, a token endpoint that mints RS256 ID tokens for whatever person the
// test says, and user-info. The service does discovery against it, sends the
// browser-side redirect to it, and exchanges codes with it -- the whole path,
// without a network beyond the loopback.
//
// The suite runs on the same host as the service, which is why an httptest
// server is reachable; the dev stack's pod runs the databases, not the API.
type fakeProvider struct {
	srv   *httptest.Server
	key   *rsa.PrivateKey
	mu    sync.Mutex
	users map[string]map[string]any // code -> claims
	nonce map[string]string         // code -> nonce asked for on /authorize
}

func newFakeProvider(t *testing.T) *fakeProvider {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	f := &fakeProvider{key: key, users: map[string]map[string]any{}, nonce: map[string]string{}}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": f.srv.URL, "authorization_endpoint": f.srv.URL + "/authorize", "token_endpoint": f.srv.URL + "/token",
			"userinfo_endpoint": f.srv.URL + "/userinfo", "jwks_uri": f.srv.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"}, "code_challenge_methods_supported": []string{"S256"},
		})
	})
	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, _ *http.Request) {
		pub := &key.PublicKey
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{{
			"kty": "RSA", "kid": "k1", "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		code := r.Form.Get("code")

		f.mu.Lock()
		claims, ok := f.users[code]
		nonce := f.nonce[code]
		f.mu.Unlock()

		if !ok || r.Form.Get("code_verifier") == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid_grant"})

			return
		}

		all := jwt.MapClaims{"iss": f.srv.URL, "aud": "client-id", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(), "nonce": nonce}
		for k, v := range claims {
			all[k] = v
		}

		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, all)
		tok.Header["kid"] = "k1"
		signed, _ := tok.SignedString(key)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 3600, "id_token": signed})
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)

	return f
}

// authorize plays the browser's visit to the provider: it records the nonce
// the service asked for and returns a code for the person the test names.
func (f *fakeProvider) authorize(t *testing.T, redirect string, person map[string]any) (state, code string) {
	t.Helper()

	u, err := url.Parse(redirect)
	require.NoError(t, err)
	require.Equal(t, f.srv.URL+"/authorize", u.Scheme+"://"+u.Host+u.Path, "the service must send the browser to the DISCOVERED endpoint")

	q := u.Query()
	require.NotEmpty(t, q.Get("code_challenge"), "PKCE challenge missing")
	require.Equal(t, "S256", q.Get("code_challenge_method"))
	require.NotEmpty(t, q.Get("nonce"), "nonce missing")

	code = mustUUIDString(t)

	f.mu.Lock()
	f.users[code] = person
	f.nonce[code] = q.Get("nonce")
	f.mu.Unlock()

	return q.Get("state"), code
}

var (
	idpRegisterEndpoint   = newAPIEndpoint(http.MethodGet, "/auth/idp/{idp_id}/register")
	idpAvailableEndpoint  = newAPIEndpoint(http.MethodGet, "/auth/idp/available")
	idpLinkEndpoint       = newAPIEndpoint(http.MethodGet, "/auth/idp/{idp_id}/link")
	meIdentitiesEndpoint  = newAPIEndpoint(http.MethodGet, "/me/identities")
	meIdentityUnlinkPoint = newAPIEndpoint(http.MethodDelete, "/me/identities/{idp_id}")
)

// createOIDCIDP registers an Okta-kind provider pointing at the fake.
func createOIDCIDP(t *testing.T, admin string, f *fakeProvider, autoProvision bool) string {
	t.Helper()

	hdr := map[string]string{"Authorization": "Bearer " + admin}
	id := mustUUIDString(t)

	resp, err := sendHTTPRequest(t, t.Context(), idpsCreateEndpoint, map[string]any{
		"id": id, "idp_type_id": getIDPTypeFromDBByName(t, "Okta").ID, "name": generateRandomName(t, "FlowIDP"),
		"description": "created by the IdP flow test", "callback_url": "http://localhost:5173/auth/idp/" + id + "/callback",
		"issuer_url": f.srv.URL, "client_id": "client-id", "client_secret": "client-secret", "logo": "https://example.com/logo.png",
		"auto_provision": autoProvision,
	}, hdr)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode, readResponseBody(t, resp))
	resp.Body.Close()

	t.Cleanup(func() {
		r, err := sendHTTPRequest(t, context.Background(), idpsDeleteEndpoint.Clone().RewriteSlugs(id), nil, hdr)
		if err == nil {
			r.Body.Close()
		}
	})

	return id
}

func startFlow(t *testing.T, endpoint *apiEndpoint, idpID string, hdr map[string]string) string {
	t.Helper()

	resp, err := sendHTTPRequest(t, t.Context(), endpoint.Clone().RewriteSlugs(idpID), nil, hdr)
	require.NoError(t, err)

	body := readResponseBody(t, resp)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, body)

	var out struct {
		RedirectURL string `json:"redirect_url"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &out))

	return out.RedirectURL
}

func callback(t *testing.T, idpID, state, code string) (int, string) {
	t.Helper()

	ep := idpCallbackEndpoint.Clone().RewriteSlugs(idpID)
	ep.requestURL.RawQuery = url.Values{"state": {state}, "code": {code}}.Encode()

	resp, err := sendHTTPRequest(t, t.Context(), ep, nil)
	require.NoError(t, err)

	body := readResponseBody(t, resp)
	resp.Body.Close()

	return resp.StatusCode, body
}

func person(sub, email string, verified bool) map[string]any {
	return map[string]any{"sub": sub, "email": email, "email_verified": verified, "given_name": "Flow", "family_name": "Test"}
}

// The whole path: discovery, redirect with PKCE and nonce, exchange, a
// verified new email is provisioned and linked, and the callback answers a
// session as JSON with no cookie.
func TestIDPFlowProvisionsAndSignsIn(t *testing.T) {
	admin := getAdminUserTokens(t)
	f := newFakeProvider(t)
	idpID := createOIDCIDP(t, admin.AccessToken, f, true)

	_, _, email := generateUserData(t)
	t.Cleanup(func() { deleteUserByEmailFromDB(t, email) })

	state, code := f.authorize(t, startFlow(t, idpLoginEndpoint, idpID, nil), person("sub-"+email, email, true))

	status, body := callback(t, idpID, state, code)
	require.Equal(t, http.StatusOK, status, body)

	var out payload.IDPCallbackResponse
	require.NoError(t, json.Unmarshal([]byte(body), &out))
	require.Equal(t, "login", out.Event)
	require.NotNil(t, out.Login, "a login answers a session")
	assert.NotEmpty(t, out.Login.AccessToken)

	// The session works and belongs to the provisioned account.
	me, err := sendHTTPRequest(t, t.Context(), meAuthzEndpoint, nil, map[string]string{"Authorization": "Bearer " + out.Login.AccessToken})
	require.NoError(t, err)
	me.Body.Close()
	assert.Equal(t, http.StatusOK, me.StatusCode)

	// And the identity is linked, visible on the profile.
	list, err := sendHTTPRequest(t, t.Context(), meIdentitiesEndpoint, nil, map[string]string{"Authorization": "Bearer " + out.Login.AccessToken})
	require.NoError(t, err)

	var ids payload.ListUserIdentitiesResponse
	require.NoError(t, json.NewDecoder(list.Body).Decode(&ids))
	list.Body.Close()
	require.Len(t, ids.Items, 1)
	assert.Equal(t, idpID, ids.Items[0].IDPID.String())

	// A second sign-in with the same subject and a CHANGED email is the same
	// account: the subject is the identity.
	state, code = f.authorize(t, startFlow(t, idpLoginEndpoint, idpID, nil), person("sub-"+email, "renamed-"+email, true))
	status, body = callback(t, idpID, state, code)
	require.Equal(t, http.StatusOK, status, body)

	var again payload.IDPCallbackResponse
	require.NoError(t, json.Unmarshal([]byte(body), &again))
	assert.Equal(t, out.Login.UserID, again.Login.UserID)
}

// The takeover that used to exist: a provider identity whose email matches
// an existing password account is refused, and the account keeps its password.
func TestIDPFlowRefusesAnUnlinkedIdentityWithAnExistingEmail(t *testing.T) {
	admin := getAdminUserTokens(t)
	f := newFakeProvider(t)
	idpID := createOIDCIDP(t, admin.AccessToken, f, true)

	victimEmail, victim := loginAs(t)

	state, code := f.authorize(t, startFlow(t, idpLoginEndpoint, idpID, nil), person("attacker-sub", victimEmail, true))
	status, body := callback(t, idpID, state, code)
	assert.Equal(t, http.StatusUnauthorized, status, body)
	assert.NotContains(t, body, victimEmail, "the refusal must not echo the address")

	// The victim still signs in with the password: local_account was not
	// flipped by the attempt.
	probe, err := sendHTTPRequest(t, t.Context(), meAuthzEndpoint, nil, map[string]string{"Authorization": "Bearer " + victim.AccessToken})
	require.NoError(t, err)
	probe.Body.Close()
	assert.Equal(t, http.StatusOK, probe.StatusCode)
}

// Unverified email, and auto-provisioning off: both refused with the same
// wording as the case above.
func TestIDPFlowProvisioningGates(t *testing.T) {
	admin := getAdminUserTokens(t)
	f := newFakeProvider(t)

	t.Run("unverified_email", func(t *testing.T) {
		idpID := createOIDCIDP(t, admin.AccessToken, f, true)
		_, _, email := generateUserData(t)

		state, code := f.authorize(t, startFlow(t, idpRegisterEndpoint, idpID, nil), person("s-"+email, email, false))
		status, body := callback(t, idpID, state, code)
		assert.Equal(t, http.StatusUnauthorized, status, body)
		assert.Zero(t, countUsersByEmailInDB(t, email), "nothing may be created")
	})

	t.Run("auto_provision_off", func(t *testing.T) {
		idpID := createOIDCIDP(t, admin.AccessToken, f, false)
		_, _, email := generateUserData(t)

		state, code := f.authorize(t, startFlow(t, idpLoginEndpoint, idpID, nil), person("s-"+email, email, true))
		status, body := callback(t, idpID, state, code)
		assert.Equal(t, http.StatusUnauthorized, status, body)
		assert.Zero(t, countUsersByEmailInDB(t, email))
	})
}

// A signed-in user links the provider from their profile; afterwards the
// identity signs in as them, and the last way in cannot be removed from a
// password-less account -- but a password account may unlink it.
func TestIDPFlowLinkAndUnlink(t *testing.T) {
	admin := getAdminUserTokens(t)
	f := newFakeProvider(t)
	idpID := createOIDCIDP(t, admin.AccessToken, f, true)

	email, me := loginAs(t)
	hdr := map[string]string{"Authorization": "Bearer " + me.AccessToken}

	// Start the link as the signed-in user.
	state, code := f.authorize(t, startFlow(t, idpLinkEndpoint, idpID, hdr), person("linked-sub", email, true))
	status, body := callback(t, idpID, state, code)
	require.Equal(t, http.StatusOK, status, body)

	var out payload.IDPCallbackResponse
	require.NoError(t, json.Unmarshal([]byte(body), &out))
	assert.Equal(t, "link", out.Event)
	assert.Nil(t, out.Login, "a link issues no session")
	require.NotNil(t, out.LinkedTo)
	assert.Equal(t, me.UserID, *out.LinkedTo)

	// The identity now signs in as this account.
	state, code = f.authorize(t, startFlow(t, idpLoginEndpoint, idpID, nil), person("linked-sub", email, true))
	status, body = callback(t, idpID, state, code)
	require.Equal(t, http.StatusOK, status, body)

	var login payload.IDPCallbackResponse
	require.NoError(t, json.Unmarshal([]byte(body), &login))
	assert.Equal(t, me.UserID, login.Login.UserID)

	// Unlink is allowed: the account has a password.
	del, err := sendHTTPRequest(t, t.Context(), meIdentityUnlinkPoint.Clone().RewriteSlugs(idpID), nil, hdr)
	require.NoError(t, err)
	del.Body.Close()
	assert.Equal(t, http.StatusOK, del.StatusCode)

	// And is gone.
	state, code = f.authorize(t, startFlow(t, idpLoginEndpoint, idpID, nil), person("linked-sub", email, true))
	status, body = callback(t, idpID, state, code)
	assert.Equal(t, http.StatusUnauthorized, status, body)
}

// A disabled provider is not offered, cannot start a flow, and does not
// complete one that was started before it was disabled.
func TestIDPFlowDisabledProvider(t *testing.T) {
	admin := getAdminUserTokens(t)
	f := newFakeProvider(t)
	idpID := createOIDCIDP(t, admin.AccessToken, f, true)
	hdr := map[string]string{"Authorization": "Bearer " + admin.AccessToken}

	redirect := startFlow(t, idpLoginEndpoint, idpID, nil)

	upd, err := sendHTTPRequest(t, t.Context(), idpsUpdateEndpoint.Clone().RewriteSlugs(idpID), map[string]any{"enabled": false}, hdr)
	require.NoError(t, err)
	upd.Body.Close()
	require.Equal(t, http.StatusOK, upd.StatusCode)

	avail, err := sendHTTPRequest(t, t.Context(), idpAvailableEndpoint, nil, nil)
	require.NoError(t, err)
	assert.NotContains(t, readResponseBody(t, avail), idpID)
	avail.Body.Close()

	start, err := sendHTTPRequest(t, t.Context(), idpLoginEndpoint.Clone().RewriteSlugs(idpID), nil, nil)
	require.NoError(t, err)
	start.Body.Close()
	assert.Equal(t, http.StatusNotFound, start.StatusCode)

	_, _, email := generateUserData(t)
	state, code := f.authorize(t, redirect, person("s", email, true))
	status, _ := callback(t, idpID, state, code)
	assert.Equal(t, http.StatusNotFound, status)
}
