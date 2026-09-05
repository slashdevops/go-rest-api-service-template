package middleware

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
)

// ClientIPResolver derives the client IP of a request for rate-limiting and
// logging purposes.
//
// # Why this is not just "read X-Forwarded-For"
//
// Forwarding headers are supplied by whoever sent the request. A client can
// put anything in X-Forwarded-For, so a resolver that reads it unconditionally
// lets the caller choose its own rate-limit bucket — a fresh value per request
// is a fresh budget per request, which removes the limit entirely rather than
// weakening it. That is the bug this type exists to prevent: before it, a
// rotating header turned 30 blocked login attempts into 30 evaluated ones.
//
// A forwarding header is only meaningful when the hop that set it is one we
// control. So the resolver trusts headers only when the immediate peer
// (http.Request.RemoteAddr) is inside a configured set of trusted proxy
// networks, and with an empty set it ignores the headers completely.
//
// # The walk
//
// A trusted proxy appends the address it saw to the right-hand end of
// X-Forwarded-For. The chain therefore reads oldest-first, and everything to
// the left of the first hop we control is hearsay the client could have
// written. So the resolver walks the chain from the right and returns the
// first address that is not itself a trusted proxy:
//
//	X-Forwarded-For: 10.9.9.9, 203.0.113.7      RemoteAddr: 172.16.0.1 (trusted)
//	                 ^ client-supplied  ^ appended by our proxy
//	                                    └── returned
//
// The spoofed 10.9.9.9 is skipped because a real address sits to its right.
// If every hop in the chain is trusted, the leftmost entry is returned — that
// is the only candidate left, and it came from a proxy we control.
//
// # Failure is biased toward over-limiting
//
// When the chain cannot be read — a malformed entry, an unparsable header —
// the resolver falls back to the peer address rather than guessing. That
// groups every client behind that proxy into one bucket, which throttles too
// much rather than too little. Given the alternative is honouring a value an
// attacker wrote, that is the direction to fail in.
type ClientIPResolver struct {
	trusted []netip.Prefix
}

// NewClientIPResolver builds a resolver that trusts the given proxy networks.
//
// Entries may be CIDR blocks ("10.0.0.0/8", "fd00::/8") or single addresses
// ("192.0.2.10"), which are treated as a /32 or /128. An empty or blank list
// yields a resolver that ignores forwarding headers entirely and always
// reports the peer address — the correct behaviour for a service exposed
// directly, and the safe default for one whose deployment is unknown.
func NewClientIPResolver(trustedProxies []string) (*ClientIPResolver, error) {
	prefixes, err := ParseIPMatchers(trustedProxies, "trusted proxy")
	if err != nil {
		return nil, err
	}

	return &ClientIPResolver{trusted: prefixes}, nil
}

// ParseIPMatchers turns a list of CIDR blocks and bare addresses into prefixes.
//
// Entries may be CIDR blocks ("10.0.0.0/8", "fd00::/8") or single addresses
// ("192.0.2.10"), which become a /32 or /128. Blank entries are skipped, so a
// trailing comma in a flag is not an error.
//
// what names the setting in the error, because "neither a CIDR block nor an IP
// address" is only actionable if it says which list the bad entry is in.
//
// Shared by the trusted-proxy list and the rate limiter's excluded-IP list. They
// were not shared before, and the excluded list did an exact string comparison
// instead -- so a CIDR block there matched nothing at all, silently, and an
// operator who wrote one believed a monitoring network was exempt when it was
// not. Two lists of the same kind of thing should accept the same syntax.
func ParseIPMatchers(entries []string, what string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(entries))

	for _, raw := range entries {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}

		if prefix, err := netip.ParsePrefix(entry); err == nil {
			// Masked so that a sloppy "10.1.2.3/8" still matches the /8 it
			// names; netip.Prefix.Contains reports false for an unmasked one.
			prefixes = append(prefixes, prefix.Masked())

			continue
		}

		addr, err := netip.ParseAddr(entry)
		if err != nil {
			return nil, fmt.Errorf("%s %q is neither a CIDR block nor an IP address", what, entry)
		}

		prefixes = append(prefixes, netip.PrefixFrom(addr.Unmap(), addr.Unmap().BitLen()))
	}

	return prefixes, nil
}

// TrustsAnything reports whether any proxy network is configured. When false
// the resolver ignores X-Forwarded-For and X-Real-IP.
func (ref *ClientIPResolver) TrustsAnything() bool {
	return len(ref.trusted) > 0
}

// ClientIP returns the address to attribute the request to.
//
// The result is a bare IP with any IPv4-mapped IPv6 form unwrapped, so that
// 203.0.113.7 and ::ffff:203.0.113.7 share a rate-limit bucket rather than
// splitting into two.
func (ref *ClientIPResolver) ClientIP(r *http.Request) string {
	peer, ok := peerAddr(r.RemoteAddr)
	if !ok {
		// RemoteAddr is set by the server, so this should not happen; return it
		// verbatim rather than inventing an address to attribute the request to.
		return r.RemoteAddr
	}

	// Not behind a proxy we control: the peer is the client, and any
	// forwarding header on the request was written by the client itself.
	if !ref.isTrusted(peer) {
		return peer.String()
	}

	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		if client, ok := ref.walkForwardedFor(forwarded); ok {
			return client.String()
		}
		// Chain unreadable — fall back to the peer rather than to a value the
		// client may have chosen.
		return peer.String()
	}

	// X-Real-IP carries a single address and no chain, so it is only usable
	// when the trusted peer set it. It is checked after X-Forwarded-For
	// because the chain is strictly more informative.
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		if addr, ok := parseChainAddr(realIP); ok {
			return addr.String()
		}
		return peer.String()
	}

	return peer.String()
}

// walkForwardedFor scans the chain right to left and returns the first
// address that is not a trusted proxy. It reports false when no entry parses.
func (ref *ClientIPResolver) walkForwardedFor(header string) (netip.Addr, bool) {
	hops := strings.Split(header, ",")

	var leftmost netip.Addr
	var haveLeftmost bool

	for _, hop := range slices.Backward(hops) {
		addr, ok := parseChainAddr(hop)
		if !ok {
			// A malformed hop makes everything to its left unverifiable.
			// Stop and let the caller fall back to the peer.
			return netip.Addr{}, false
		}

		leftmost, haveLeftmost = addr, true

		if !ref.isTrusted(addr) {
			return addr, true
		}
	}

	// Every hop was a proxy we control. The leftmost is the closest thing to
	// an origin the chain offers, and a trusted hop put it there.
	return leftmost, haveLeftmost
}

func (ref *ClientIPResolver) isTrusted(addr netip.Addr) bool {
	unmapped := addr.Unmap()

	for _, prefix := range ref.trusted {
		if prefix.Contains(unmapped) {
			return true
		}
	}

	return false
}

// parseChainAddr parses one X-Forwarded-For entry. Some proxies append a port
// and some quote IPv6 in brackets, so both forms are accepted.
func parseChainAddr(entry string) (netip.Addr, bool) {
	trimmed := strings.TrimSpace(entry)
	if trimmed == "" {
		return netip.Addr{}, false
	}

	if addr, err := netip.ParseAddr(trimmed); err == nil {
		return addr.Unmap().WithZone(""), true
	}

	if addrPort, err := netip.ParseAddrPort(trimmed); err == nil {
		return addrPort.Addr().Unmap().WithZone(""), true
	}

	return netip.Addr{}, false
}

// peerAddr extracts the IP from an "ip:port" RemoteAddr.
func peerAddr(remoteAddr string) (netip.Addr, bool) {
	if addrPort, err := netip.ParseAddrPort(remoteAddr); err == nil {
		return addrPort.Addr().Unmap().WithZone(""), true
	}

	// Some transports (and httptest) hand over a bare address.
	if addr, err := netip.ParseAddr(remoteAddr); err == nil {
		return addr.Unmap().WithZone(""), true
	}

	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return netip.Addr{}, false
	}

	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}

	return addr.Unmap().WithZone(""), true
}
