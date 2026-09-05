// Package cipheraes is the driven adapter that satisfies the
// cipher.Cipher port using AES-GCM symmetric encryption with the
// ciphertext base64-encoded for transport. The adapter holds the
// symmetric key once at construction; use-cases never see it.
package cipheraes

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// Adapter implements cipher.Cipher.
type Adapter struct {
	key []byte
}

// New returns an Adapter bound to key, which must be 16, 24 or 32 bytes --
// AES-128, AES-192 or AES-256. There is no other valid length.
//
// # Why this is checked here, and strictly
//
// It used to accept anything between 3 and 255 bytes, which was not a weaker
// version of this rule but a different rule entirely: those bounds belong to
// the PATH of the key file, and were borrowed from the config package as though
// they bounded the key. The doc comment on this function stated the 16/24/32
// rule that the code did not enforce.
//
// The consequence was that a wrong-length key started the service cleanly and
// failed at FIRST USE, which is not a rare path: the key encrypts an
// engine's api_token and an identity provider's client_secret, and decrypts the
// api_token on every query that reaches a hosted provider. So a truncated or
// half-copied aes-256-symmetric-hex.key looked like a healthy deployment right
// up until someone ran a search, and then presented as
//
//	crypto/aes: invalid key size 4
//
// on the login path -- the thing the deployment exists to do.
//
// Refusing at construction makes it a startup failure instead. The composition
// root builds this adapter while wiring services, so an operator learns about a
// bad key when the process refuses to come up, naming the setting.
func New(key []byte) (*Adapter, error) {
	if !domain.ValidAESKeySize(len(key)) {
		return nil, &domain.InvalidSymmetricKeyError{
			Message: fmt.Sprintf(
				"symmetric key is %d bytes; AES requires exactly %d, %d or %d "+
					"(authn.symmetric.key.file holds the key hex-encoded, so those are %d, %d or %d hex characters)",
				len(key),
				domain.AESKeySize128, domain.AESKeySize192, domain.AESKeySize256,
				domain.AESKeySize128*2, domain.AESKeySize192*2, domain.AESKeySize256*2,
			),
		}
	}

	return &Adapter{key: key}, nil
}

// EncryptString implements cipher.Cipher.
func (a *Adapter) EncryptString(plaintext []byte) (string, error) {
	block, err := aes.NewCipher(a.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ct := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// DecryptString implements cipher.Cipher.
func (a *Adapter) DecryptString(encoded string) ([]byte, error) {
	ct, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(a.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ct) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, body := ct[:nonceSize], ct[nonceSize:]
	return gcm.Open(nil, nonce, body, nil)
}
