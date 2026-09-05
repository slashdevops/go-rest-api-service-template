package cipheraes

import (
	"crypto/rand"
	"fmt"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	a, err := New(key)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	plaintext := []byte("a secret token value")

	encoded, err := a.EncryptString(plaintext)
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}

	got, err := a.DecryptString(encoded)
	if err != nil {
		t.Fatalf("DecryptString: %v", err)
	}

	if string(got) != string(plaintext) {
		t.Fatalf("round-trip mismatch: got %q, want %q", got, plaintext)
	}
}

// TestNewRejectsBadKey is kept as-is, and is worth reading as a lesson.
//
// It asserts nothing false, it passed continuously, and it gave cover to a real
// bug for the whole of its life: nil and 1024 bytes both fall outside the old
// 3-to-255 bound, so it never touched the gap where every wrong-length key
// actually lands. Every size from 4 to 255 was accepted by the constructor and
// refused by AES at first use, and this test could not have noticed.
//
// TestNewRejectsAKeyAESCannotUse enumerates that gap deliberately.
func TestNewRejectsBadKey(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Fatalf("New should reject nil key")
	}
	if _, err := New(make([]byte, 1024)); err == nil {
		t.Fatalf("New should reject over-long key")
	}
}

func TestDecryptRejectsWrongKey(t *testing.T) {
	keyA := make([]byte, 32)
	keyB := make([]byte, 32)
	if _, err := rand.Read(keyA); err != nil {
		t.Fatalf("rand.Read keyA: %v", err)
	}
	if _, err := rand.Read(keyB); err != nil {
		t.Fatalf("rand.Read keyB: %v", err)
	}

	a, _ := New(keyA)
	b, _ := New(keyB)

	encoded, err := a.EncryptString([]byte("secret"))
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}

	if _, err := b.DecryptString(encoded); err == nil {
		t.Fatalf("DecryptString with wrong key should fail")
	}
}

func TestDecryptRejectsGarbage(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	a, _ := New(key)

	if _, err := a.DecryptString("not-base64-!!!"); err == nil {
		t.Fatalf("DecryptString should reject non-base64 input")
	}
	if _, err := a.DecryptString(""); err == nil {
		t.Fatalf("DecryptString should reject empty/short ciphertext")
	}
}

// TestNewRejectsAKeyAESCannotUse is the whole point of this change.
//
// The check here used to be a bound on a FILE PATH -- 3 to 255 bytes -- applied
// to the key, so every length in this table except the three valid ones was
// accepted. The service started, looked healthy, and failed later with
// "crypto/aes: invalid key size N" on the first request that needed the key.
//
// That first request is one reaching a third-party provider,
// because the key encrypts an engine's api_token and decrypts it on every such
// query. A truncated aes-256-symmetric-hex.key therefore presented as a broken
// integration path, not as a broken configuration.
func TestNewRejectsAKeyAESCannotUse(t *testing.T) {
	t.Parallel()

	for _, size := range []int{0, 1, 3, 4, 8, 15, 17, 23, 25, 31, 33, 64, 128, 255} {
		t.Run(fmt.Sprintf("%d_bytes", size), func(t *testing.T) {
			t.Parallel()

			adapter, err := New(make([]byte, size))
			if err == nil {
				t.Fatalf("New accepted a %d-byte key; AES cannot use it, so this would have failed on the first encrypt instead", size)
			}

			if adapter != nil {
				t.Error("New returned an adapter alongside an error")
			}

			// The message has to be actionable: an operator reading it should
			// know what to do without going to the source. The file is
			// hex-encoded, so the byte count alone is not enough.
			if msg := err.Error(); !strings.Contains(msg, "64") {
				t.Errorf("error does not mention the hex length an operator would check: %q", msg)
			}
		})
	}
}

// TestNewAcceptsEveryAESKeySize is the other direction: the fix must not have
// narrowed the rule to AES-256 only.
func TestNewAcceptsEveryAESKeySize(t *testing.T) {
	t.Parallel()

	for _, size := range []int{16, 24, 32} {
		t.Run(fmt.Sprintf("%d_bytes", size), func(t *testing.T) {
			t.Parallel()

			adapter, err := New(make([]byte, size))
			if err != nil {
				t.Fatalf("New rejected a valid AES-%d key: %v", size*8, err)
			}

			// And it works, which is what "valid" has to mean here. A
			// constructor that accepts a key the cipher then refuses would be
			// the same bug with a different line number.
			encrypted, err := adapter.EncryptString([]byte("a third-party api token"))
			if err != nil {
				t.Fatalf("EncryptString with an accepted %d-byte key: %v", size, err)
			}

			decrypted, err := adapter.DecryptString(encrypted)
			if err != nil {
				t.Fatalf("DecryptString: %v", err)
			}

			if string(decrypted) != "a third-party api token" {
				t.Errorf("round trip returned %q", decrypted)
			}
		})
	}
}
