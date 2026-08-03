package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/subproc"
)

func TestControlClientSubprocessAdmission(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/debug/subprocess-admission" {
			t.Errorf("request = %s %s, want GET /api/v1/debug/subprocess-admission", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer admission-secret" {
			t.Errorf("authorization = %q, want bearer token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(subproc.Snapshot{
			Pool:     "git",
			Capacity: 4,
			Inflight: 1,
			Waiters:  2,
			Classes:  map[string]subproc.ClassSnapshot{"interactive": {Waiters: 2}},
		})
	}))
	t.Cleanup(server.Close)

	host, port, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split server address: %v", err)
	}
	var portNumber int
	if _, err := fmt.Sscanf(port, "%d", &portNumber); err != nil {
		t.Fatalf("parse server port: %v", err)
	}
	client := NewControlClient(host, portNumber, logger.Default(), WithControlAuthToken("admission-secret"))

	snapshot, err := client.SubprocessAdmission(context.Background())
	if err != nil {
		t.Fatalf("SubprocessAdmission() error = %v", err)
	}
	if snapshot.Pool != "git" || snapshot.Capacity != 4 || snapshot.Classes["interactive"].Waiters != 2 {
		t.Fatalf("snapshot = %+v, want decoded admission snapshot", snapshot)
	}
}
