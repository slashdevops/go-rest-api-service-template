// Package oauthidp is the driven adapter that satisfies the oauth.Provider port.
//
// Two kinds of provider, decided by the IdP type's Kind and never by its name:
//
//   - oidc: discovery at <issuer>/.well-known/openid-configuration (cached per
//     issuer for the life of the process; the key set refreshes itself), the
//     authorization code flow with PKCE (S256) and a nonce, and the ID token
//     verified with coreos/go-oidc against the discovered JWKS, the issuer and
//     this client id. The identity is the token's `sub`. The user-info
//     endpoint is read only when the ID token carries no email, which Entra ID
//     does when the optional claim is not configured.
//   - github: GitHub has no OpenID Connect for users, so this is
//     golang.org/x/oauth2 against GitHub's fixed endpoints (with PKCE, which
//     GitHub accepts), /user for the identity and /user/emails for the primary
//     VERIFIED address -- /user.email is null for anyone whose email is private.
//
// # What a provider's word is worth
//
// UserInfo.EmailVerified is what gates account creation in the use case, and
// it is decided here, per provider: Google and Okta assert `email_verified`
// and it is passed through; GitHub's /user/emails says `verified` per address
// and only a verified primary is used; Entra ID has no such claim, and a
// single-tenant registration -- the only shape this service supports, the
// issuer pins the tenant -- means the email is the directory's own attribute,
// so it is treated as verified.
//
// # Errors
//
// The provider's own wording -- `oauth2: "invalid_client" ...`, go-oidc's
// "oidc: id token issued by a different provider" -- is logged at WARN with
// the IdP name and never returned: it would publish a dependency's text as
// this API's contract and tell whoever is probing how our client is
// registered with the provider.
package oauthidp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/oauth"
)

// Config configures [New].
type Config struct {
	// HTTPClient is the outbound client every request to a provider goes
	// through: discovery, the token exchange, JWKS, user-info. Required, and it
	// must carry a timeout -- golang.org/x/oauth2 falls back to
	// http.DefaultClient, which has none, and the token exchange used to run
	// on exactly that.
	HTTPClient *http.Client
}

// githubEndpoint is GitHub's fixed OAuth2 endpoint pair. A variable so the
// contract test can point it at a fake GitHub; nothing else assigns it.
var githubEndpoint = github.Endpoint

// Provider implements oauth.Provider.
type Provider struct {
	httpClient *http.Client

	// providers caches one go-oidc Provider per issuer. Discovery is one HTTP
	// round trip that never changes for a given issuer, and the provider
	// holds the remote key set, which refreshes itself when a signature does
	// not verify. Keyed on the issuer string exactly as stored, so two rows
	// with the same tenant share one.
	providers sync.Map // issuer string -> *oidc.Provider
}

var _ oauth.Provider = (*Provider)(nil)

// New constructs a Provider.
func New(conf Config) (*Provider, error) {
	if conf.HTTPClient == nil {
		return nil, fmt.Errorf("oauthidp: HTTPClient is required")
	}

	if conf.HTTPClient.Timeout <= 0 {
		return nil, fmt.Errorf("oauthidp: HTTPClient must carry a timeout; a provider that hangs must not hold a sign-in open forever")
	}

	return &Provider{httpClient: conf.HTTPClient}, nil
}

// AuthCodeURL implements oauth.Provider.
func (p *Provider) AuthCodeURL(ctx context.Context, idp *domain.IDP, req oauth.AuthRequest) (string, error) {
	if req.State == "" || req.CodeVerifier == "" {
		return "", &domain.InvalidIdentityProvidersError{Message: "a sign-in needs a state and a PKCE verifier"}
	}

	cfg, err := p.configFor(ctx, idp)
	if err != nil {
		return "", err
	}

	opts := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(req.CodeVerifier)}

	if idp.IsOIDC() {
		if req.Nonce == "" {
			return "", &domain.InvalidIdentityProvidersError{Message: "an OpenID Connect sign-in needs a nonce"}
		}

		opts = append(opts, oidc.Nonce(req.Nonce))
	}

	// No AccessTypeOffline and no ApprovalForce. Both are Google-specific: the
	// first asks for a refresh token this service never uses, the second forces
	// the consent screen on every sign-in, which Entra ID and Okta honour as
	// prompt=consent.
	return cfg.AuthCodeURL(req.State, opts...), nil
}

// Exchange implements oauth.Provider.
func (p *Provider) Exchange(ctx context.Context, idp *domain.IDP, code string, req oauth.AuthRequest) (*domain.UserInfo, error) {
	if code == "" || req.CodeVerifier == "" {
		return nil, &domain.InvalidIdentityProvidersError{Message: "the exchange needs the code and the PKCE verifier"}
	}

	cfg, err := p.configFor(ctx, idp)
	if err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, oauth2.HTTPClient, p.httpClient)

	token, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(req.CodeVerifier))
	if err != nil {
		slog.Warn("oauthidp: the identity provider refused the authorization code",
			"idp", idp.Name, "kind", idp.IDPType.Kind, "error", err)

		return nil, &domain.InvalidAuthnServiceError{Message: "the identity provider did not accept this sign-in"}
	}

	if idp.IsOIDC() {
		return p.identityFromIDToken(ctx, idp, cfg, token, req.Nonce)
	}

	return p.identityFromGithub(ctx, idp, cfg, token)
}

// configFor builds the oauth2.Config for one provider row.
func (p *Provider) configFor(ctx context.Context, idp *domain.IDP) (*oauth2.Config, error) {
	if idp == nil {
		return nil, &domain.InvalidIdentityProvidersError{Message: "unknown idp"}
	}

	if idp.ClientID == "" || idp.ClientSecret == "" || idp.CallbackURL == "" {
		return nil, &domain.InvalidIdentityProvidersError{Message: fmt.Sprintf("idp %s is missing required fields", idp.Name)}
	}

	switch idp.IDPType.Kind {
	case domain.IDPTypeKindOIDC:
		if idp.IssuerURL == "" {
			return nil, &domain.InvalidIdentityProvidersError{Message: fmt.Sprintf("idp %s has no issuer URL", idp.Name)}
		}

		provider, err := p.discover(ctx, idp)
		if err != nil {
			return nil, err
		}

		return &oauth2.Config{
			Endpoint:     provider.Endpoint(),
			ClientID:     idp.ClientID,
			ClientSecret: idp.ClientSecret,
			RedirectURL:  idp.CallbackURL,
			Scopes:       withOpenID(idp.IDPType.Scopes),
		}, nil

	case domain.IDPTypeKindGithub:
		scopes := idp.IDPType.Scopes
		if len(scopes) == 0 {
			scopes = []string{"read:user", "user:email"}
		}

		return &oauth2.Config{
			Endpoint:     githubEndpoint,
			ClientID:     idp.ClientID,
			ClientSecret: idp.ClientSecret,
			RedirectURL:  idp.CallbackURL,
			Scopes:       scopes,
		}, nil

	default:
		// The kind, not the name. A provider the adapter does not know how to
		// talk to used to be one whose NAME the switch did not list, so a row
		// called anything but "Google" or "Github" failed at the callback.
		return nil, &domain.InvalidIdentityProvidersError{Message: fmt.Sprintf("idp %s has an unknown kind %q", idp.Name, idp.IDPType.Kind)}
	}
}

// discover returns the cached go-oidc provider for the issuer, fetching the
// discovery document on first use.
func (p *Provider) discover(ctx context.Context, idp *domain.IDP) (*oidc.Provider, error) {
	if cached, ok := p.providers.Load(idp.IssuerURL); ok {
		return cached.(*oidc.Provider), nil
	}

	ctx = oidc.ClientContext(ctx, p.httpClient)

	provider, err := oidc.NewProvider(ctx, idp.IssuerURL)
	if err != nil {
		slog.Warn("oauthidp: discovery failed for the identity provider",
			"idp", idp.Name, "issuer", idp.IssuerURL, "error", err)

		return nil, &domain.IDPUnreachableError{Message: "the identity provider is not reachable"}
	}

	actual, _ := p.providers.LoadOrStore(idp.IssuerURL, provider)

	return actual.(*oidc.Provider), nil
}

// identityFromIDToken verifies the ID token and maps its claims.
func (p *Provider) identityFromIDToken(ctx context.Context, idp *domain.IDP, cfg *oauth2.Config, token *oauth2.Token, nonce string) (*domain.UserInfo, error) {
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		slog.Warn("oauthidp: the token response carried no id_token", "idp", idp.Name)

		return nil, &domain.InvalidAuthnServiceError{Message: "the identity provider did not return an identity"}
	}

	provider, err := p.discover(ctx, idp)
	if err != nil {
		return nil, err
	}

	// Issuer, audience (this client id), expiry and signature against the
	// discovered keys. The issuer is the one the row names, so a token from
	// another tenant of the same vendor -- same keys, different iss -- is
	// refused here.
	idToken, err := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}).Verify(oidc.ClientContext(ctx, p.httpClient), rawIDToken)
	if err != nil {
		slog.Warn("oauthidp: the ID token did not verify", "idp", idp.Name, "error", err)

		return nil, &domain.InvalidAuthnServiceError{Message: "the identity provider did not return a valid identity"}
	}

	if idToken.Nonce != nonce {
		slog.Warn("oauthidp: the ID token's nonce does not match this sign-in", "idp", idp.Name)

		return nil, &domain.InvalidAuthnServiceError{Message: "the identity provider did not return a valid identity"}
	}

	var claims oidcClaims
	if err := idToken.Claims(&claims); err != nil {
		slog.Warn("oauthidp: the ID token's claims could not be read", "idp", idp.Name, "error", err)

		return nil, &domain.InvalidAuthnServiceError{Message: "the identity provider did not return a valid identity"}
	}

	// Entra ID sends `email` only when the optional claim is configured on
	// the app registration; the user-info endpoint has it in the same shape.
	// One extra round trip, only when the token was short.
	if claims.Email == "" {
		info, err := provider.UserInfo(oidc.ClientContext(ctx, p.httpClient), oauth2.StaticTokenSource(token))
		if err != nil {
			slog.Warn("oauthidp: could not read the account details from the identity provider", "idp", idp.Name, "error", err)
		} else {
			var extra oidcClaims
			if err := info.Claims(&extra); err == nil {
				claims.merge(extra)
			}
		}
	}

	return mapOIDCClaims(idp, idToken.Subject, claims)
}

// oidcClaims are the claims this service reads from an ID token or user-info
// document. Everything else the provider sends is ignored.
type oidcClaims struct {
	Email             string `json:"email"`
	PreferredUsername string `json:"preferred_username"`
	Name              string `json:"name"`
	GivenName         string `json:"given_name"`
	FamilyName        string `json:"family_name"`
	EmailVerified     bool   `json:"email_verified"`
}

func (c *oidcClaims) merge(o oidcClaims) {
	if c.Email == "" {
		c.Email, c.EmailVerified = o.Email, o.EmailVerified
	}

	if c.PreferredUsername == "" {
		c.PreferredUsername = o.PreferredUsername
	}

	if c.Name == "" {
		c.Name = o.Name
	}

	if c.GivenName == "" {
		c.GivenName = o.GivenName
	}

	if c.FamilyName == "" {
		c.FamilyName = o.FamilyName
	}
}

// mapOIDCClaims turns verified claims into a UserInfo, with the per-vendor
// quirks in one place.
func mapOIDCClaims(idp *domain.IDP, subject string, c oidcClaims) (*domain.UserInfo, error) {
	if subject == "" {
		return nil, &domain.InvalidAuthnServiceError{Message: "the identity provider did not return a valid identity"}
	}

	email, verified := c.Email, c.EmailVerified

	switch domain.IDPTypeName(idp.IDPType.Name) {
	case domain.IDPTypeNameEntraID:
		// No email_verified claim exists. The row pins ONE tenant through its
		// issuer, so the address is the directory's own attribute for a member
		// of that tenant, set by its administrators: verified by construction.
		// preferred_username is the UPN, which is an address in most tenants
		// and is the fallback when the optional email claim is not configured.
		if email == "" && isEmail(c.PreferredUsername) {
			email = c.PreferredUsername
		}

		verified = email != ""
	default:
		// Google and Okta assert it; anything else is taken at its word too.
	}

	if email == "" {
		return nil, &domain.InvalidAuthnServiceError{Message: "the identity provider did not return an email address"}
	}

	first, last := c.GivenName, c.FamilyName
	if first == "" && last == "" {
		first, last = splitName(c.Name, email)
	}

	return &domain.UserInfo{
		Subject:       subject,
		Email:         strings.ToLower(email),
		EmailVerified: verified,
		FirstName:     first,
		LastName:      last,
	}, nil
}

// identityFromGithub reads /user for the identity and /user/emails for the
// primary verified address.
func (p *Provider) identityFromGithub(ctx context.Context, idp *domain.IDP, cfg *oauth2.Config, token *oauth2.Token) (*domain.UserInfo, error) {
	client := cfg.Client(ctx, token)

	userURL := idp.IDPType.UserInfoAPIURL
	if userURL == "" {
		userURL = "https://api.github.com/user"
	}

	var user struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	if err := getJSON(ctx, client, userURL, &user); err != nil {
		slog.Warn("oauthidp: could not read the account details from the identity provider", "idp", idp.Name, "error", err)

		return nil, &domain.InvalidAuthnServiceError{Message: "the identity provider did not return the account details"}
	}

	if user.ID == 0 {
		return nil, &domain.InvalidAuthnServiceError{Message: "the identity provider did not return a valid identity"}
	}

	// /user.email is null for anyone whose email is private, which is most
	// people. /user/emails lists every address with `primary` and `verified`;
	// only a verified primary is an address this service will act on.
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}

	email, verified := "", false

	if err := getJSON(ctx, client, strings.TrimSuffix(userURL, "/")+"/emails", &emails); err != nil {
		slog.Warn("oauthidp: could not read the email addresses from the identity provider", "idp", idp.Name, "error", err)
	} else {
		for _, e := range emails {
			if e.Primary {
				email, verified = e.Email, e.Verified

				break
			}
		}
	}

	if email == "" && user.Email != "" {
		// The public profile email, whose verification GitHub does not state.
		email, verified = user.Email, false
	}

	if email == "" {
		return nil, &domain.InvalidAuthnServiceError{Message: "the identity provider did not return an email address"}
	}

	first, last := splitName(user.Name, user.Login)

	return &domain.UserInfo{
		// The numeric id, which survives a rename; the login does not.
		Subject:       strconv.FormatInt(user.ID, 10),
		Email:         strings.ToLower(email),
		EmailVerified: verified,
		FirstName:     first,
		LastName:      last,
	}, nil
}

// splitName makes a first and last name out of whatever the provider gave.
// A single word is the first name and the last is the local part of the
// fallback, so a person called "Cher" or with a private name can still be
// registered; it used to be an error.
func splitName(name, fallback string) (string, string) {
	name = strings.TrimSpace(name)

	if name == "" {
		name = fallback
		if at := strings.Index(name, "@"); at > 0 {
			name = name[:at]
		}
	}

	first, last, found := strings.Cut(name, " ")
	if !found || strings.TrimSpace(last) == "" {
		return first, first
	}

	return first, strings.TrimSpace(last)
}

func withOpenID(scopes []string) []string {
	if slices.Contains(scopes, oidc.ScopeOpenID) {
		return scopes
	}

	return append([]string{oidc.ScopeOpenID}, scopes...)
}

func isEmail(s string) bool {
	if s == "" {
		return false
	}

	_, err := mail.ParseAddress(s)

	return err == nil
}

// userInfoAPITimeout caps one call to a github endpoint, on top of the
// client's own timeout.
const userInfoAPITimeout = 10 * time.Second

func getJSON(ctx context.Context, client *http.Client, apiURL string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, userInfoAPITimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}
