package payload

import (
	"time"

	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// TokenLifetimeRange is a min/max pair for one lifetime.
//
//	@Description	The inclusive bounds a lifetime is validated against
type TokenLifetimeRange struct {
	Min string `json:"min" example:"2m0s" format:"string"`    // Shortest accepted value, as a Go duration
	Max string `json:"max" example:"48h0m0s" format:"string"` // Longest accepted value, as a Go duration
}

// TokenLifetimeBoundsResponse carries the bounds for both lifetimes.
//
//	@Description	The bounds every change is validated against, so a client never has to hardcode them
type TokenLifetimeBoundsResponse struct {
	AccessTokenDuration  TokenLifetimeRange `json:"access_token_duration"`  // Bounds on the access token lifetime
	RefreshTokenDuration TokenLifetimeRange `json:"refresh_token_duration"` // Bounds on the refresh token lifetime
}

// TokenLifetimeDefaultsResponse carries the values a fresh database is seeded with.
//
//	@Description	The shipped defaults; "reset to defaults" is a PUT of exactly these
type TokenLifetimeDefaultsResponse struct {
	AccessTokenDuration  string `json:"access_token_duration" example:"5m0s" format:"string"`     // Seeded access token lifetime
	RefreshTokenDuration string `json:"refresh_token_duration" example:"24h0m0s" format:"string"` // Seeded refresh token lifetime
}

// TokenLifetimesResponse is what GET and PUT /auth/token_lifetimes answer with.
//
//	@Description	The lifetimes tokens issued from now on will carry, the bounds a change is validated against, the shipped defaults, and who last changed them
type TokenLifetimesResponse struct {
	UpdatedAt            time.Time                     `json:"updated_at" example:"2026-09-05T10:12:00Z" format:"date-time"`                      // When the row was last changed
	UpdatedBy            *uuid.UUID                    `json:"updated_by,omitempty" example:"019b4b0d-a682-7e34-a20c-c71a7147d7e7" format:"uuid"` // Who last changed it; absent for the seeded values
	AccessTokenDuration  string                        `json:"access_token_duration" example:"5m0s" format:"string"`                              // Lifetime of access tokens issued from now on, as a Go duration
	RefreshTokenDuration string                        `json:"refresh_token_duration" example:"24h0m0s" format:"string"`                          // Lifetime of refresh tokens issued at the next login, as a Go duration
	Bounds               TokenLifetimeBoundsResponse   `json:"bounds"`                                                                            // What a change is validated against
	Defaults             TokenLifetimeDefaultsResponse `json:"defaults"`                                                                          // The shipped values
}

// UpdateTokenLifetimesRequest is the body of PUT /auth/token_lifetimes.
//
//	@Description	Both lifetimes, as Go duration strings. The refresh lifetime must be strictly longer than the access lifetime
type UpdateTokenLifetimesRequest struct {
	AccessTokenDuration  string `json:"access_token_duration" example:"10m0s" format:"string"`    // 2m to 48h
	RefreshTokenDuration string `json:"refresh_token_duration" example:"72h0m0s" format:"string"` // 12h to 168h, and longer than the access token
}

// ToTokenLifetimesResponse renders the domain value with its bounds and defaults.
func ToTokenLifetimesResponse(in *domain.TokenLifetimes) TokenLifetimesResponse {
	bounds := domain.TokenLifetimesBounds()
	defaults := domain.DefaultTokenLifetimes()

	out := TokenLifetimesResponse{
		UpdatedAt:            in.UpdatedAt,
		AccessTokenDuration:  in.AccessTokenDuration.String(),
		RefreshTokenDuration: in.RefreshTokenDuration.String(),
		Bounds: TokenLifetimeBoundsResponse{
			AccessTokenDuration:  TokenLifetimeRange{Min: bounds.AccessTokenMin.String(), Max: bounds.AccessTokenMax.String()},
			RefreshTokenDuration: TokenLifetimeRange{Min: bounds.RefreshTokenMin.String(), Max: bounds.RefreshTokenMax.String()},
		},
		Defaults: TokenLifetimeDefaultsResponse{
			AccessTokenDuration:  defaults.AccessTokenDuration.String(),
			RefreshTokenDuration: defaults.RefreshTokenDuration.String(),
		},
	}

	if in.UpdatedBy != uuid.Nil() {
		out.UpdatedBy = new(in.UpdatedBy)
	}

	return out
}

// ToUpdateTokenLifetimesInput parses the request. UpdatedBy is the caller's
// job: it comes from the verified token, never from the body.
//
// The parse error is this package's own wording. time.ParseDuration's text
// would otherwise become part of the API contract, which is the rule the uuid
// migration wrote down.
func ToUpdateTokenLifetimesInput(in UpdateTokenLifetimesRequest) (*domain.UpdateTokenLifetimesInput, error) {
	var errs domain.ValidationErrors

	access, err := time.ParseDuration(in.AccessTokenDuration)
	if err != nil {
		errs.AddError(domain.FieldAccessTokenDuration,
			"must be a duration such as 5m, 30m0s or 2h0m0s: "+in.AccessTokenDuration+" is not one", "INVALID_FORMAT")
	}

	refresh, err := time.ParseDuration(in.RefreshTokenDuration)
	if err != nil {
		errs.AddError(domain.FieldRefreshTokenDuration,
			"must be a duration such as 24h, 72h0m0s or 168h0m0s: "+in.RefreshTokenDuration+" is not one", "INVALID_FORMAT")
	}

	if errs.HasErrors() {
		return nil, &errs
	}

	return &domain.UpdateTokenLifetimesInput{
		AccessTokenDuration:  access,
		RefreshTokenDuration: refresh,
	}, nil
}
