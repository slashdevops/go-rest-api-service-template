// Package cipher defines the driven port that use-cases consume to
// keep secrets (third-party API tokens, IDP client secrets, …) at
// rest. The implementation lives in internal/adapter/driven/cipheraes.
//
// The port deals in the natural shape callers want: encrypt a
// plaintext into a transport-safe string and decrypt that string back
// to plaintext. The key, AES-GCM details, and base64 framing are the
// adapter's business.
package cipher

//go:generate go tool mockgen -package=mocks -destination=../../../../../mocks/service/cipher.go -source=cipher.go Cipher

// Cipher is the driven port consumed by use-cases.
type Cipher interface {
	// EncryptString encrypts plaintext with the adapter's key and
	// returns a transport-safe string suitable for storage.
	EncryptString(plaintext []byte) (string, error)

	// DecryptString reverses EncryptString.
	DecryptString(encoded string) ([]byte, error)
}
