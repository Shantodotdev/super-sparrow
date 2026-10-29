package httpserver

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// clientIP returns the address of the client that sent r. When the connection comes from a
// trusted proxy, it walks X-Forwarded-For from the right and returns the first address that is
// not a trusted proxy, since only the entries trusted proxies appended can be believed.
func clientIP(r *http.Request, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	remote, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	remote = remote.Unmap()
	if !isTrusted(remote, trusted) {
		return remote.String()
	}

	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	client := remote
	for i := len(hops) - 1; i >= 0; i-- {
		hop, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		client = hop.Unmap()
		if !isTrusted(client, trusted) {
			break
		}
	}

	return client.String()
}

func isTrusted(addr netip.Addr, trusted []netip.Prefix) bool {
	for _, prefix := range trusted {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
