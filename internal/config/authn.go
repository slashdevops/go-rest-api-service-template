package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

const (
	// DefaultAuthnIssuer is the default issuer of the JWT tokens
	DefaultAuthnIssuer = "https://goapitemplate.local"

	// DefaultAuthnTokenLifetimesReloadInterval is how often each replica
	// re-reads the authn_token_lifetimes row, and therefore the worst case for
	// how long a change made on ANOTHER replica takes to reach this one when
	// the Valkey change signal is lost or absent. A change made on this
	// replica applies before its PUT answers, and with a cache the signal
	// reaches the others in under a second, so this is a floor, not the
	// mechanism.
	//
	// The two token lifetimes themselves are NOT settings any more. They used
	// to be authn.access.token.duration and authn.refresh.token.duration; they
	// live in the database now, seeded by migration and edited through
	// PUT /auth/token_lifetimes, so a change is not a redeploy and the
	// ordering rule (refresh strictly longer than access) is enforced.
	DefaultAuthnTokenLifetimesReloadInterval = time.Minute

	// DefaultAuthnRefreshTokenRotationEnabled makes every refresh spend the
	// token it consumed and issue a new one in its place, so a refresh token is
	// a one-use credential rather than a bearer credential good for its whole
	// life.
	//
	// It is on by default because the alternative is worse in both directions:
	// without rotation a stolen refresh token is usable, undetectably, until it
	// expires. The switch exists because rotation is a two-repo contract — a
	// client that discards the new token will be locked out on its next refresh
	// — so an operator can turn it off without redeploying the API.
	DefaultAuthnRefreshTokenRotationEnabled = true

	// DefaultAuthnRefreshTokenRotationGrace is how long the token a rotation
	// consumed still answers, returning the successor it already issued.
	//
	// It exists because a lost response is not a theft. A dropped answer, a
	// client that crashed before storing the new token, two tabs refreshing at
	// once: all present a token that was already rotated, and all are ordinary.
	// Without this window the reuse alarm would fire on them constantly, and an
	// alarm that fires constantly is one nobody can act on.
	//
	// The window is the trade: inside it, a stolen token is accepted alongside
	// the real one. Thirty seconds is far longer than a retry needs and far
	// shorter than a theft takes to notice. Set it to zero for strict
	// detection, and expect occasional spurious logouts.
	DefaultAuthnRefreshTokenRotationGrace = 30 * time.Second

	// DefaultAuthnRevokedTokensSweepInterval is how often expired rows are
	// removed from the denylist.
	//
	// Rotation is what makes this necessary rather than tidy: the table takes a
	// row per refresh instead of a row per logout, so with nothing sweeping it
	// grows without bound. A row is dead the moment the token it names would
	// have expired anyway — past that the token is refused for being expired,
	// not for being revoked — so the sweep never changes an answer.
	DefaultAuthnRevokedTokensSweepInterval = time.Hour

	// DefaultAuthnAccessTokenRevocationEnabled makes a logout end the access
	// token it was called with, not just the refresh token.
	//
	// On by default because the alternative is a window: without it, logging
	// out revokes the refresh token — the session cannot be extended — but the
	// access token in the caller's hand keeps authenticating until it expires
	// on its own. That is the standard JWT trade and it is why the access
	// token's lifetime is short, but it is a residue that can be closed for
	// the cost of an in-memory set and one query every
	// DefaultAuthnAccessTokenRevocationReloadInterval.
	//
	// Turning it off restores the previous behaviour exactly. It does not turn
	// off refresh-token revocation, which has never been optional.
	DefaultAuthnAccessTokenRevocationEnabled = true

	// DefaultAuthnAccessTokenRevocationReloadInterval is how often each replica
	// rebuilds its revoked-access-token set from the store, and therefore the
	// worst case for how long a revocation made on ANOTHER replica is still
	// honoured here. A revocation made on this replica is immediate.
	//
	// Ten seconds against a five-minute access token leaves a residue of about
	// 3% of an already-small window. Lowering it costs one indexed range scan
	// per replica per interval; the set itself is bounded by logouts in the
	// last access-token lifetime, not by traffic.
	DefaultAuthnAccessTokenRevocationReloadInterval = 10 * time.Second

	// DefaultAuthnLoginThrottleEnabled bounds password guessing per account,
	// independently of the per-IP limiter. The IP limiter does nothing about
	// guesses spread across many addresses, which is the shape a real attack
	// takes.
	DefaultAuthnLoginThrottleEnabled = true

	// DefaultAuthnSeedAdminPasswordAllowed is false: a service whose seeded
	// administrator still has the password written in the migration comment
	// refuses to start. The dev stack sets it to true. A warning would have
	// been the posture that ships.
	DefaultAuthnSeedAdminPasswordAllowed = false

	// DefaultAuthnPasswordBcryptCost is 12. It was the library default, 10,
	// chosen in 1999; each step doubles the work of an offline guess and of
	// one login. 12 is about a quarter of a second.
	DefaultAuthnPasswordBcryptCost  = 12
	ValidAuthnMinPasswordBcryptCost = 10
	ValidAuthnMaxPasswordBcryptCost = 14

	// DefaultAuthnLoginThrottleMaxAttempts is how many failures in a row an
	// account tolerates before it starts refusing. Five is enough for a person
	// who is mistyping, and a success resets it, so a legitimate user does not
	// accumulate toward the ceiling.
	DefaultAuthnLoginThrottleMaxAttempts = 5

	// DefaultAuthnLoginThrottleWindow is how long a fully spent budget takes to
	// refill. Tokens come back steadily rather than all at once, so a refused
	// account recovers one attempt every Window/MaxAttempts — three minutes at
	// the defaults.
	//
	// This delays, it does not lock: anyone who knows an address can spend its
	// budget, and a ceiling that recovers on its own is what stops that being
	// an account someone can keep shut.
	DefaultAuthnLoginThrottleWindow = 15 * time.Minute

	// DefaultAuthnLoginThrottleIdleAfter is how long an untouched account is
	// kept before its record is evicted. Evicting an idle key is the same thing
	// as resetting it.
	DefaultAuthnLoginThrottleIdleAfter = 30 * time.Minute
)

var (
	DefaultAuthnPrivateKeyFile           = FileVar{os.NewFile(0, "jwt.key"), os.O_RDONLY}
	DefaultAuthnPublicKeyFile            = FileVar{os.NewFile(0, "jwt.pub"), os.O_RDONLY}
	DefaultAuthnSymmetricKeyFile         = FileVar{os.NewFile(0, "aes-256-symmetric.key"), os.O_RDONLY}
	DefaultAuthnUserVerificationTokenTTL = 24 * time.Hour
	// DefaultAuthnUserVerificationWebEndpoint is the page a verification email
	// links to. It points at the FRONTEND, not at this API, so that the token
	// arrives as something the page hands over in a header rather than as a URL
	// this service writes to its request log. The password-reset flow has always
	// worked this way; verification did not.
	DefaultAuthnUserVerificationWebEndpoint = "http://localhost:5173/verify"
	DefaultAuthnUserResetPasswordEndpoint   = "http://localhost:5173/reset-password"
	DefaultAuthnUserResetPasswordTokenTTL   = 15 * time.Minute
)

type AuthnConfig struct {
	PrivateKeyFile                      Field[FileVar]
	PublicKeyFile                       Field[FileVar]
	AdditionalPublicKeyFiles            Field[string]
	SymmetricKeyFile                    Field[FileVar]
	Issuer                              Field[string]
	LoginThrottleMaxAttempts            Field[int]
	LoginThrottleWindow                 Field[time.Duration]
	LoginThrottleIdleAfter              Field[time.Duration]
	LoginThrottleEnabled                Field[bool]
	SeedAdminPasswordAllowed            Field[bool]
	PasswordBcryptCost                  Field[int]
	TokenLifetimesReloadInterval        Field[time.Duration]
	RefreshTokenRotationGrace           Field[time.Duration]
	RefreshTokenRotationEnabled         Field[bool]
	RevokedTokensSweepInterval          Field[time.Duration]
	AccessTokenRevocationEnabled        Field[bool]
	AccessTokenRevocationReloadInterval Field[time.Duration]
	UserVerificationWebEndpoint         Field[string]
	UserVerificationTokenTTL            Field[time.Duration]
	UserResetPasswordEndpoint           Field[string]
	UserResetPasswordTokenTTL           Field[time.Duration]
}

func NewAuthConfig() *AuthnConfig {
	return &AuthnConfig{
		PrivateKeyFile:           NewField("authn.private.key.file", "AUTHN_PRIVATE_KEY_FILE", "Auth Private Key File used to sign the JWT tokens. Using Elliptic Curve keys (prime256v1)", DefaultAuthnPrivateKeyFile),
		PublicKeyFile:            NewField("authn.public.key.file", "AUTHN_PUBLIC_KEY_FILE", "Auth Public Key File used to verify the JWT tokens", DefaultAuthnPublicKeyFile),
		AdditionalPublicKeyFiles: NewField("authn.additional.public.key.files", "AUTHN_ADDITIONAL_PUBLIC_KEY_FILES", "Comma-separated PEM files whose keys may verify tokens but never sign them. This is how a signing key is rotated without downtime: a new key verifies before it signs, an old key verifies after it stops", ""),
		SymmetricKeyFile:         NewField("authn.symmetric.key.file", "AUTHN_SYMMETRIC_KEY_FILE", "Auth Symmetric Key File used to encrypt/decrypt Application tokens and API tokens", DefaultAuthnSymmetricKeyFile),
		Issuer:                   NewField("authn.issuer", "AUTHN_ISSUER", "Issuer of the JWT tokens", DefaultAuthnIssuer),
		PasswordBcryptCost:       NewField("authn.password.bcrypt.cost", "AUTHN_PASSWORD_BCRYPT_COST", "bcrypt cost for new password hashes; each step doubles the work of a guess and of a login", DefaultAuthnPasswordBcryptCost),
		SeedAdminPasswordAllowed: NewField("authn.seed.admin.password.allowed", "AUTHN_SEED_ADMIN_PASSWORD_ALLOWED", "Start even though the seeded administrator still has the seeded password. For development only", DefaultAuthnSeedAdminPasswordAllowed),
		LoginThrottleEnabled:     NewField("authn.login.throttle.enabled", "AUTHN_LOGIN_THROTTLE_ENABLED", "Bound failed login attempts per account, independently of the per-IP rate limiter", DefaultAuthnLoginThrottleEnabled),
		LoginThrottleMaxAttempts: NewField("authn.login.throttle.max.attempts", "AUTHN_LOGIN_THROTTLE_MAX_ATTEMPTS", "Consecutive failed logins an account tolerates before it is refused. A successful login resets it", DefaultAuthnLoginThrottleMaxAttempts),
		LoginThrottleWindow:      NewField("authn.login.throttle.window", "AUTHN_LOGIN_THROTTLE_WINDOW", "How long a fully spent login budget takes to refill; a refused account recovers one attempt every window/max-attempts", DefaultAuthnLoginThrottleWindow),
		LoginThrottleIdleAfter:   NewField("authn.login.throttle.idle.after", "AUTHN_LOGIN_THROTTLE_IDLE_AFTER", "How long an untouched account keeps its throttle record before it is evicted", DefaultAuthnLoginThrottleIdleAfter),

		TokenLifetimesReloadInterval:        NewField("authn.token.lifetimes.reload.interval", "AUTHN_TOKEN_LIFETIMES_RELOAD_INTERVAL", "How often each replica re-reads the access and refresh token lifetimes from the database. The lifetimes themselves are not flags: they are edited through PUT /auth/token_lifetimes and a change reaches other replicas at once through the cache, or within this interval without one", DefaultAuthnTokenLifetimesReloadInterval),
		RefreshTokenRotationEnabled:         NewField("authn.refresh.token.rotation.enabled", "AUTHN_REFRESH_TOKEN_ROTATION_ENABLED", "Issue a new refresh token on every refresh and revoke the one it replaced; a replayed token ends the whole session", DefaultAuthnRefreshTokenRotationEnabled),
		RefreshTokenRotationGrace:           NewField("authn.refresh.token.rotation.grace", "AUTHN_REFRESH_TOKEN_ROTATION_GRACE", "How long a just-rotated refresh token still answers with the successor it issued, so a lost response is a retry and not a detected replay", DefaultAuthnRefreshTokenRotationGrace),
		RevokedTokensSweepInterval:          NewField("authn.revoked.tokens.sweep.interval", "AUTHN_REVOKED_TOKENS_SWEEP_INTERVAL", "How often expired rows are deleted from the token denylist; zero disables the sweep", DefaultAuthnRevokedTokensSweepInterval),
		AccessTokenRevocationEnabled:        NewField("authn.access.token.revocation.enabled", "AUTHN_ACCESS_TOKEN_REVOCATION_ENABLED", "Make a logout end the access token it was called with, not only the refresh token. Off restores the previous behaviour: the access token keeps working until it expires", DefaultAuthnAccessTokenRevocationEnabled),
		AccessTokenRevocationReloadInterval: NewField("authn.access.token.revocation.reload.interval", "AUTHN_ACCESS_TOKEN_REVOCATION_RELOAD_INTERVAL", "How often each replica rebuilds its revoked-access-token set from the store. This is the worst case for how long a revocation made on another replica is still honoured here", DefaultAuthnAccessTokenRevocationReloadInterval),
		UserVerificationWebEndpoint:         NewField("authn.user.verification.web.endpoint", "AUTHN_USER_VERIFICATION_WEB_ENDPOINT", "Page a verification email links to; it receives the token as a query parameter and hands it to the API in a header", DefaultAuthnUserVerificationWebEndpoint),
		UserVerificationTokenTTL:            NewField("authn.user.verification.token.ttl", "AUTHN_USER_VERIFICATION_TOKEN_TTL", "User Verification Token TTL", DefaultAuthnUserVerificationTokenTTL),
		UserResetPasswordEndpoint:           NewField("authn.user.reset.password.api.endpoint", "AUTHN_USER_RESET_PASSWORD_API_ENDPOINT", "User Reset Password API Endpoint", DefaultAuthnUserResetPasswordEndpoint),
		UserResetPasswordTokenTTL:           NewField("authn.user.reset.password.token.ttl", "AUTHN_USER_RESET_PASSWORD_TOKEN_TTL", "User Reset Password Token TTL", DefaultAuthnUserResetPasswordTokenTTL),
	}
}

// ParseEnvVars reads the server configuration from environment variables
// and sets the values in the configuration
func (ref *AuthnConfig) ParseEnvVars() {
	ref.PrivateKeyFile.Value = GetEnv(ref.PrivateKeyFile.EnVarName, ref.PrivateKeyFile.Value)
	ref.PublicKeyFile.Value = GetEnv(ref.PublicKeyFile.EnVarName, ref.PublicKeyFile.Value)
	ref.AdditionalPublicKeyFiles.Value = GetEnv(ref.AdditionalPublicKeyFiles.EnVarName, ref.AdditionalPublicKeyFiles.Value)
	ref.SymmetricKeyFile.Value = GetEnv(ref.SymmetricKeyFile.EnVarName, ref.SymmetricKeyFile.Value)
	ref.Issuer.Value = GetEnv(ref.Issuer.EnVarName, ref.Issuer.Value)
	ref.LoginThrottleEnabled.Value = GetEnv(ref.LoginThrottleEnabled.EnVarName, ref.LoginThrottleEnabled.Value)
	ref.SeedAdminPasswordAllowed.Value = GetEnv(ref.SeedAdminPasswordAllowed.EnVarName, ref.SeedAdminPasswordAllowed.Value)
	ref.PasswordBcryptCost.Value = GetEnv(ref.PasswordBcryptCost.EnVarName, ref.PasswordBcryptCost.Value)
	ref.LoginThrottleMaxAttempts.Value = GetEnv(ref.LoginThrottleMaxAttempts.EnVarName, ref.LoginThrottleMaxAttempts.Value)
	ref.LoginThrottleWindow.Value = GetEnv(ref.LoginThrottleWindow.EnVarName, ref.LoginThrottleWindow.Value)
	ref.LoginThrottleIdleAfter.Value = GetEnv(ref.LoginThrottleIdleAfter.EnVarName, ref.LoginThrottleIdleAfter.Value)
	ref.TokenLifetimesReloadInterval.Value = GetEnv(ref.TokenLifetimesReloadInterval.EnVarName, ref.TokenLifetimesReloadInterval.Value)
	ref.RefreshTokenRotationEnabled.Value = GetEnv(ref.RefreshTokenRotationEnabled.EnVarName, ref.RefreshTokenRotationEnabled.Value)
	ref.RefreshTokenRotationGrace.Value = GetEnv(ref.RefreshTokenRotationGrace.EnVarName, ref.RefreshTokenRotationGrace.Value)
	ref.RevokedTokensSweepInterval.Value = GetEnv(ref.RevokedTokensSweepInterval.EnVarName, ref.RevokedTokensSweepInterval.Value)
	ref.AccessTokenRevocationEnabled.Value = GetEnv(ref.AccessTokenRevocationEnabled.EnVarName, ref.AccessTokenRevocationEnabled.Value)
	ref.AccessTokenRevocationReloadInterval.Value = GetEnv(ref.AccessTokenRevocationReloadInterval.EnVarName, ref.AccessTokenRevocationReloadInterval.Value)
	ref.UserVerificationWebEndpoint.Value = GetEnv(ref.UserVerificationWebEndpoint.EnVarName, ref.UserVerificationWebEndpoint.Value)
	ref.UserVerificationTokenTTL.Value = GetEnv(ref.UserVerificationTokenTTL.EnVarName, ref.UserVerificationTokenTTL.Value)
	ref.UserResetPasswordEndpoint.Value = GetEnv(ref.UserResetPasswordEndpoint.EnVarName, ref.UserResetPasswordEndpoint.Value)
	ref.UserResetPasswordTokenTTL.Value = GetEnv(ref.UserResetPasswordTokenTTL.EnVarName, ref.UserResetPasswordTokenTTL.Value)
}

func (ref *AuthnConfig) Validate() error {
	if ref.PasswordBcryptCost.Value < ValidAuthnMinPasswordBcryptCost || ref.PasswordBcryptCost.Value > ValidAuthnMaxPasswordBcryptCost {
		return &InvalidConfigurationError{
			Field:   "authn.password.bcrypt.cost",
			Value:   fmt.Sprintf("%d", ref.PasswordBcryptCost.Value),
			Message: fmt.Sprintf("invalid authn.password.bcrypt.cost, must be between %d and %d", ValidAuthnMinPasswordBcryptCost, ValidAuthnMaxPasswordBcryptCost),
		}
	}

	if len(ref.PrivateKeyFile.Value.Name()) <= domain.ValidAuthnKeyFilePathMinLength || len(ref.PrivateKeyFile.Value.Name()) > domain.ValidAuthnKeyFilePathMaxLength {
		return &InvalidConfigurationError{
			Field:   "authn.private.key.file",
			Value:   ref.PrivateKeyFile.Value.Name(),
			Message: fmt.Sprintf("invalid private key file, must be between %d and %d characters", domain.ValidAuthnKeyFilePathMinLength, domain.ValidAuthnKeyFilePathMaxLength),
		}
	}

	if len(ref.PublicKeyFile.Value.Name()) <= domain.ValidAuthnKeyFilePathMinLength || len(ref.PublicKeyFile.Value.Name()) > domain.ValidAuthnKeyFilePathMaxLength {
		return &InvalidConfigurationError{
			Field:   "authn.public.key.file",
			Value:   ref.PublicKeyFile.Value.Name(),
			Message: fmt.Sprintf("invalid public key file, must be between %d and %d characters", domain.ValidAuthnKeyFilePathMinLength, domain.ValidAuthnKeyFilePathMaxLength),
		}
	}

	if len(ref.SymmetricKeyFile.Value.Name()) <= domain.ValidAuthnKeyFilePathMinLength || len(ref.SymmetricKeyFile.Value.Name()) > domain.ValidAuthnKeyFilePathMaxLength {
		return &InvalidConfigurationError{
			Field:   "authn.symmetric.key.file",
			Value:   ref.SymmetricKeyFile.Value.Name(),
			Message: fmt.Sprintf("invalid symmetric key file, must be between %d and %d characters", domain.ValidAuthnKeyFilePathMinLength, domain.ValidAuthnKeyFilePathMaxLength),
		}
	}

	if ref.Issuer.Value == "" || len(ref.Issuer.Value) < domain.ValidAuthnIssuerMinLength || len(ref.Issuer.Value) > domain.ValidAuthnIssuerMaxLength {
		return &InvalidConfigurationError{
			Field:   "authn.issuer",
			Value:   ref.Issuer.Value,
			Message: fmt.Sprintf("invalid issuer, must be between %d and %d characters", domain.ValidAuthnIssuerMinLength, domain.ValidAuthnIssuerMaxLength),
		}
	}

	// Zero would be a ticker that panics; the mirror has no "off" -- a serving
	// replica always has lifetimes, and this is only how often it refreshes
	// them.
	if ref.TokenLifetimesReloadInterval.Value <= 0 {
		return &InvalidConfigurationError{
			Field:   "authn.token.lifetimes.reload.interval",
			Value:   ref.TokenLifetimesReloadInterval.Value.String(),
			Message: "invalid authn.token.lifetimes.reload.interval, must be positive",
		}
	}

	// A negative grace is not the same as zero: zero is strict detection, which
	// is a legitimate posture, while a negative value reads as one and is
	// almost certainly a typo.
	if ref.RefreshTokenRotationGrace.Value < 0 {
		return &InvalidConfigurationError{
			Field:   "authn.refresh.token.rotation.grace",
			Value:   ref.RefreshTokenRotationGrace.Value.String(),
			Message: "invalid authn.refresh.token.rotation.grace, must not be negative; use zero for strict reuse detection",
		}
	}

	// The grace window is a period during which a spent token still answers, so
	// it has to stay small relative to the life of the token it applies to. The
	// lifetime is a runtime setting, so the strongest statement a startup check
	// can make is against the SHORTEST refresh token an operator may configure.
	if ref.RefreshTokenRotationGrace.Value >= domain.ValidAuthnRefreshTokenMinDuration {
		return &InvalidConfigurationError{
			Field:   "authn.refresh.token.rotation.grace",
			Value:   ref.RefreshTokenRotationGrace.Value.String(),
			Message: fmt.Sprintf("invalid authn.refresh.token.rotation.grace, must be shorter than the minimum refresh token lifetime (%s); a grace as long as the token disables reuse detection entirely", domain.ValidAuthnRefreshTokenMinDuration),
		}
	}

	if ref.RevokedTokensSweepInterval.Value < 0 {
		return &InvalidConfigurationError{
			Field:   "authn.revoked.tokens.sweep.interval",
			Value:   ref.RevokedTokensSweepInterval.Value.String(),
			Message: "invalid authn.revoked.tokens.sweep.interval, must not be negative; use zero to disable the sweep",
		}
	}

	// Zero is not "disabled" here, it is a ticker that panics. The switch to
	// turn this off is authn.access.token.revocation.enabled.
	if ref.AccessTokenRevocationEnabled.Value && ref.AccessTokenRevocationReloadInterval.Value <= 0 {
		return &InvalidConfigurationError{
			Field:   "authn.access.token.revocation.reload.interval",
			Value:   ref.AccessTokenRevocationReloadInterval.Value.String(),
			Message: "invalid authn.access.token.revocation.reload.interval, must be positive; to switch the check off use authn.access.token.revocation.enabled=false",
		}
	}

	// A reload slower than the tokens it tracks is a mirror that is stale more
	// often than it is fresh: a revocation made elsewhere would be honoured
	// here for longer than the token it revokes would have lived anyway, which
	// is the same as not having the check. The lifetime is a runtime setting,
	// so this is checked against the SHORTEST access token an operator may
	// configure; the Prometheus rule compares the live values.
	if ref.AccessTokenRevocationEnabled.Value && ref.AccessTokenRevocationReloadInterval.Value >= domain.ValidAuthnAccessTokenMinDuration {
		return &InvalidConfigurationError{
			Field:   "authn.access.token.revocation.reload.interval",
			Value:   ref.AccessTokenRevocationReloadInterval.Value.String(),
			Message: fmt.Sprintf("invalid authn.access.token.revocation.reload.interval, must be shorter than the minimum access token lifetime (%s); a reload slower than the token it tracks leaves the check doing nothing for most of the token's life", domain.ValidAuthnAccessTokenMinDuration),
		}
	}

	if _, err := url.Parse(ref.UserVerificationWebEndpoint.Value); err != nil {
		return &InvalidConfigurationError{
			Field:   "authn.user.verification.web.endpoint",
			Value:   ref.UserVerificationWebEndpoint.Value,
			Message: "invalid user verification web endpoint",
		}
	}

	if ref.UserVerificationTokenTTL.Value < domain.ValidAuthnMinUserVerificationTokenTTL || ref.UserVerificationTokenTTL.Value > domain.ValidAuthnMaxUserVerificationTokenTTL {
		return &InvalidConfigurationError{
			Field:   "authn.user.verification.token.ttl",
			Value:   fmt.Sprintf("%d", ref.UserVerificationTokenTTL.Value),
			Message: fmt.Sprintf("invalid user verification token TTL, must be between %d and %d", domain.ValidAuthnMinUserVerificationTokenTTL, domain.ValidAuthnMaxUserVerificationTokenTTL),
		}
	}

	if _, err := url.Parse(ref.UserResetPasswordEndpoint.Value); err != nil {
		return &InvalidConfigurationError{
			Field:   "authn.user.reset.password.api.endpoint",
			Value:   ref.UserResetPasswordEndpoint.Value,
			Message: "invalid user reset password API endpoint",
		}
	}

	if ref.UserResetPasswordTokenTTL.Value < domain.ValidAuthnMinUserResetPasswordTokenTTL || ref.UserResetPasswordTokenTTL.Value > domain.ValidAuthnMaxUserResetPasswordTokenTTL {
		return &InvalidConfigurationError{
			Field:   "authn.user.reset.password.token.ttl",
			Value:   fmt.Sprintf("%d", ref.UserResetPasswordTokenTTL.Value),
			Message: fmt.Sprintf("invalid user reset password token TTL, must be between %d and %d", domain.ValidAuthnMinUserResetPasswordTokenTTL, domain.ValidAuthnMaxUserResetPasswordTokenTTL),
		}
	}

	if ref.LoginThrottleEnabled.Value {
		if ref.LoginThrottleMaxAttempts.Value < 1 {
			return &InvalidConfigurationError{
				Field:   "authn.login.throttle.max.attempts",
				Value:   fmt.Sprintf("%d", ref.LoginThrottleMaxAttempts.Value),
				Message: "invalid authn.login.throttle.max.attempts, must be at least 1; disable the throttle instead of setting it to zero",
			}
		}

		if ref.LoginThrottleWindow.Value <= 0 {
			return &InvalidConfigurationError{
				Field:   "authn.login.throttle.window",
				Value:   ref.LoginThrottleWindow.Value.String(),
				Message: "invalid authn.login.throttle.window, must be greater than zero; it is the period a spent budget refills over",
			}
		}

		if ref.LoginThrottleIdleAfter.Value <= 0 {
			return &InvalidConfigurationError{
				Field:   "authn.login.throttle.idle.after",
				Value:   ref.LoginThrottleIdleAfter.Value.String(),
				Message: "invalid authn.login.throttle.idle.after, must be greater than zero",
			}
		}
	}

	return nil
}

// AdditionalPublicKeyFilesList splits the configured value into paths, dropping
// blanks so that a trailing comma or a value padded for readability is not an
// error. Empty is the ordinary case: no rotation is in progress.
func (ref *AuthnConfig) AdditionalPublicKeyFilesList() []string {
	paths := make([]string, 0)

	for raw := range strings.SplitSeq(ref.AdditionalPublicKeyFiles.Value, ",") {
		if path := strings.TrimSpace(raw); path != "" {
			paths = append(paths, path)
		}
	}

	return paths
}
