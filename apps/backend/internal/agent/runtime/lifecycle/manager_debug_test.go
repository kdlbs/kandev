package lifecycle

import (
	"context"
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

type debugExportResult struct {
	body io.ReadCloser
	err  error
}

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

func TestManagerExportACPDebugHoldsClientLeaseDuringRequest(t *testing.T) {
	requestEntered := make(chan struct{})
	releaseRequest := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(requestEntered)
		<-releaseRequest
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
	execution := &AgentExecution{
		ID: "execution-1", SessionID: "task-session-1", ACPSessionID: "acp-session-1",
		agentctl: agentctlclient.NewClient(parsed.Hostname(), port, log),
	}
	mgr := newTestManager(t)
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}
	result := make(chan debugExportResult, 1)
	go func() {
		body, exportErr := mgr.ExportACPDebug(context.Background(), "task-session-1", 4096)
		result <- debugExportResult{body: body, err: exportErr}
	}()
	<-requestEntered
	if execution.agentctlLifecycleMu.TryLock() {
		execution.agentctlLifecycleMu.Unlock()
		close(releaseRequest)
		got := <-result
		if got.body != nil {
			_ = got.body.Close()
		}
		t.Fatal("agentctl operation did not hold the client replacement lease")
	}
	close(releaseRequest)
	got := <-result
	if got.err != nil {
		t.Fatal(got.err)
	}
	_ = got.body.Close()
}

func TestManagerExportACPDebugHoldsClientLeaseUntilBodyClose(t *testing.T) {
	headersSent := make(chan struct{})
	releaseBody := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(headersSent)
		<-releaseBody
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
	execution := &AgentExecution{
		ID: "execution-1", SessionID: "task-session-1", ACPSessionID: "acp-session-1",
		agentctl: agentctlclient.NewClient(parsed.Hostname(), port, log),
	}
	mgr := newTestManager(t)
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}

	body, err := mgr.ExportACPDebug(context.Background(), "task-session-1", 4096)
	if err != nil {
		t.Fatal(err)
	}
	<-headersSent
	if execution.agentctlLifecycleMu.TryLock() {
		execution.agentctlLifecycleMu.Unlock()
		close(releaseBody)
		_ = body.Close()
		t.Fatal("agentctl replacement lease ended before the response body closed")
	}
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
	if !execution.agentctlLifecycleMu.TryLock() {
		close(releaseBody)
		t.Fatal("agentctl replacement lease remained held after response body close")
	}
	execution.agentctlLifecycleMu.Unlock()
	close(releaseBody)
}
