package fetch

import (
	"context"
	"fmt"
	"net"
	"syscall"
)

// isBlockedIP reports whether an IP must not be dialed: loopback, private,
// link-local, unique-local, multicast, or unspecified addresses. This is the
// SSRF guard — it keeps user-supplied URLs from reaching internal services
// (localhost, RFC1918 ranges, and cloud metadata at 169.254.169.254).
func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return true
	}
	return false
}

// guardedControl is a net.Dialer Control hook that rejects connections to
// blocked IPs. It runs after DNS resolution on the actual address being dialed,
// so it also defends against DNS-rebinding to an internal IP.
func guardedControl(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("parse dial address: %w", err)
	}
	if isBlockedIP(net.ParseIP(host)) {
		return fmt.Errorf("blocked target address %s", host)
	}
	return nil
}

// guardedDialContext wraps a base dialer with the SSRF Control hook.
func guardedDialContext(d *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	d.Control = guardedControl
	return d.DialContext
}
