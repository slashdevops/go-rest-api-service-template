package driving

import (
	"context"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// Authn is the driving port consumed by the HTTP authn handler.
type Authn interface {
	LoginUser(ctx context.Context, input *domain.LoginUserInput) (*domain.LoginUserOutput, error)
	LogoutUser(ctx context.Context, input *domain.LogoutUserInput) (*domain.LogoutUserOutput, error)
	RefreshAccessToken(ctx context.Context, input *domain.RefreshAccessTokenInput) (*domain.RefreshAccessTokenOutput, error)
	RegisterUser(ctx context.Context, input *domain.RegisterUserInput) error
	VerifyUser(ctx context.Context, jwtToken string) error
	ReVerifyUser(ctx context.Context, email string) error

	RecoverPassword(ctx context.Context, input *domain.RecoverPasswordInput) error
	ResetPassword(ctx context.Context, input *domain.ResetPasswordInput) error
}
