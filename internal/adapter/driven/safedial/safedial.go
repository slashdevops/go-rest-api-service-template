package safedial

import (
	"fmt"
	"net"
	"net/netip"
	"syscall"
)

// RefusedAddressError is the dial refused because of where it was going.
type RefusedAddressError struct {
	Address string
	Reason  string
}

func (e *RefusedAddressError) Error() string {
	return fmt.Sprintf("outbound connection to %s refused: %s", e.Address, e.Reason)
}

// Policy decides which destinations may be dialled.
type Policy struct {
	// AllowPrivate admits loopback, private (RFC 1918, ULA) and unspecified
	// addresses. Link-local is never admitted.
	AllowPrivate bool
}

// Check classifies one resolved address.
func (p Policy) Check(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}

	ip, err := netip.ParseAddr(host)
	if err != nil {
		return &RefusedAddressError{Address: address, Reason: "not an IP address after resolution"}
	}

	ip = ip.Unmap()

	switch {
	case ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast():
		return &RefusedAddressError{Address: address, Reason: "link-local address (cloud metadata lives here)"}
	case ip.IsMulticast():
		return &RefusedAddressError{Address: address, Reason: "multicast address"}
	case p.AllowPrivate:
		return nil
	case ip.IsLoopback():
		return &RefusedAddressError{Address: address, Reason: "loopback address; set http.client.allow.private.addresses for a local engine"}
	case ip.IsPrivate():
		return &RefusedAddressError{Address: address, Reason: "private address; set http.client.allow.private.addresses for an on-premises engine"}
	case ip.IsUnspecified():
		return &RefusedAddressError{Address: address, Reason: "unspecified address"}
	}

	return nil
}

// Control is a [net.Dialer.Control] that applies the policy to the address
// the dialer is about to connect to, after name resolution.
func (p Policy) Control(_, address string, _ syscall.RawConn) error {
	return p.Check(address)
}
