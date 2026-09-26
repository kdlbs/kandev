//go:build !windows

package api

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// installPortDetectorShim puts a fake `ss`/`lsof` on PATH so the detectors can
// be driven with fixed output. The real commands report whatever the host
// happens to be listening on, which is neither stable nor assertable.
func installPortDetectorShim(t *testing.T, name, stdout string, exitCode int) {
	t.Helper()
	binDir := t.TempDir()
	script := "#!/bin/sh\n"
	if stdout != "" {
		script += "cat <<'KANDEV_EOF'\n" + stdout + "\nKANDEV_EOF\n"
	}
	script += "exit " + strconv.Itoa(exitCode) + "\n"
	if err := os.WriteFile(filepath.Join(binDir, name), []byte(script), 0o755); err != nil {
		t.Fatalf("write %s shim: %v", name, err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func portsByNumber(t *testing.T, ports []ListeningPort) map[int]ListeningPort {
	t.Helper()
	byPort := make(map[int]ListeningPort, len(ports))
	for _, port := range ports {
		if _, dup := byPort[port.Port]; dup {
			t.Errorf("port %d reported twice: %+v", port.Port, ports)
		}
		byPort[port.Port] = port
	}
	return byPort
}

const ssShimOutput = `LISTEN 0      511          0.0.0.0:3000       0.0.0.0:*    users:(("next-server (v1",pid=16186,fd=24))
LISTEN 0      511          0.0.0.0:3000       0.0.0.0:*    users:(("next-server (v1",pid=16187,fd=25))
LISTEN 0      128        127.0.0.1:22         0.0.0.0:*    users:(("sshd",pid=900,fd=3))
LISTEN 0      4096             *:9000             *:*      users:(("vite",pid=17000,fd=11))
LISTEN 0      4096       127.0.0.1:5432       0.0.0.0:*
LISTEN 0      4096
LISTEN 0      4096          garbage       0.0.0.0:*    users:(("bogus",pid=1,fd=1))`

// TestDetectFromSS_ParsesListeners covers every branch of the `ss` parser at
// once: process-name extraction, first-wins de-duplication, the privileged-port
// filter, wildcard-host normalisation, a row with no process column, and two
// malformed rows that must be skipped rather than crash the sweep.
func TestDetectFromSS_ParsesListeners(t *testing.T) {
	installPortDetectorShim(t, "ss", ssShimOutput, 0)

	ports, err := detectFromSS()
	if err != nil {
		t.Fatalf("detectFromSS: %v", err)
	}
	byPort := portsByNumber(t, ports)

	if len(byPort) != 3 {
		t.Fatalf("detected %d ports, want 3 (3000, 9000, 5432): %+v", len(byPort), ports)
	}
	if got := byPort[3000]; got.Process != "next-server (v1" || got.Address != "0.0.0.0" {
		t.Errorf("port 3000 = %+v, want the quoted process name and 0.0.0.0", got)
	}
	if got := byPort[9000]; got.Address != defaultListenAddr || got.Process != "vite" {
		t.Errorf("port 9000 = %+v, want %q normalised to %q", got, "*", defaultListenAddr)
	}
	if got := byPort[5432]; got.Address != "127.0.0.1" || got.Process != "" {
		t.Errorf("port 5432 = %+v, want a loopback listener with no process name", got)
	}
	if _, listed := byPort[22]; listed {
		t.Errorf("port 22 was reported; ports below 1024 are filtered out")
	}
}

// TestDetectFromSS_ReportsCommandFailure covers the error branch, which is what
// pushes detectListeningPorts onto its /proc/net/tcp fallback.
func TestDetectFromSS_ReportsCommandFailure(t *testing.T) {
	installPortDetectorShim(t, "ss", "", 1)

	ports, err := detectFromSS()
	if err == nil {
		t.Fatalf("detectFromSS succeeded with a failing command: %+v", ports)
	}
	if !strings.Contains(err.Error(), "ss failed") {
		t.Errorf("error = %q, want it to name the failed command", err)
	}
}

const lsofShimOutput = `cnext-server
n*:3000
n127.0.0.1:3000
csshd
n127.0.0.1:22
cvite
n[::1]:9000
nnot-an-address`

// TestDetectFromLsof_ParsesListeners covers the non-Linux fallback parser: the
// `c` record carries the process name for every `n` record that follows it, and
// the same filtering and de-duplication rules apply as for `ss` (port 3000 is
// bound on two addresses and must be reported once, keeping the first).
func TestDetectFromLsof_ParsesListeners(t *testing.T) {
	installPortDetectorShim(t, "lsof", lsofShimOutput, 0)

	ports, err := detectFromLsof()
	if err != nil {
		t.Fatalf("detectFromLsof: %v", err)
	}
	byPort := portsByNumber(t, ports)

	if len(byPort) != 2 {
		t.Fatalf("detected %d ports, want 2 (3000, 9000): %+v", len(byPort), ports)
	}
	if got := byPort[3000]; got.Process != "next-server" {
		t.Errorf("port 3000 = %+v, want the preceding `c` record's process name", got)
	}
	if got := byPort[9000]; got.Process != "vite" {
		t.Errorf("port 9000 = %+v, want the process name to follow the newest `c` record", got)
	}
	if _, listed := byPort[22]; listed {
		t.Errorf("port 22 was reported; ports below 1024 are filtered out")
	}
}

// TestDetectFromLsof_ReportsCommandFailure covers the last-resort error, which
// is the only way handleListPorts can answer 500.
func TestDetectFromLsof_ReportsCommandFailure(t *testing.T) {
	installPortDetectorShim(t, "lsof", "", 3)

	if _, err := detectFromLsof(); err == nil || !strings.Contains(err.Error(), "lsof failed") {
		t.Errorf("error = %v, want an lsof failure", err)
	}
}

// TestDetectFromProcNet_FindsRealListener covers the container fallback against
// the kernel's own table. A socket this test just opened has to show up, which
// is the one assertion that proves the hex parsing, the LISTEN-state filter and
// the file walk all agree with what /proc/net/tcp actually contains.
func TestDetectFromProcNet_FindsRealListener(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("/proc/net/tcp is Linux-only")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	_, portStr, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split listener address: %v", err)
	}
	want, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse listener port: %v", err)
	}

	ports, err := detectFromProcNet()
	if err != nil {
		t.Fatalf("detectFromProcNet: %v", err)
	}
	byPort := portsByNumber(t, ports)
	if _, ok := byPort[want]; !ok {
		t.Fatalf("port %d is missing from %+v", want, ports)
	}
	for _, port := range ports {
		if port.Port < 1024 {
			t.Errorf("port %d is below the 1024 floor: %+v", port.Port, port)
		}
	}
}

// TestHandleListPorts_ReturnsDetectedPorts drives the HTTP endpoint end to end
// with both detectors shimmed, so the assertion holds whichever one the host
// platform selects.
func TestHandleListPorts_ReturnsDetectedPorts(t *testing.T) {
	installPortDetectorShim(t, "ss", ssShimOutput, 0)
	installPortDetectorShim(t, "lsof", lsofShimOutput, 0)
	srv := newTestServer(t)

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ports", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}

	var body struct {
		Ports []ListeningPort `json:"ports"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode ports: %v", err)
	}
	byPort := portsByNumber(t, body.Ports)
	if _, ok := byPort[3000]; !ok {
		t.Errorf("ports = %+v, want the shimmed listener on 3000", body.Ports)
	}
	if _, ok := byPort[22]; ok {
		t.Errorf("ports = %+v, want privileged ports filtered out", body.Ports)
	}
}
