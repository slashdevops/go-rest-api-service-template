package safedial

import "testing"

func TestPolicyCheck(t *testing.T) {
	t.Parallel()

	cases := []struct {
		address      string
		allowPrivate bool
		refused      bool
	}{
		{"93.184.216.34:443", false, false},
		{"[2606:2800:220:1:248:1893:25c8:1946]:443", false, false},
		{"169.254.169.254:80", false, true}, // metadata, always
		{"169.254.169.254:80", true, true},  // even when private is allowed
		{"[fe80::1]:80", true, true},
		{"127.0.0.1:11434", false, true}, // loopback refused by default
		{"127.0.0.1:11434", true, false}, // the dev stack's Ollama
		{"10.0.0.5:8080", false, true},
		{"10.0.0.5:8080", true, false},
		{"[fd00::5]:8080", false, true},
		{"[::ffff:10.0.0.5]:8080", false, true}, // v4-mapped v6 is unmapped first
		{"0.0.0.0:80", false, true},
		{"224.0.0.1:80", true, true},
		{"example.com:443", false, true}, // a name has no business here: the hook runs after resolution
	}

	for _, tc := range cases {
		err := Policy{AllowPrivate: tc.allowPrivate}.Check(tc.address)
		if (err != nil) != tc.refused {
			t.Errorf("%s allowPrivate=%v: refused=%v, want %v (%v)", tc.address, tc.allowPrivate, err != nil, tc.refused, err)
		}
	}
}
