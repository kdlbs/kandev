package lifecycle

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

func TestManagerExportACPDebugUsesExecutionACPConversationID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/debug/acp/acp-session-1/export" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte("zip"))
	}))
	defer server.Close()

	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}

	mgr := newTestManager(t)
	if err := mgr.executionStore.Add(&AgentExecution{
		ID:           "execution-1",
		SessionID:    "task-session-1",
		ACPSessionID: "acp-session-1",
		agentctl:     agentctlclient.NewClient(parsed.Hostname(), port, log),
	}); err != nil {
		t.Fatal(err)
	}

	body, err := mgr.ExportACPDebug(t.Context(), "task-session-1", 4096)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "zip" {
		t.Fatalf("body = %q", data)
	}
}
