package tokenjwt

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// keyID derives a token's `kid` from the key that signs it: the RFC 7638 JWK
// thumbprint, which is the SHA-256 of a canonical JSON encoding of the key's
// public parameters, base64url-encoded without padding.
//
// # Why this and not something simpler
//
// `kid` used to be `signKey.Params().N` — the order of the P-256 base point.
// That is a public constant of the curve, identical for every key anyone has
// ever generated, so every token this service issued carried the same value and
// the check on the verify side compared a constant to itself. It could not
// distinguish two keys, which is the one thing a key identifier exists to do,
// and without that there is no way to introduce a new key before retiring the
// old one — so key rotation meant downtime and invalidating every live token.
//
// The thumbprint is derived from the key material itself, so it needs no
// registry to stay in step: anyone holding the public key can compute the same
// value, and two different keys cannot collide without breaking SHA-256.
//
// # The canonical form is the whole point
//
// RFC 7638 fixes both the member set and their order — for EC keys exactly
// `crv`, `kty`, `x`, `y`, lexicographically ordered, no whitespace — so that
// the same key always hashes to the same thumbprint regardless of who encodes
// it. The JSON is built by hand here rather than with encoding/json for that
// reason: the canonical form is a wire format, not a struct, and a marshaller
// that reorders or adds a field would silently change every kid.
//
// x and y are taken from the key's uncompressed SEC 1 encoding, which is
// already fixed-length and left-padded. The obvious alternative, big.Int.Bytes()
// on pub.X and pub.Y, trims a leading zero byte — that would give the same key
// two different thumbprints depending on its value, for roughly one key in 256.
// (Those fields are also deprecated as of Go 1.26.)
func keyID(pub *ecdsa.PublicKey) (string, error) {
	if pub == nil || pub.Curve == nil {
		return "", fmt.Errorf("cannot derive a key id from a nil key")
	}

	crv, ok := curveName(pub)
	if !ok {
		return "", fmt.Errorf("unsupported curve %q; this service signs with ES256 (P-256)", pub.Curve.Params().Name)
	}

	// 0x04 || X || Y, each coordinate padded to the curve's byte size.
	point, err := pub.Bytes()
	if err != nil {
		return "", fmt.Errorf("cannot encode the public key: %w", err)
	}

	byteLen := (pub.Curve.Params().BitSize + 7) / 8

	if len(point) != 1+2*byteLen || point[0] != 4 {
		return "", fmt.Errorf("unexpected public key encoding: %d bytes", len(point))
	}

	x := point[1 : 1+byteLen]
	y := point[1+byteLen:]

	// RFC 7638 section 3.2: the required members for an EC key, in lexicographic
	// order, with no whitespace and no other members.
	canonical := fmt.Sprintf(`{"crv":"%s","kty":"EC","x":"%s","y":"%s"}`,
		crv,
		base64.RawURLEncoding.EncodeToString(x),
		base64.RawURLEncoding.EncodeToString(y),
	)

	sum := sha256.Sum256([]byte(canonical))

	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// curveName maps a curve to its JWK `crv` name. Only P-256 is accepted: the
// service signs ES256 and nothing else, and a thumbprint computed for a curve
// we cannot sign with would name a key that can never be used.
func curveName(pub *ecdsa.PublicKey) (string, bool) {
	if pub.Curve == elliptic.P256() {
		return "P-256", true
	}

	return "", false
}
