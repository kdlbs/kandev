package utility

import (
	"context"
	"go.uber.org/zap"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type drainingProbeExecutor struct{ entered, cancelled, finish chan struct{} }

func (e *drainingProbeExecutor) Probe(ctx context.Context, _ *ProbeRequest) (*ProbeResponse, error) {
	close(e.entered)
	<-ctx.Done()
	close(e.cancelled)
	<-e.finish
	return &ProbeResponse{Success: false}, nil
}
func (*drainingProbeExecutor) Execute(context.Context, *PromptRequest) (*PromptResponse, error) {
	return nil, nil
}

func TestManagedRuntimeProbeTeardownCancelsAndDrainsBeforeCleanup(t *testing.T) {
	e := &drainingProbeExecutor{make(chan struct{}), make(chan struct{}), make(chan struct{})}
	h := NewHandler("", zap.NewNop())
	h.acpExecutor = e
	router := newProbeDiagnosticRouter(t, h)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); close(e.finish) })
	request := httptest.NewRequest(http.MethodPost, "/api/v1/inference/probe", strings.NewReader(`{"agent_id":"test"}`)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	requestDone := make(chan struct{})
	go func() { router.ServeHTTP(httptest.NewRecorder(), request); close(requestDone) }()
	<-e.entered
	closed := make(chan error, 1)
	go func() { closed <- h.Close() }()
	select {
	case <-e.cancelled:
	case <-time.After(time.Second):
		t.Fatal("teardown did not cancel active probe")
	}
	select {
	case <-closed:
		t.Fatal("teardown returned before the probe released its resources")
	default:
	}
	e.finish <- struct{}{}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	<-requestDone
	rec := postProbeRequest(t, router, ProbeRequest{AgentID: "test"})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("probe admitted after teardown: %d", rec.Code)
	}
}
