package restart

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestSupervisorManagerReportsUnsupportedWithoutSocket(t *testing.T) {
	mgr := NewSupervisorManager("", "")
	cap := mgr.Capability(context.Background())

	if cap.Supported {
		t.Fatal("Supported = true, want false")
	}
	if cap.Adapter != AdapterUnsupported {
		t.Fatalf("Adapter = %q, want %q", cap.Adapter, AdapterUnsupported)
	}
	if cap.Reason == "" {
		t.Fatal("Reason empty")
	}
}

func TestSupervisorManagerRequestsRestart(t *testing.T) {
	socket := filepath.Join(os.TempDir(), "kdv-supervisor-test.sock")
	_ = os.Remove(socket)
	t.Cleanup(func() { _ = os.Remove(socket) })
	requests := make(chan map[string]string, 1)
	closeServer := startFakeSupervisor(t, socket, requests)
	defer closeServer()

	mgr := NewSupervisorManager(socket, "")
	cap := mgr.Capability(context.Background())
	if !cap.Supported {
		t.Fatalf("Supported = false, reason=%q", cap.Reason)
	}
	if cap.Adapter != AdapterSupervisor {
		t.Fatalf("Adapter = %q, want %q", cap.Adapter, AdapterSupervisor)
	}

	resp, err := mgr.RequestRestart(context.Background())
	if err != nil {
		t.Fatalf("RequestRestart: %v", err)
	}
	if !resp.Accepted {
		t.Fatalf("Accepted = false, message=%q", resp.Message)
	}
	req := <-requests
	if req["action"] != "restart" {
		t.Fatalf("action = %q, want restart", req["action"])
	}
}

func startFakeSupervisor(
	t *testing.T,
	socket string,
	requests chan<- map[string]string,
) func() {
	t.Helper()
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleFakeSupervisorConn(conn, requests)
		}
	}()
	return func() {
		_ = ln.Close()
		<-done
	}
}

func handleFakeSupervisorConn(conn net.Conn, requests chan<- map[string]string) {
	defer func() { _ = conn.Close() }()
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return
	}
	var req map[string]string
	if err := json.Unmarshal(line, &req); err == nil {
		requests <- req
	}
	_ = json.NewEncoder(conn).Encode(RestartResponse{
		Accepted: true,
		Message:  "Restart accepted",
	})
}
