package app

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"

	"github.com/slashdevops/go-rest-api-service-template/internal/config"
)

// caFixture writes a real PEM certificate to a temp file. A syntactically
// invalid file would exercise the wrong branch: AppendCertsFromPEM reports
// failure by returning false, so the test has to distinguish "unreadable" from
// "readable but not a certificate".
func caFixture(t *testing.T) string {
	t.Helper()

	// A self-signed certificate, DER-encoded and PEM-wrapped. Content does not
	// matter beyond parsing.
	const pemCert = `-----BEGIN CERTIFICATE-----
MIIBhTCCASugAwIBAgIQIRi6zePL6mKjOipn+dNuaTAKBggqhkjOPQQDAjASMRAw
DgYDVQQKEwdBY21lIENvMB4XDTE3MTAyMDE5NDMwNloXDTE4MTAyMDE5NDMwNlow
EjEQMA4GA1UEChMHQWNtZSBDbzBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABD0d
7VNhbWvZLWPuj/RtHFjvtJBEwOkhbN/BnnE8rnZR8+sbwnc/KhCk3FhnpHZnQz7B
5aETbbIgmuvewdjvSBSjYzBhMA4GA1UdDwEB/wQEAwICpDATBgNVHSUEDDAKBggr
BgEFBQcDATAPBgNVHRMBAf8EBTADAQH/MCkGA1UdEQQiMCCCDmxvY2FsaG9zdDo1
NDUzgg4xMjcuMC4wLjE6NTQ1MzAKBggqhkjOPQQDAgNIADBFAiEA2zpJEPQyz6/l
Wf86aX6PepsntZv2GYlA5UpabfT2EZICICpJ5h/iI+i341gBmLiAFQOyTDT+/wQc
6MF9+Yw1Yy0t
-----END CERTIFICATE-----
`

	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, []byte(pemCert), 0o600); err != nil {
		t.Fatalf("write CA fixture: %v", err)
	}

	return path
}

func appWithCacheConfig(t *testing.T) *App {
	t.Helper()

	return &App{configs: &Configs{Cache: config.NewCacheConfig()}}
}

// TestCacheTLSConfig covers what the cache client is actually handed. The
// connection carries bcrypt password hashes and the cache AUTH password, so
// "TLS is enabled" has to mean a verified connection rather than merely an
// encrypted one.
func TestCacheTLSConfig(t *testing.T) {
	t.Parallel()

	t.Run("disabled yields no TLS config", func(t *testing.T) {
		t.Parallel()

		a := appWithCacheConfig(t)

		got, err := a.cacheTLSConfig()
		if err != nil {
			t.Fatalf("cacheTLSConfig: %v", err)
		}

		if got != nil {
			t.Error("TLS is off, so the client must receive a nil TLS config")
		}
	})

	t.Run("enabled pins TLS 1.3 and verifies by default", func(t *testing.T) {
		t.Parallel()

		a := appWithCacheConfig(t)
		a.configs.Cache.TLSEnabled.Value = true

		got, err := a.cacheTLSConfig()
		if err != nil {
			t.Fatalf("cacheTLSConfig: %v", err)
		}

		if got.MinVersion != tls.VersionTLS13 {
			t.Errorf("MinVersion is %d, want TLS 1.3", got.MinVersion)
		}

		if got.InsecureSkipVerify {
			t.Error("verification must be on unless explicitly disabled")
		}

		if got.RootCAs != nil {
			t.Error("with no CA file the host trust store should be used, which means RootCAs stays nil")
		}
	})

	t.Run("a CA file becomes the root pool", func(t *testing.T) {
		t.Parallel()

		a := appWithCacheConfig(t)
		a.configs.Cache.TLSEnabled.Value = true
		a.configs.Cache.TLSCAFile.Value = caFixture(t)

		got, err := a.cacheTLSConfig()
		if err != nil {
			t.Fatalf("cacheTLSConfig: %v", err)
		}

		if got.RootCAs == nil {
			t.Error("a configured CA file must replace the host trust store")
		}
	})

	t.Run("a CA file with no certificate in it is refused", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "junk.pem")
		if err := os.WriteFile(path, []byte("this is not a certificate"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}

		a := appWithCacheConfig(t)
		a.configs.Cache.TLSEnabled.Value = true
		a.configs.Cache.TLSCAFile.Value = path

		// AppendCertsFromPEM signals failure only by returning false. Without an
		// explicit check this would surface as an unexplained handshake error
		// naming neither the file nor the reason.
		if _, err := a.cacheTLSConfig(); err == nil {
			t.Error("expected an error naming the unusable CA file")
		}
	})

	t.Run("a missing CA file is refused", func(t *testing.T) {
		t.Parallel()

		a := appWithCacheConfig(t)
		a.configs.Cache.TLSEnabled.Value = true
		a.configs.Cache.TLSCAFile.Value = filepath.Join(t.TempDir(), "absent.pem")

		if _, err := a.cacheTLSConfig(); err == nil {
			t.Error("expected an error for an unreadable CA file")
		}
	})

	t.Run("skip verify is honoured when asked for", func(t *testing.T) {
		t.Parallel()

		a := appWithCacheConfig(t)
		a.configs.Cache.TLSEnabled.Value = true
		a.configs.Cache.TLSInsecureSkipVerify.Value = true

		got, err := a.cacheTLSConfig()
		if err != nil {
			t.Fatalf("cacheTLSConfig: %v", err)
		}

		if !got.InsecureSkipVerify {
			t.Error("InsecureSkipVerify was requested and must be applied")
		}
	})
}
