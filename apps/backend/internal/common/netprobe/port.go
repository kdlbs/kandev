// Package netprobe holds the single contract for deciding whether a local
// TCP port is occupied.
//
// It lives in internal/common because the two callers sit on opposite sides
// of a dependency boundary -- the process launcher that picks the backend,
// web, and agentctl ports, and the runtime-tier launcher that starts a
// standalone agentctl -- and a port answered "free" by one and "busy" by the
// other is how a backend ends up talking to a control server it did not
// start.
package netprobe

import (
	"fmt"
	"net"
	"strconv"
	"time"
)

// ConnectProbeTimeout bounds each loopback connect probe so a silently
// dropped SYN (for example under WSL2 mirrored networking to an unbound
// loopback port) cannot hang port selection. A timed-out connect counts as
// "nothing listening".
const ConnectProbeTimeout = 500 * time.Millisecond

// PortAvailable reports whether a port is free. It is free only when a
// dual-stack loopback connect finds nothing listening AND a fresh loopback
// bind succeeds.
//
// The connect probe is required because a server binds the wildcard address
// (0.0.0.0 and [::]): on macOS/BSD a specific 127.0.0.1 bind succeeds against
// an active wildcard listener (Go also sets SO_REUSEADDR), so a bind-only
// check reports a busy port as free. The bind probe is retained because it
// catches reservations a connect misses (Windows phantom reservations,
// TIME_WAIT).
func PortAvailable(port int) bool {
	if HasListener(port) {
		return false
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// HasListener reports whether anything answers a loopback connect on either
// the IPv4 or IPv6 loopback address. A server may hold only the IPv6 wildcard
// socket, so both families are probed.
func HasListener(port int) bool {
	for _, host := range []string{"127.0.0.1", "::1"} {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), ConnectProbeTimeout)
		if err == nil {
			_ = conn.Close()
			return true
		}
	}
	return false
}
