package tokenjwt

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

const (
	testPrivateKey = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIOmpej94DsPn0at2LdHg32HbIZRdRkCzudFHknBuHHIboAoGCCqGSM49
AwEHoUQDQgAE0hqpf2JNz+OPFld6kTIur+N6h+a+I1MoXutxDGuHuN7sgJ6ofgSl
qojg1fKEgwrL4tPdUTXFU9btztkMUFT8qQ==
-----END EC PRIVATE KEY-----`

	testPublicKey = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE0hqpf2JNz+OPFld6kTIur+N6h+a+
I1MoXutxDGuHuN7sgJ6ofgSlqojg1fKEgwrL4tPdUTXFU9btztkMUFT8qQ==
-----END PUBLIC KEY-----`

	testInvalidPrivateKey = `-----BEGIN EC PRIVATE KEY-----
invalid key data here
-----END EC PRIVATE KEY-----`

	testIssuer = "test-issuer"
)

func newSigner(t *testing.T, additional ...[]byte) *Signer {
	t.Helper()

	s, err := New(Config{
		PrivateKey:           []byte(testPrivateKey),
		PublicKey:            []byte(testPublicKey),
		AdditionalPublicKeys: additional,
		Issuer:               testIssuer,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return s
}

// newKeyPair generates a second EC key, PEM-encoded, so rotation can be tested
// against a genuinely different key rather than a variation of the same one.
func newKeyPair(t *testing.T) (privPEM, pubPEM []byte, priv *ecdsa.PrivateKey) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	privDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("MarshalECPrivateKey: %v", err)
	}

	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey: %v", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privDER}),
		pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}),
		priv
}

// signRaw signs arbitrary claims with the test key and the kid the signer
// expects, so a test can produce a token that is genuinely ours in every respect
// except the one under test.
func signRaw(t *testing.T, method jwt.SigningMethod, claims jwt.MapClaims) string {
	t.Helper()

	key, err := jwt.ParseECPrivateKeyFromPEM([]byte(testPrivateKey))
	if err != nil {
		t.Fatalf("ParseECPrivateKeyFromPEM: %v", err)
	}

	return signRawWith(t, key, method, claims)
}

// signRawWith signs with a specific key and names that key in the kid header,
// which is what a token minted by a rotating deployment looks like.
func signRawWith(t *testing.T, key *ecdsa.PrivateKey, method jwt.SigningMethod, claims jwt.MapClaims) string {
	t.Helper()

	kid, err := keyID(&key.PublicKey)
	if err != nil {
		t.Fatalf("keyID: %v", err)
	}

	tok := jwt.NewWithClaims(method, claims)
	tok.Header["kid"] = kid

	var signKey any = key
	if _, isHMAC := method.(*jwt.SigningMethodHMAC); isHMAC {
		signKey = []byte("a shared secret an attacker would pick")
	}

	raw, err := tok.SignedString(signKey)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}

	return raw
}

func validClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"sub":        "test-user-123",
		"token_type": "access",
		"iss":        testIssuer,
		"aud":        jwt.ClaimStrings{testIssuer},
		"iat":        jwt.NewNumericDate(time.Now()),
		"exp":        jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
}

func TestNewRejectsUnusableInputs(t *testing.T) {
	_, otherPub, _ := newKeyPair(t)

	for name, conf := range map[string]Config{
		"empty private key": {PublicKey: []byte(testPublicKey), Issuer: testIssuer},
		"empty public key":  {PrivateKey: []byte(testPrivateKey), Issuer: testIssuer},

		// The issuer is what iss and aud are checked against, so verification
		// cannot be meaningful without it.
		"empty issuer": {PrivateKey: []byte(testPrivateKey), PublicKey: []byte(testPublicKey)},

		// Keys are parsed once, at construction. They used to be parsed per
		// call, so a malformed one was accepted at startup and found by a
		// request.
		"malformed private key": {PrivateKey: []byte(testInvalidPrivateKey), PublicKey: []byte(testPublicKey), Issuer: testIssuer},
		"malformed public key":  {PrivateKey: []byte(testPrivateKey), PublicKey: []byte("not a public key"), Issuer: testIssuer},

		// A mismatched pair means every token signed here is one this service
		// cannot verify. There is no valid reading of that, so it must not
		// start -- otherwise it surfaces as a 401 on every request instead.
		"mismatched pair": {PrivateKey: []byte(testPrivateKey), PublicKey: otherPub, Issuer: testIssuer},

		"malformed additional key": {
			PrivateKey:           []byte(testPrivateKey),
			PublicKey:            []byte(testPublicKey),
			AdditionalPublicKeys: [][]byte{[]byte("not a key")},
			Issuer:               testIssuer,
		},
	} {
		if _, err := New(conf); err == nil {
			t.Fatalf("New should reject: %s", name)
		}
	}
}

// TestKeyIDIdentifiesTheKey covers A-05: the kid header could not name a key.
//
// It was set to signKey.Params().N -- the order of the P-256 base point, a
// public constant of the curve -- so every token this service ever issued
// carried the same value and the check compared a constant to itself. Two keys
// were indistinguishable, which is the one thing a key identifier exists to do.
func TestKeyIDIdentifiesTheKey(t *testing.T) {
	signer := newSigner(t)

	priv, err := jwt.ParseECPrivateKeyFromPEM([]byte(testPrivateKey))
	if err != nil {
		t.Fatalf("ParseECPrivateKeyFromPEM: %v", err)
	}

	t.Run("is_not_the_curve_constant", func(t *testing.T) {
		assert.NotEqual(t, priv.Params().N.String(), signer.SigningKeyID(),
			"the kid must identify the key, not name the curve every key shares")
	})

	t.Run("differs_between_keys", func(t *testing.T) {
		_, _, other := newKeyPair(t)

		mine, err := keyID(&priv.PublicKey)
		assert.NoError(t, err)

		theirs, err := keyID(&other.PublicKey)
		assert.NoError(t, err)

		assert.NotEqual(t, mine, theirs, "two different keys must not share an id")
	})

	t.Run("is_stable_for_one_key", func(t *testing.T) {
		// A thumbprint that varied between processes would break verification
		// across replicas -- each would refuse the others' tokens.
		first, err := keyID(&priv.PublicKey)
		assert.NoError(t, err)

		second, err := keyID(&priv.PublicKey)
		assert.NoError(t, err)

		assert.Equal(t, first, second)
	})

	t.Run("is_an_unpadded_base64url_sha256", func(t *testing.T) {
		// RFC 7638: SHA-256 over the canonical JWK, base64url, no padding.
		id := signer.SigningKeyID()

		assert.Len(t, id, 43, "a base64url SHA-256 with no padding is 43 characters")
		assert.NotContains(t, id, "=")
		assert.NotContains(t, id, "+")
		assert.NotContains(t, id, "/")

		raw, err := base64.RawURLEncoding.DecodeString(id)
		assert.NoError(t, err)
		assert.Len(t, raw, sha256.Size)
	})

	t.Run("is_written_into_the_tokens_it_signs", func(t *testing.T) {
		signed, err := signer.Sign(context.Background(), domain.JWTClaims{
			Subject:       "test-user",
			Issuer:        testIssuer,
			TokenType:     domain.TokenTypeAccess,
			TokenDuration: time.Hour,
		})
		assert.NoError(t, err)

		parsed, _, err := jwt.NewParser().ParseUnverified(signed, jwt.MapClaims{})
		assert.NoError(t, err)
		assert.Equal(t, signer.SigningKeyID(), parsed.Header["kid"])
	})
}

// TestKeyRotation covers what naming the key is for: replacing a signing key
// without invalidating every token already issued.
func TestKeyRotation(t *testing.T) {
	ctx := context.Background()

	t.Run("a_key_can_verify_before_it_signs", func(t *testing.T) {
		// Step 1 of a rotation: the incoming key is trusted for verification
		// while the outgoing key is still the one signing.
		incomingPriv, incomingPub, incoming := newKeyPair(t)
		_ = incomingPriv

		signer := newSigner(t, incomingPub)

		// A token from a replica that has already switched over.
		token := signRawWith(t, incoming, jwt.SigningMethodES256, validClaims())

		claims, err := signer.Verify(ctx, token)
		assert.NoError(t, err, "a token signed by the incoming key must verify during the overlap")
		assert.Equal(t, "test-user-123", claims["sub"])

		// And this replica still signs with the outgoing key.
		outgoing, err := jwt.ParseECPrivateKeyFromPEM([]byte(testPrivateKey))
		assert.NoError(t, err)

		expected, err := keyID(&outgoing.PublicKey)
		assert.NoError(t, err)
		assert.Equal(t, expected, signer.SigningKeyID())
	})

	t.Run("a_key_can_verify_after_it_stops_signing", func(t *testing.T) {
		// Step 2: the new key signs, the old one is kept only to verify tokens
		// that are still in the wild. Without this, replacing a key invalidated
		// every live token at the instant of the deploy.
		newPriv, newPub, _ := newKeyPair(t)

		signer, err := New(Config{
			PrivateKey:           newPriv,
			PublicKey:            newPub,
			AdditionalPublicKeys: [][]byte{[]byte(testPublicKey)},
			Issuer:               testIssuer,
		})
		assert.NoError(t, err)

		retired, err := jwt.ParseECPrivateKeyFromPEM([]byte(testPrivateKey))
		assert.NoError(t, err)

		token := signRawWith(t, retired, jwt.SigningMethodES256, validClaims())

		_, err = signer.Verify(ctx, token)
		assert.NoError(t, err, "a token signed by the retired key must still verify during the overlap")

		assert.Len(t, signer.VerifyKeyIDs(), 2, "both keys are trusted during an overlap")
	})

	t.Run("a_key_that_was_never_trusted_is_refused", func(t *testing.T) {
		// The point of resolving by kid rather than trying every key: a token
		// naming a key we do not hold is refused, not attempted.
		_, _, stranger := newKeyPair(t)

		signer := newSigner(t)

		_, err := signer.Verify(ctx, signRawWith(t, stranger, jwt.SigningMethodES256, validClaims()))
		assert.Error(t, err, "a token signed by an untrusted key must be refused")
	})

	t.Run("the_old_constant_kid_is_refused", func(t *testing.T) {
		// Tokens issued before this change carry the P-256 group order as their
		// kid. That names no key, so they are refused -- the deploy consequence
		// this change carries, and the reason it wants a fresh key rather than
		// a quiet upgrade.
		priv, err := jwt.ParseECPrivateKeyFromPEM([]byte(testPrivateKey))
		assert.NoError(t, err)

		tok := jwt.NewWithClaims(jwt.SigningMethodES256, validClaims())
		tok.Header["kid"] = priv.Params().N.String()

		legacy, err := tok.SignedString(priv)
		assert.NoError(t, err)

		_, err = newSigner(t).Verify(ctx, legacy)
		assert.Error(t, err, "a kid that names no key must be refused")
	})
}

func TestSign(t *testing.T) {
	s := newSigner(t)
	ctx := context.Background()

	t.Run("success_valid_claims", func(t *testing.T) {
		claims := domain.JWTClaims{
			Subject:       "test-user-123",
			Issuer:        "test-issuer",
			TokenType:     domain.TokenTypeAccess,
			Email:         "test@example.com",
			IDP:           "local",
			TokenDuration: time.Hour,
		}

		signed, err := s.Sign(ctx, claims)
		assert.NoError(t, err)
		assert.NotEmpty(t, signed)

		parsed, err := jwt.Parse(signed, func(*jwt.Token) (any, error) {
			return jwt.ParseECPublicKeyFromPEM([]byte(testPublicKey))
		})
		assert.NoError(t, err)
		assert.True(t, parsed.Valid)
		assert.Equal(t, "ES256", parsed.Header["alg"])
		assert.Contains(t, parsed.Header, "kid")
	})

	t.Run("error_empty_subject", func(t *testing.T) {
		_, err := s.Sign(ctx, domain.JWTClaims{Issuer: "test", TokenType: domain.TokenTypeAccess})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "subject is required")
	})

	t.Run("error_empty_issuer", func(t *testing.T) {
		_, err := s.Sign(ctx, domain.JWTClaims{Subject: "test", TokenType: domain.TokenTypeAccess})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "issuer is required")
	})

	t.Run("error_invalid_token_type", func(t *testing.T) {
		_, err := s.Sign(ctx, domain.JWTClaims{Subject: "test", Issuer: "test", TokenType: domain.TokenType("invalid")})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid token type")
	})
}

func TestVerify(t *testing.T) {
	s := newSigner(t)
	ctx := context.Background()

	signed, err := s.Sign(ctx, domain.JWTClaims{
		Subject:       "test-user-123",
		Issuer:        testIssuer,
		TokenType:     domain.TokenTypeAccess,
		Email:         "test@example.com",
		TokenDuration: time.Hour,
	})
	assert.NoError(t, err)

	t.Run("success_valid_token", func(t *testing.T) {
		claims, err := s.Verify(ctx, signed)
		assert.NoError(t, err)
		assert.NotNil(t, claims)
		assert.Equal(t, "test-user-123", claims["sub"])
		assert.Equal(t, testIssuer, claims["iss"])
		assert.Equal(t, "access", claims["token_type"])
		assert.Equal(t, "test@example.com", claims["email"])
	})

	t.Run("error_invalid_token_format", func(t *testing.T) {
		_, err := s.Verify(ctx, "not.a.valid.jwt.token")
		assert.Error(t, err)
	})

	t.Run("error_token_without_kid", func(t *testing.T) {
		tok := jwt.NewWithClaims(jwt.SigningMethodES256, validClaims())
		pk, err := jwt.ParseECPrivateKeyFromPEM([]byte(testPrivateKey))
		assert.NoError(t, err)
		raw, err := tok.SignedString(pk)
		assert.NoError(t, err)

		_, err = s.Verify(ctx, raw)
		assert.Error(t, err)
		var jwtErr *domain.InvalidJWTError
		assert.ErrorAs(t, err, &jwtErr)
		assert.Contains(t, jwtErr.Message, "no kid header")
	})

	t.Run("error_expired_token", func(t *testing.T) {
		expired, err := s.Sign(ctx, domain.JWTClaims{
			Subject:       "test",
			Issuer:        testIssuer,
			TokenType:     domain.TokenTypeAccess,
			TokenDuration: -time.Hour,
		})
		assert.NoError(t, err)

		_, err = s.Verify(ctx, expired)
		assert.Error(t, err)
		var jwtErr *domain.InvalidJWTError
		assert.ErrorAs(t, err, &jwtErr)
		assert.True(t, jwtErr.Expired, "an expired token must be marked Expired; logout depends on telling it from an unreadable one")
	})
}

// TestVerifyChecksTheClaimsItWrites covers A-07: iss and aud were set on every
// token and validated on none, so a token minted for another issuer -- another
// deployment sharing this key -- was accepted on its signature alone.
//
// Reproduced against the running API before the fix: a token with
// iss = aud = "https://attacker.example" answered 200 with a full model list.
func TestVerifyChecksTheClaimsItWrites(t *testing.T) {
	s := newSigner(t)
	ctx := context.Background()

	t.Run("rejects_a_foreign_issuer", func(t *testing.T) {
		claims := validClaims()
		claims["iss"] = "https://attacker.example"

		_, err := s.Verify(ctx, signRaw(t, jwt.SigningMethodES256, claims))
		assert.Error(t, err, "a token issued by someone else must not be accepted on our signature")
	})

	t.Run("rejects_a_foreign_audience", func(t *testing.T) {
		claims := validClaims()
		claims["aud"] = jwt.ClaimStrings{"https://attacker.example"}

		_, err := s.Verify(ctx, signRaw(t, jwt.SigningMethodES256, claims))
		assert.Error(t, err, "a token minted for another audience must not be accepted here")
	})

	t.Run("rejects_a_token_with_no_expiry", func(t *testing.T) {
		claims := validClaims()
		delete(claims, "exp")

		_, err := s.Verify(ctx, signRaw(t, jwt.SigningMethodES256, claims))
		assert.Error(t, err, "a token with no deadline must not be read as one that has not expired")
	})

	t.Run("rejects_a_signing_method_that_is_not_ES256", func(t *testing.T) {
		// Honest note: this subtest still passes with WithValidMethods removed,
		// and that was checked rather than assumed. golang-jwt refuses an
		// *ecdsa.PublicKey where HMAC expects bytes, so the outcome is
		// currently guaranteed twice over.
		//
		// It is kept because it pins the OUTCOME rather than the mechanism: if
		// keyFunc ever returns something an HMAC method could consume, the
		// library's half of the guarantee disappears and WithValidMethods
		// becomes the only thing left. That is the day this needs to fail.
		_, err := s.Verify(ctx, signRaw(t, jwt.SigningMethodHS256, validClaims()))
		assert.Error(t, err, "only ES256 may verify")
	})

	t.Run("says_nothing_about_the_token_or_the_library", func(t *testing.T) {
		// The caller used to receive the jwt package's own text, and the raw
		// token was carried in the error's Value field on its way to a log.
		raw := signRaw(t, jwt.SigningMethodES256, validClaims())

		claims := validClaims()
		claims["iss"] = "https://attacker.example"

		for _, token := range []string{
			"not.a.jwt",
			signRaw(t, jwt.SigningMethodES256, claims),
			raw[:len(raw)-6] + "AAAAAA",
		} {
			_, err := s.Verify(ctx, token)
			assert.Error(t, err)

			assert.NotContains(t, err.Error(), token, "the error must not carry the token")
			assert.NotContains(t, err.Error(), "crypto/ecdsa", "the error must not carry the library's wording")
			assert.NotContains(t, err.Error(), "JSON decode", "the error must not carry the library's wording")
		}
	})
}
