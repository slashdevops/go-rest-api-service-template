// Package oauthidp is the driven adapter that satisfies the
// oauth.Provider port. It wraps golang.org/x/oauth2 (with the
// per-IDP endpoint configurations from oauth2/google and
// oauth2/github) and the IDP-specific UserInfo HTTP endpoints.
//
// Use-cases under internal/core hand it a *domain.IDP and either ask
// for a login URL or hand back the OAuth callback `code` to be
// exchanged for a UserInfo. All HTTP, JSON, and provider-shape
// knowledge lives here.
package oauthidp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// userInfoAPITimeout caps how long the adapter waits for an IDP's
// UserInfo endpoint to respond.
const userInfoAPITimeout = 5 * time.Second

// Provider implements oauth.Provider.
type Provider struct{}

// New constructs a Provider.
func New() *Provider {
	return &Provider{}
}

// LoginURL implements oauth.Provider.
func (p *Provider) LoginURL(_ context.Context, idp *domain.IDP, state string) (string, error) {
	cfg, err := configFor(idp)
	if err != nil {
		return "", err
	}
	return cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce), nil
}

// Authenticate implements oauth.Provider.
func (p *Provider) Authenticate(ctx context.Context, idp *domain.IDP, code string) (*domain.UserInfo, error) {
	cfg, err := configFor(idp)
	if err != nil {
		return nil, err
	}

	token, err := cfg.Exchange(ctx, code)
	if err != nil {
		// The library's text is logged, never returned. golang.org/x/oauth2
		// reports the provider's own words -- `oauth2: "invalid_client" "The
		// OAuth client was not found."` -- and writing that to the caller both
		// publishes a dependency's wording as this API's contract and tells
		// whoever is probing how our client is registered with the provider.
		slog.Warn("oauthidp: the identity provider refused the authorization code",
			"idp", idp.Name, "error", err)

		return nil, &domain.InvalidAuthnServiceError{Message: "the identity provider did not accept this sign-in"}
	}

	httpClient := cfg.Client(ctx, token)

	raw, err := fetchUserInfo(ctx, httpClient, idp.IDPType.UserInfoAPIURL)
	if err != nil {
		// Same reasoning, and one more: these errors carry the provider's
		// user-info URL and, for a transport failure, net/http's description of
		// our outbound connection.
		slog.Warn("oauthidp: could not read the account details from the identity provider",
			"idp", idp.Name, "error", err)

		return nil, &domain.InvalidAuthnServiceError{Message: "the identity provider did not return the account details"}
	}

	return parseUserInfo(raw, idp.Name)
}

// configFor builds the per-IDP oauth2.Config from the stored IDP
// entity. Validates the required fields are present and the IDP type
// is one we know how to talk to.
func configFor(idp *domain.IDP) (*oauth2.Config, error) {
	if idp == nil {
		return nil, &domain.InvalidIdentityProvidersError{Message: "unknown idp"}
	}
	if idp.ClientID == "" || idp.ClientSecret == "" || idp.CallbackURL == "" {
		return nil, &domain.InvalidIdentityProvidersError{Message: fmt.Sprintf("idp %s is missing required fields", idp.Name)}
	}
	if len(idp.IDPType.Scopes) == 0 {
		return nil, &domain.InvalidIdentityProvidersError{Message: fmt.Sprintf("idp %s is missing scopes", idp.Name)}
	}

	switch idp.IDPType.Name {
	case domain.IDPTypeNameGoogle.String():
		return &oauth2.Config{
			Endpoint:     google.Endpoint,
			ClientID:     idp.ClientID,
			ClientSecret: idp.ClientSecret,
			RedirectURL:  idp.CallbackURL,
			Scopes:       idp.IDPType.Scopes,
		}, nil
	case domain.IDPTypeNameGithub.String():
		return &oauth2.Config{
			Endpoint:     github.Endpoint,
			ClientID:     idp.ClientID,
			ClientSecret: idp.ClientSecret,
			RedirectURL:  idp.CallbackURL,
			Scopes:       idp.IDPType.Scopes,
		}, nil
	default:
		return nil, &domain.InvalidIdentityProvidersError{Message: fmt.Sprintf("unknown idp %s", idp.Name)}
	}
}

func fetchUserInfo(ctx context.Context, httpClient *http.Client, apiURL string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, userInfoAPITimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get user info: %s", resp.Status)
	}

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode user info: %w", err)
	}
	return raw, nil
}

func parseUserInfo(raw map[string]any, idpName string) (*domain.UserInfo, error) {
	switch idpName {
	case domain.IDPTypeNameGoogle.String():
		return parseGoogle(raw)
	case domain.IDPTypeNameGithub.String():
		return parseGithub(raw)
	default:
		return nil, &domain.InvalidIdentityProvidersError{Message: fmt.Sprintf("unknown idp %s", idpName)}
	}
}

func parseGoogle(raw map[string]any) (*domain.UserInfo, error) {
	email, ok := raw["email"].(string)
	if !ok {
		return nil, fmt.Errorf("failed to get email from user info")
	}
	familyName, ok := raw["family_name"].(string)
	if !ok {
		return nil, fmt.Errorf("failed to get family name from user info")
	}
	givenName, ok := raw["given_name"].(string)
	if !ok {
		return nil, fmt.Errorf("failed to get given name from user info")
	}
	return &domain.UserInfo{Email: email, FirstName: givenName, LastName: familyName}, nil
}

func parseGithub(raw map[string]any) (*domain.UserInfo, error) {
	email, ok := raw["email"].(string)
	if !ok {
		return nil, fmt.Errorf("failed to get email from user info")
	}
	name, ok := raw["name"].(string)
	if !ok {
		return nil, fmt.Errorf("failed to get name from user info")
	}
	parts := strings.SplitN(name, " ", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("failed to split name into first and last name")
	}
	return &domain.UserInfo{Email: email, FirstName: parts[0], LastName: parts[1]}, nil
}
