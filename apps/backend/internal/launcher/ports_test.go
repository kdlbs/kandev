package launcher

import (
	"net"
	"strconv"
	"strings"
	"testing"
)

func TestPickAvailablePortExceptSkipsUsedPreferredPort(t *testing.T) {
	port, err := pickAvailablePortExcept(defaultBackendPort, map[int]bool{defaultBackendPort: true})
	if err != nil {
		t.Fatal(err)
	}
	if port == defaultBackendPort {
		t.Fatalf("picked reserved preferred port %d", port)
	}
}

func TestPickPortsRejectsOccupiedExplicitBackendPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	port := ln.Addr().(*net.TCPAddr).Port
	_, err = pickPorts(port, "KANDEV_PORT")
	if err == nil {
		t.Fatalf("pickPorts(%d) succeeded while the requested port was occupied", port)
	}
	if !strings.Contains(err.Error(), "backend port "+strconv.Itoa(port)) {
		t.Fatalf("error = %v, want the occupied backend port", err)
	}
	if !strings.Contains(err.Error(), "from KANDEV_PORT") {
		t.Fatalf("error = %v, want the configuration source", err)
	}
}

func TestBackendPortSourceUsesLauncherPrecedence(t *testing.T) {
	t.Setenv("KANDEV_BACKEND_PORT", "4000")
	t.Setenv("KANDEV_PORT", "5000")
	if got := backendPortSource(Options{BackendPort: 3000}); got != "--port" {
		t.Fatalf("backendPortSource(cli) = %q, want --port", got)
	}
	if got := backendPortSource(Options{}); got != "KANDEV_BACKEND_PORT" {
		t.Fatalf("backendPortSource(specific env) = %q, want KANDEV_BACKEND_PORT", got)
	}
	t.Setenv("KANDEV_BACKEND_PORT", "")
	if got := backendPortSource(Options{}); got != "KANDEV_BACKEND_PORT" {
		t.Fatalf("backendPortSource(empty specific env) = %q, want KANDEV_BACKEND_PORT", got)
	}
}
