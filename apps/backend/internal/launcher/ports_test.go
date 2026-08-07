package launcher

import (
	"net"
	"os"
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

func TestResolveWebPortPrecedence(t *testing.T) {
	// Unset first: an empty KANDEV_WEB_PORT is a value and errors like the
	// backend port env vars do.
	old, hadOld := os.LookupEnv("KANDEV_WEB_PORT")
	if hadOld {
		t.Cleanup(func() { t.Setenv("KANDEV_WEB_PORT", old) })
	} else {
		t.Cleanup(func() { _ = os.Unsetenv("KANDEV_WEB_PORT") })
	}
	_ = os.Unsetenv("KANDEV_WEB_PORT")

	if got, err := resolveWebPort(Options{}); err != nil || got != 0 {
		t.Fatalf("resolveWebPort(unset) = %d, %v, want 0", got, err)
	}

	t.Setenv("KANDEV_WEB_PORT", "37001")
	if got, err := resolveWebPort(Options{}); err != nil || got != 37001 {
		t.Fatalf("resolveWebPort(env) = %d, %v, want 37001", got, err)
	}
	if got, err := resolveWebPort(Options{WebPort: 37430}); err != nil || got != 37430 {
		t.Fatalf("resolveWebPort(flag) = %d, %v, want 37430", got, err)
	}
	if got := webPortSource(Options{WebPort: 37430}); got != "--web-internal-port" {
		t.Fatalf("webPortSource(flag) = %q", got)
	}
	if got := webPortSource(Options{}); got != "KANDEV_WEB_PORT" {
		t.Fatalf("webPortSource(env) = %q", got)
	}
	t.Setenv("KANDEV_WEB_PORT", "")
	if got := webPortSource(Options{}); got != "KANDEV_WEB_PORT" {
		t.Fatalf("webPortSource(empty env) = %q, want the env var name", got)
	}
	if _, err := resolveWebPort(Options{}); err == nil {
		t.Fatal("resolveWebPort(empty env) = nil error, want a ParseError")
	}
}

func TestPickDevPortsDefaultsToDistinctPreferredPorts(t *testing.T) {
	cfg, err := pickDevPorts(0, "", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]bool{cfg.BackendPort: true, cfg.WebPort: true, cfg.AgentctlPort: true}
	if len(seen) != 3 {
		t.Fatalf("ports not distinct: %+v", cfg)
	}
	// The preferred defaults are used when bindable; a busy default falls back
	// to a random free port. Assert the default when it is free so the test is
	// meaningful on machines where the ports are available.
	if canBind(defaultBackendPort) && cfg.BackendPort != defaultBackendPort {
		t.Fatalf("backend port = %d, want %d", cfg.BackendPort, defaultBackendPort)
	}
	if canBind(defaultWebPort) && cfg.WebPort != defaultWebPort {
		t.Fatalf("web port = %d, want %d", cfg.WebPort, defaultWebPort)
	}
}

func TestPickDevPortsRejectsOccupiedExplicitWebPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	port := ln.Addr().(*net.TCPAddr).Port
	_, err = pickDevPorts(0, "", port, "--web-internal-port")
	if err == nil {
		t.Fatalf("pickDevPorts succeeded while the requested web port was occupied")
	}
	if !strings.Contains(err.Error(), "web port "+strconv.Itoa(port)) {
		t.Fatalf("error = %v, want the occupied web port", err)
	}
	if !strings.Contains(err.Error(), "from --web-internal-port") {
		t.Fatalf("error = %v, want the configuration source", err)
	}
}

func TestPickDevPortsBackendAndWebStayDistinct(t *testing.T) {
	cfg, err := pickDevPorts(0, "", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	// If the default web port collided with the backend port, pickAvailablePortExcept
	// must have shifted it rather than returning a duplicate.
	if cfg.WebPort == cfg.BackendPort {
		t.Fatalf("web port %d collides with backend port %d", cfg.WebPort, cfg.BackendPort)
	}
	if cfg.AgentctlPort == cfg.BackendPort || cfg.AgentctlPort == cfg.WebPort {
		t.Fatalf("agentctl port %d collides with another service port: %+v", cfg.AgentctlPort, cfg)
	}
}

func TestPickPortsWithoutWebPortMatchesStartSemantics(t *testing.T) {
	cfg, err := pickPorts(0, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WebPort != 0 {
		t.Fatalf("pickPorts set WebPort = %d, want 0 for start/run", cfg.WebPort)
	}
}
