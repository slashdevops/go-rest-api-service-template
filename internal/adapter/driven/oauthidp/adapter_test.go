//go:build unit

package oauthidp

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/oauth"
)

// fakeOIDC is an OpenID Connect provider in a test server: discovery, JWKS, a
// token endpoint that mints an RS256 ID token, and user-info. It is a REAL
// provider as far as go-oidc can tell, which is the point -- a mock replaying
// this package's own structs would assert nothing about discovery, PKCE, the
// nonce or the signature.
type fakeOIDC struct {
	srv    *httptest.Server
	key    *rsa.PrivateKey
	claims map[string]any // what the next ID token and user-info carry
	// what the token endpoint saw, so a test can assert PKCE reached it
	gotVerifier string
	gotCode     string
	nonce       string
}

func newFakeOIDC(t *testing.T) *fakeOIDC {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	f := &fakeOIDC{key: key, claims: map[string]any{}}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                f.srv.URL,
			"authorization_endpoint":                f.srv.URL + "/authorize",
			"token_endpoint":                        f.srv.URL + "/token",
			"userinfo_endpoint":                     f.srv.URL + "/userinfo",
			"jwks_uri":                              f.srv.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"code_challenge_methods_supported":      []string{"S256"},
		})
	})

	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, _ *http.Request) {
		pub := &key.PublicKey
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{{
			"kty": "RSA", "kid": "test-key", "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}}})
	})

	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.gotVerifier = r.Form.Get("code_verifier")
		f.gotCode = r.Form.Get("code")

		claims := jwt.MapClaims{
			"iss": f.srv.URL, "aud": "client-id", "sub": "subject-1",
			"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(), "nonce": f.nonce,
		}
		for k, v := range f.claims {
			claims[k] = v
		}

		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		tok.Header["kid"] = "test-key"

		signed, err := tok.SignedString(key)
		if err != nil {
			t.Fatal(err)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-1", "token_type": "Bearer", "expires_in": 3600, "id_token": signed,
		})
	})

	mux.HandleFunc("GET /userinfo", func(w http.ResponseWriter, _ *http.Request) {
		out := map[string]any{"sub": "subject-1"}
		for k, v := range f.claims {
			out[k] = v
		}

		_ = json.NewEncoder(w).Encode(out)
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)

	return f
}

func newProvider(t *testing.T) *Provider {
	t.Helper()

	p, err := New(Config{HTTPClient: &http.Client{Timeout: 5 * time.Second}})
	if err != nil {
		t.Fatal(err)
	}

	return p
}

func oidcIDP(issuer, typeName string) *domain.IDP {
	return &domain.IDP{
		Name: "Test " + typeName, ClientID: "client-id", ClientSecret: "secret",
		CallbackURL: "https://app.example/auth/idp/x/callback", IssuerURL: issuer, Enabled: true,
		IDPType: domain.IDPTypes{Name: typeName, Kind: domain.IDPTypeKindOIDC, Scopes: []string{"openid", "email", "profile"}},
	}
}

// The authorization URL carries what the flow needs on the way out: the
// state, an S256 challenge derived from the verifier (never the verifier), the
// nonce, and the openid scope -- and none of the Google-only flags.
func TestOIDCAuthCodeURLCarriesPKCEAndNonce(t *testing.T) {
	t.Parallel()

	f := newFakeOIDC(t)
	p := newProvider(t)

	raw, err := p.AuthCodeURL(t.Context(), oidcIDP(f.srv.URL, "Okta"), oauth.AuthRequest{State: "st", Nonce: "nn", CodeVerifier: "verifier-verifier-verifier-verifier-verifier"})
	if err != nil {
		t.Fatalf("AuthCodeURL: %v", err)
	}

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}

	q := u.Query()

	if !strings.HasPrefix(raw, f.srv.URL+"/authorize") {
		t.Errorf("must go to the DISCOVERED authorization endpoint, got %s", raw)
	}

	if q.Get("state") != "st" || q.Get("nonce") != "nn" || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
		t.Errorf("missing state/nonce/PKCE: %s", q.Encode())
	}

	if strings.Contains(raw, "verifier-verifier") {
		t.Error("the PKCE verifier must never appear in the authorization URL")
	}

	if !strings.Contains(q.Get("scope"), "openid") {
		t.Errorf("openid scope missing: %q", q.Get("scope"))
	}

	if q.Get("access_type") != "" || q.Get("prompt") != "" {
		t.Errorf("Google-only options leaked into a generic provider's URL: %s", q.Encode())
	}
}

// The exchange presents the verifier, verifies the ID token against the
// discovered keys, checks the nonce, and maps the claims.
func TestOIDCExchangeVerifiesTheIDTokenAndMapsClaims(t *testing.T) {
	t.Parallel()

	f := newFakeOIDC(t)
	f.nonce = "nonce-1"
	f.claims = map[string]any{"email": "Jane@Example.com", "email_verified": true, "given_name": "Jane", "family_name": "Doe"}

	p := newProvider(t)

	info, err := p.Exchange(t.Context(), oidcIDP(f.srv.URL, "Okta"), "code-1", oauth.AuthRequest{Nonce: "nonce-1", CodeVerifier: "verifier-verifier-verifier-verifier-verifier"})
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}

	if f.gotVerifier != "verifier-verifier-verifier-verifier-verifier" || f.gotCode != "code-1" {
		t.Errorf("token endpoint saw verifier=%q code=%q", f.gotVerifier, f.gotCode)
	}

	want := domain.UserInfo{Subject: "subject-1", Email: "jane@example.com", EmailVerified: true, FirstName: "Jane", LastName: "Doe"}
	if *info != want {
		t.Errorf("UserInfo = %+v, want %+v", *info, want)
	}
}

// A nonce that does not match is a token minted for another request.
func TestOIDCExchangeRefusesAWrongNonce(t *testing.T) {
	t.Parallel()

	f := newFakeOIDC(t)
	f.nonce = "minted-for-someone-else"
	f.claims = map[string]any{"email": "jane@example.com", "email_verified": true}

	p := newProvider(t)

	_, err := p.Exchange(t.Context(), oidcIDP(f.srv.URL, "Okta"), "code-1", oauth.AuthRequest{Nonce: "mine", CodeVerifier: "v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v"})
	if err == nil {
		t.Fatal("a nonce mismatch must be refused")
	}

	if strings.Contains(err.Error(), "oidc:") {
		t.Errorf("the library's wording leaked: %v", err)
	}
}

// An ID token from another issuer -- same vendor, another tenant -- does not
// verify against the row's issuer.
func TestOIDCExchangeRefusesAnotherIssuer(t *testing.T) {
	t.Parallel()

	f := newFakeOIDC(t)
	f.nonce = "n"
	f.claims = map[string]any{"iss": "https://login.microsoftonline.com/another-tenant/v2.0", "email": "x@example.com"}

	p := newProvider(t)

	_, err := p.Exchange(t.Context(), oidcIDP(f.srv.URL, "EntraID"), "c", oauth.AuthRequest{Nonce: "n", CodeVerifier: "v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v"})
	if err == nil {
		t.Fatal("a token issued by another tenant must be refused")
	}
}

// Entra ID: no email_verified claim, email possibly absent from the token and
// present as preferred_username. Single tenant means the address is the
// directory's own attribute, so it is verified by construction.
func TestEntraIDMapsPreferredUsernameAndTrustsTheTenant(t *testing.T) {
	t.Parallel()

	f := newFakeOIDC(t)
	f.nonce = "n"
	f.claims = map[string]any{"preferred_username": "jane@corp.example", "name": "Jane Doe"}

	p := newProvider(t)

	info, err := p.Exchange(t.Context(), oidcIDP(f.srv.URL, "EntraID"), "c", oauth.AuthRequest{Nonce: "n", CodeVerifier: "v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v"})
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}

	if info.Email != "jane@corp.example" || !info.EmailVerified || info.FirstName != "Jane" || info.LastName != "Doe" {
		t.Errorf("UserInfo = %+v", *info)
	}
}

// Google and Okta assert email_verified; false is passed through as false.
func TestOIDCPassesAnUnverifiedEmailThroughAsUnverified(t *testing.T) {
	t.Parallel()

	f := newFakeOIDC(t)
	f.nonce = "n"
	f.claims = map[string]any{"email": "x@example.com", "email_verified": false, "name": "Cher"}

	p := newProvider(t)

	info, err := p.Exchange(t.Context(), oidcIDP(f.srv.URL, "Google"), "c", oauth.AuthRequest{Nonce: "n", CodeVerifier: "v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v"})
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}

	if info.EmailVerified {
		t.Error("email_verified=false must not become true")
	}

	// A single-word name is a first name, not an error.
	if info.FirstName != "Cher" || info.LastName != "Cher" {
		t.Errorf("name split = %q %q", info.FirstName, info.LastName)
	}
}

// GitHub: the identity is the numeric id, the email the primary VERIFIED
// address from /user/emails, and a null /user.email is not a failure.
func TestGithubReadsThePrimaryVerifiedEmail(t *testing.T) {
	// Not parallel: it swaps the package-level GitHub endpoint for the fake.
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "gh-at", "token_type": "bearer"})
	})
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 4242, "login": "octocat", "name": "The Octocat", "email": nil})
	})
	mux.HandleFunc("GET /user/emails", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"email": "old@example.com", "primary": false, "verified": true},
			{"email": "Octo@Example.com", "primary": true, "verified": true},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	p := newProvider(t)

	// The github kind uses fixed endpoints; point them at the fake.
	saved := githubEndpoint
	githubEndpoint = oauth2.Endpoint{AuthURL: srv.URL + "/login/oauth/authorize", TokenURL: srv.URL + "/login/oauth/access_token"}
	t.Cleanup(func() { githubEndpoint = saved })

	idp := &domain.IDP{
		Name: "GH", ClientID: "cid", ClientSecret: "sec", CallbackURL: "https://app.example/cb", Enabled: true,
		IDPType: domain.IDPTypes{Name: "Github", Kind: domain.IDPTypeKindGithub, UserInfoAPIURL: srv.URL + "/user", Scopes: []string{"read:user", "user:email"}},
	}

	info, err := p.Exchange(t.Context(), idp, "code", oauth.AuthRequest{CodeVerifier: "v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v-v"})
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}

	want := domain.UserInfo{Subject: "4242", Email: "octo@example.com", EmailVerified: true, FirstName: "The", LastName: "Octocat"}
	if *info != want {
		t.Errorf("UserInfo = %+v, want %+v", *info, want)
	}
}

// The kind decides, never the name: a row named anything works for its kind,
// and an unknown kind is refused before any network call.
func TestKindDecidesNotTheName(t *testing.T) {
	t.Parallel()

	p := newProvider(t)

	idp := oidcIDP("http://127.0.0.1:1", "Company SSO")
	idp.IDPType.Kind = "saml"

	if _, err := p.AuthCodeURL(t.Context(), idp, oauth.AuthRequest{State: "s", Nonce: "n", CodeVerifier: "v"}); err == nil {
		t.Fatal("an unknown kind must be refused")
	}
}

// New refuses a client without a timeout: the token exchange used to run on
// http.DefaultClient, which has none.
func TestNewRequiresATimeout(t *testing.T) {
	t.Parallel()

	if _, err := New(Config{HTTPClient: &http.Client{}}); err == nil {
		t.Fatal("a client without a timeout must be refused")
	}

	if _, err := New(Config{}); err == nil {
		t.Fatal("a nil client must be refused")
	}
}
