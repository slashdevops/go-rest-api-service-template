package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
)

func request(remoteAddr string, headers map[string]string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return req
}

// TestClientIPResolver_untrustedPeerCannotChooseItsBucket is the regression
// test for the rate-limiter bypass. Every case here is a request whose
// forwarding headers must be ignored outright, because the peer that sent them
// is not a proxy we trust.
//
// The previous resolver honoured X-Forwarded-For unconditionally, so all of
// these returned the header value — which let a caller rotate the header and
// draw a fresh rate-limit budget on every request.
func TestClientIPResolver_untrustedPeerCannotChooseItsBucket(t *testing.T) {
	t.Parallel()

	resolver, err := middleware.NewClientIPResolver(nil)
	require.NoError(t, err)

	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
	}{
		{"forwarded_for_single", "203.0.113.9:1234", map[string]string{"X-Forwarded-For": "10.0.0.1"}},
		{"forwarded_for_chain", "203.0.113.9:1234", map[string]string{"X-Forwarded-For": "1.1.1.1, 2.2.2.2, 3.3.3.3"}},
		{"real_ip", "203.0.113.9:1234", map[string]string{"X-Real-IP": "198.51.100.42"}},
		{"both_headers", "203.0.113.9:1234", map[string]string{"X-Forwarded-For": "1.1.1.1", "X-Real-IP": "2.2.2.2"}},
		{"header_naming_a_trusted_looking_net", "203.0.113.9:1234", map[string]string{"X-Forwarded-For": "127.0.0.1"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, "203.0.113.9", resolver.ClientIP(request(tc.remoteAddr, tc.headers)),
				"an untrusted peer must be attributed to its own address, whatever it puts in a header")
		})
	}
}

// TestClientIPResolver_rotatingHeaderIsOneBucket states the property the
// bypass violated: a caller varying the header must still resolve to a single
// address, so it draws from a single rate-limit bucket.
func TestClientIPResolver_rotatingHeaderIsOneBucket(t *testing.T) {
	t.Parallel()

	resolver, err := middleware.NewClientIPResolver(nil)
	require.NoError(t, err)

	seen := make(map[string]struct{})
	for _, spoofed := range []string{"203.0.113.1", "203.0.113.2", "198.51.100.7", "10.0.0.5", "8.8.8.8"} {
		seen[resolver.ClientIP(request("192.0.2.50:9999", map[string]string{"X-Forwarded-For": spoofed}))] = struct{}{}
	}

	assert.Len(t, seen, 1, "a rotating X-Forwarded-For must not produce more than one rate-limit key")
	assert.Contains(t, seen, "192.0.2.50")
}

// TestClientIPResolver_trustedPeerWalksTheChain covers the behaviour that has
// to keep working for a real deployment behind a proxy.
func TestClientIPResolver_trustedPeerWalksTheChain(t *testing.T) {
	t.Parallel()

	resolver, err := middleware.NewClientIPResolver([]string{"10.0.0.0/8", "192.0.2.10"})
	require.NoError(t, err)

	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		want       string
		why        string
	}{
		{
			name:       "single_hop",
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.50"},
			want:       "203.0.113.50",
			why:        "one trusted proxy appended the client it saw",
		},
		{
			name:       "spoof_to_the_left_is_skipped",
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"X-Forwarded-For": "9.9.9.9, 203.0.113.50"},
			want:       "203.0.113.50",
			why:        "the rightmost untrusted hop is the first one our proxy vouched for",
		},
		{
			name:       "internal_hops_are_skipped",
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.50, 10.1.1.1, 10.2.2.2"},
			want:       "203.0.113.50",
			why:        "hops inside the trusted range are ours, not the client",
		},
		{
			name:       "single_ip_entry_is_trusted_as_a_host_route",
			remoteAddr: "192.0.2.10:1234",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.50"},
			want:       "203.0.113.50",
			why:        "a bare IP in the trusted list behaves as a /32",
		},
		{
			name:       "every_hop_trusted_falls_back_to_leftmost",
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"X-Forwarded-For": "10.1.1.1, 10.2.2.2"},
			want:       "10.1.1.1",
			why:        "no untrusted hop exists; the leftmost is the best the chain offers",
		},
		{
			name:       "real_ip_used_when_no_chain",
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"X-Real-IP": "198.51.100.42"},
			want:       "198.51.100.42",
			why:        "a trusted peer set it and there is no chain to prefer",
		},
		{
			name:       "chain_beats_real_ip",
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.50", "X-Real-IP": "198.51.100.42"},
			want:       "203.0.113.50",
			why:        "the chain carries strictly more information",
		},
		{
			name:       "entry_with_port",
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.50:51515"},
			want:       "203.0.113.50",
			why:        "some proxies append a port",
		},
		{
			name:       "ipv6_client",
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"X-Forwarded-For": "2001:db8::1"},
			want:       "2001:db8::1",
			why:        "IPv6 clients resolve unchanged",
		},
		{
			name:       "ipv4_mapped_ipv6_is_unwrapped",
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"X-Forwarded-For": "::ffff:203.0.113.50"},
			want:       "203.0.113.50",
			why:        "the mapped and bare forms must share one bucket, not split into two",
		},
		{
			name:       "malformed_hop_falls_back_to_peer",
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"X-Forwarded-For": "not-an-ip, 203.0.113.50, banana"},
			want:       "10.0.0.1",
			why:        "an unreadable chain must over-limit, not trust a guess",
		},
		{
			name:       "empty_hop_falls_back_to_peer",
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.50, "},
			want:       "10.0.0.1",
			why:        "a trailing separator leaves a hop that cannot be read",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, resolver.ClientIP(request(tc.remoteAddr, tc.headers)), tc.why)
		})
	}
}

func TestClientIPResolver_peerAddressForms(t *testing.T) {
	t.Parallel()

	resolver, err := middleware.NewClientIPResolver(nil)
	require.NoError(t, err)

	tests := []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{"ipv4_with_port", "192.168.1.1:8080", "192.168.1.1"},
		{"ipv6_bracketed_with_port", "[::1]:8080", "::1"},
		{"bare_ipv4", "192.168.1.1", "192.168.1.1"},
		{"ipv4_mapped_ipv6_peer", "[::ffff:192.168.1.1]:8080", "192.168.1.1"},
		{"malformed_is_returned_verbatim", "not-an-address", "not-an-address"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, resolver.ClientIP(request(tc.remoteAddr, nil)))
		})
	}
}

func TestNewClientIPResolver(t *testing.T) {
	t.Parallel()

	t.Run("empty_list_trusts_nothing", func(t *testing.T) {
		t.Parallel()

		resolver, err := middleware.NewClientIPResolver([]string{"", "   "})
		require.NoError(t, err)
		assert.False(t, resolver.TrustsAnything())
	})

	t.Run("accepts_cidr_and_bare_addresses", func(t *testing.T) {
		t.Parallel()

		resolver, err := middleware.NewClientIPResolver([]string{"10.0.0.0/8", "fd00::/8", "192.0.2.10", "2001:db8::1"})
		require.NoError(t, err)
		assert.True(t, resolver.TrustsAnything())
	})

	t.Run("unmasked_cidr_still_matches_the_block_it_names", func(t *testing.T) {
		t.Parallel()

		// A human writing "10.1.2.3/8" means the /8. netip.Prefix.Contains
		// reports false on an unmasked prefix, so the constructor masks it.
		resolver, err := middleware.NewClientIPResolver([]string{"10.1.2.3/8"})
		require.NoError(t, err)

		assert.Equal(t, "203.0.113.50",
			resolver.ClientIP(request("10.0.0.1:1234", map[string]string{"X-Forwarded-For": "203.0.113.50"})))
	})

	t.Run("rejects_a_malformed_entry", func(t *testing.T) {
		t.Parallel()

		_, err := middleware.NewClientIPResolver([]string{"10.0.0.0/8", "definitely-not-an-ip"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "definitely-not-an-ip")
	})
}
