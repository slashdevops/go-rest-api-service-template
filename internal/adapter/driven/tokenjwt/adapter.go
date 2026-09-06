// Package tokenjwt is the driven adapter that satisfies the token.Signer port
// using ECDSA-signed JWTs (ES256) via github.com/golang-jwt/jwt/v5.
//
// # One verification routine
//
// Everything this service issues is verified here, by [Signer.Verify], and the
// HTTP middleware reaches the same routine through the same port rather than
// carrying a second implementation of its own. There used to be two, and they
// did not agree: one pinned the signing method and the other relied on the
// library rejecting an ECDSA key where HMAC expects bytes; one checked `kid`
// and the other did not; and neither checked `iss` or `aud` even though every
// token carries both. Two verifiers that disagree are not defence in depth —
// the weaker one is the one that decides.
//
// # What a caller is told when verification fails
//
// Nothing the library said. Every failure becomes a *domain.InvalidJWTError
// with wording this package owns, because forwarding a dependency's error text
// to an API client publishes it as part of the contract: the caller used to
// receive "token is malformed: could not JSON decode header: invalid character
// '\x9e'" and "crypto/ecdsa: verification error", which a library upgrade could
// silently rewrite. The detail belongs in the log, where an operator can read it.
//
// The token itself is never put in the error either. It is a live credential
// until it expires, and an error message is the one place guaranteed to reach a
// log file.
//
// [InvalidJWTError.Expired] is the single distinction kept, because an expired
// token and an unreadable one mean opposite things to a caller trying to end a
// session — see the field's own documentation.
//
// # A token names the key that signed it
//
// The `kid` header is the RFC 7638 thumbprint of the signing key, and
// verification resolves it against a keyset rather than comparing it to one
// value. That is what makes a key replaceable without downtime: a new key can
// verify before it signs, and an old key can verify after it stops. See [New]
// for the sequence.
//
// It used to be `signKey.Params().N` — the order of the P-256 base point, a
// public constant of the curve — so every token ever issued carried the same
// value and the check compared a constant to itself. Two keys were
// indistinguishable, so the only way to replace one was to invalidate every
// live token at the moment of the deploy.
package tokenjwt

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"time"
	"uuid"

	"github.com/golang-jwt/jwt/v5"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// Signer implements token.Signer.
//
// The keys are parsed once, here, rather than on every Sign and Verify. The
// package comment used to claim that was already true; it was not, and a
// malformed key was therefore discovered on the first request rather than at
// startup.
type Signer struct {
	privateKey *ecdsa.PrivateKey

	// verifyKeys is every key a token may be verified against, indexed by its
	// thumbprint. It always holds the signing key, plus any key supplied for
	// rotation, which is what lets a new key be introduced before the old one
	// retires: a token names the key that signed it and is checked against
	// exactly that one.
	verifyKeys map[string]*ecdsa.PublicKey

	issuer string

	// signingKeyID is the thumbprint written into every token this service
	// signs. It is one of the keys in verifyKeys by construction.
	signingKeyID string
}

// Config is what a Signer needs to sign and to verify.
type Config struct {
	// Issuer is written to `iss` and `aud` when signing, and required to equal
	// them when verifying.
	Issuer string

	// PrivateKey signs. Its public half is always a verification key.
	PrivateKey []byte

	// PublicKey must be the public half of PrivateKey. It is checked rather
	// than assumed: a mismatched pair means every token this service issues is
	// one it cannot verify, and finding that out at startup is much cheaper
	// than finding it out per request.
	PublicKey []byte

	// AdditionalPublicKeys may verify but never sign. This is the whole
	// mechanism for rotating a key without downtime — see [New].
	AdditionalPublicKeys [][]byte
}

// New constructs a Signer from PEM-encoded EC keys.
//
// # Rotating the signing key without downtime
//
// Every token names the key that signed it, by thumbprint, in its `kid` header,
// and verification resolves that name against the keyset. A key can therefore
// be introduced before it is used and retired after it stops being used:
//
//  1. Add the new public key to AdditionalPublicKeys and deploy. Every replica
//     can now verify tokens signed by either key; nothing signs with the new
//     one yet.
//  2. Switch PrivateKey/PublicKey to the new pair, and move the OLD public key
//     into AdditionalPublicKeys. New tokens are signed by the new key; tokens
//     already out there still verify.
//  3. Once every token signed by the old key has expired — the longest-lived
//     class is a personal access token, up to a year — drop the old key.
//
// Skipping step 1 is what used to be unavoidable: with `kid` unable to name a
// key, there was no way to accept two keys at once, so replacing one invalidated
// every live token at the moment of the deploy.
//
// issuer is what the `iss` and `aud` claims are set to when signing and what
// they are required to equal when verifying. A token minted for a different
// issuer — another deployment sharing this key, say — is refused rather than
// accepted on the strength of the signature alone.
func New(conf Config) (*Signer, error) {
	if len(conf.PrivateKey) == 0 {
		return nil, &domain.InvalidPrivateKeyError{Message: "PrivateKey is empty"}
	}

	if len(conf.PublicKey) == 0 {
		return nil, &domain.InvalidPublicKeyError{Message: "PublicKey is empty"}
	}

	if conf.Issuer == "" {
		return nil, &domain.InvalidIssuerError{Message: "Issuer is empty, but it is required to verify iss and aud"}
	}

	priv, err := jwt.ParseECPrivateKeyFromPEM(conf.PrivateKey)
	if err != nil {
		return nil, &domain.InvalidPrivateKeyError{Message: fmt.Sprintf("PrivateKey is not a PEM-encoded EC key: %v", err)}
	}

	pub, err := jwt.ParseECPublicKeyFromPEM(conf.PublicKey)
	if err != nil {
		return nil, &domain.InvalidPublicKeyError{Message: fmt.Sprintf("PublicKey is not a PEM-encoded EC key: %v", err)}
	}

	signingKeyID, err := keyID(&priv.PublicKey)
	if err != nil {
		return nil, &domain.InvalidPrivateKeyError{Message: fmt.Sprintf("PrivateKey cannot be identified: %v", err)}
	}

	configuredKeyID, err := keyID(pub)
	if err != nil {
		return nil, &domain.InvalidPublicKeyError{Message: fmt.Sprintf("PublicKey cannot be identified: %v", err)}
	}

	// A pair that does not match means every token signed here is one this
	// service cannot verify. That is a configuration mistake with no valid
	// reading, so it is fatal at startup rather than a 401 on every request.
	if configuredKeyID != signingKeyID {
		return nil, &domain.InvalidPublicKeyError{
			Message: "PublicKey is not the public half of PrivateKey; tokens signed by this service would not verify against it",
		}
	}

	verifyKeys := map[string]*ecdsa.PublicKey{signingKeyID: &priv.PublicKey}

	for i, raw := range conf.AdditionalPublicKeys {
		if len(raw) == 0 {
			continue
		}

		additional, err := jwt.ParseECPublicKeyFromPEM(raw)
		if err != nil {
			return nil, &domain.InvalidPublicKeyError{
				Message: fmt.Sprintf("additional public key %d is not a PEM-encoded EC key: %v", i, err),
			}
		}

		id, err := keyID(additional)
		if err != nil {
			return nil, &domain.InvalidPublicKeyError{
				Message: fmt.Sprintf("additional public key %d cannot be identified: %v", i, err),
			}
		}

		// Listing the signing key again, or the same retired key twice, is
		// harmless: the thumbprint is derived from the key, so a duplicate
		// simply maps to the same entry.
		verifyKeys[id] = additional
	}

	return &Signer{
		privateKey:   priv,
		verifyKeys:   verifyKeys,
		issuer:       conf.Issuer,
		signingKeyID: signingKeyID,
	}, nil
}

// SigningKeyID returns the thumbprint written into the tokens this Signer
// issues. It exists so the composition root can log which key is signing, which
// is the only way an operator can tell where a rotation has got to.
func (s *Signer) SigningKeyID() string {
	return s.signingKeyID
}

// VerifyKeyIDs returns the thumbprint of every key that may verify, so a
// rotation's overlap window is visible from a log line rather than inferred.
func (s *Signer) VerifyKeyIDs() []string {
	ids := slices.Collect(maps.Keys(s.verifyKeys))
	slices.Sort(ids)

	return ids
}

type tokenCustomClaims struct {
	IDP       string           `json:"idp,omitempty"`
	Email     string           `json:"email,omitempty"`
	Data      string           `json:"data,omitempty"`
	TokenType domain.TokenType `json:"token_type"`
	jwt.RegisteredClaims
}

// Sign implements token.Signer.
func (s *Signer) Sign(_ context.Context, claims domain.JWTClaims) (string, error) {
	if claims.Subject == "" {
		return "", fmt.Errorf("subject is required")
	}

	if !claims.TokenType.IsValid() {
		return "", fmt.Errorf("invalid token type")
	}

	if claims.Issuer == "" {
		return "", fmt.Errorf("issuer is required")
	}

	// A caller may pin the jti — a personal access token uses its own row id, so
	// that the token can be traced back to the record that governs it. Everything
	// else leaves it zero and gets a fresh one.
	jwtID := claims.TokenID
	if jwtID == uuid.Nil() {
		jwtID = uuid.NewV7()
	}

	now := time.Now()

	tokenClaims := tokenCustomClaims{
		IDP:       claims.IDP,
		Email:     claims.Email,
		Data:      claims.Data,
		TokenType: claims.TokenType,
		ID:        jwtID.String(),
		Issuer:    claims.Issuer,
		Audience:  jwt.ClaimStrings{claims.Issuer},
		Subject:   claims.Subject,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(claims.TokenDuration)),
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodES256, tokenClaims)
	tok.Header["kid"] = s.signingKeyID

	return tok.SignedString(s.privateKey)
}

// Verify implements token.Signer.
//
// The parser options are the checks that used to be missing. WithValidMethods
// pins ES256 rather than trusting that an ECDSA public key happens to be
// unusable as an HMAC secret; WithIssuer and WithAudience make the iss and aud
// claims mean something, having been written on every token and read on none;
// WithExpirationRequired refuses a token that simply omits exp instead of
// treating "no deadline" as "not expired".
func (s *Signer) Verify(_ context.Context, token string) (map[string]any, error) {
	parsed, err := jwt.Parse(
		token, s.keyFunc,
		jwt.WithValidMethods([]string{jwt.SigningMethodES256.Alg()}),
		jwt.WithIssuer(s.issuer),
		jwt.WithAudience(s.issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, mapJWTError(err)
	}

	if !parsed.Valid {
		return nil, &domain.InvalidJWTError{Message: "the token is not valid"}
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, &domain.InvalidJWTError{Message: "the token claims are not valid"}
	}

	return map[string]any(claims), nil
}

// keyFunc resolves the verification key the token names.
//
// This is the half of key rotation that has to happen per request: the token
// says which key signed it and is checked against that key alone, so a keyset
// holding both an outgoing and an incoming key verifies tokens from either
// without ever accepting a token against the wrong one.
//
// It no longer parses a PEM on every call; every key was parsed at construction.
func (s *Signer) keyFunc(t *jwt.Token) (any, error) {
	kid, ok := t.Header["kid"].(string)
	if !ok {
		return nil, &domain.InvalidJWTError{Message: "the token has no kid header"}
	}

	key, ok := s.verifyKeys[kid]
	if !ok {
		return nil, &domain.InvalidJWTError{Message: "the token was signed by an unknown key"}
	}

	return key, nil
}

// mapJWTError translates the library's failures into wording this package owns.
//
// The original error is logged rather than returned. A caller learns that their
// token was not accepted, which is all they can act on; an operator gets the
// reason. See the package comment for why the library's own text must not
// become part of the API contract.
func mapJWTError(err error) error {
	// A domain error raised by keyFunc has already said its piece.
	if invalid, ok := errors.AsType[*domain.InvalidJWTError](err); ok {
		return invalid
	}

	slog.Debug("tokenjwt.Verify: token rejected", "error", err)

	switch {
	case errors.Is(err, jwt.ErrTokenExpired):
		return &domain.InvalidJWTError{Message: "the token has expired", Expired: true}
	case errors.Is(err, jwt.ErrTokenUsedBeforeIssued):
		return &domain.InvalidJWTError{Message: "the token is not valid yet"}
	case errors.Is(err, jwt.ErrTokenInvalidIssuer), errors.Is(err, jwt.ErrTokenInvalidAudience):
		// Deliberately the same wording as a bad signature. A caller holding a
		// token for another audience learns only that it was refused here.
		return &domain.InvalidJWTError{Message: "the token is not valid"}
	default:
		return &domain.InvalidJWTError{Message: "the token is not valid"}
	}
}
