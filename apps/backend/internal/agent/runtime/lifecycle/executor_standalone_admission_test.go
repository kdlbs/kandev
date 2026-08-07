package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/subproc"
)

func TestStandaloneExecutorSubprocessAdmission(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/debug/subprocess-admission" {
			t.Fatalf("path = %q, want admission route", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(subproc.Snapshot{Pool: "git", Capacity: 6})
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
	client := agentctl.NewControlClient(host, portNumber, logger.Default())
	executor := NewStandaloneExecutor(client, host, portNumber, logger.Default())

	snapshot, err := executor.SubprocessAdmission(context.Background())
	if err != nil {
		t.Fatalf("SubprocessAdmission() error = %v", err)
	}
	if snapshot.Pool != "git" || snapshot.Capacity != 6 {
		t.Fatalf("snapshot = %+v, want control-client snapshot", snapshot)
	}
}
