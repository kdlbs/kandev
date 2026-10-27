package lifecycle

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/agent/nomad"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
)

type failingForwardTransport struct{}

func (failingForwardTransport) Run(context.Context, string) (string, string, error) {
	return "", "", nil
}
func (failingForwardTransport) RunStdin(context.Context, string, io.Reader) (string, string, error) {
	return "", "", nil
}
func (failingForwardTransport) Upload(context.Context, string, []byte, os.FileMode) error { return nil }
func (failingForwardTransport) Forward(int, *logger.Logger) (nomadPortForward, error) {
	return nil, errors.New("forward unavailable")
}
func (failingForwardTransport) Close() error { return nil }

func attachWithTransientForwardFailure(t *testing.T, createdNow bool) int32 {
	t.Helper()
	var deletes int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			atomic.AddInt32(&deletes, 1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	_, portText, _ := net.SplitHostPort(server.Listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	log := newSeederTestLogger(t)
	conn := &nomadAgentctlConnection{
		transport:     failingForwardTransport{},
		controlClient: agentctl.NewControlClient("127.0.0.1", port, log),
	}
	state := &nomad.EnvironmentState{}
	req := &ExecutorCreateRequest{InstanceID: "exec-1"}
	created := &agentctl.InstanceInfo{ID: "exec-1", Port: 22342}
	err := conn.attachNomadAgentctlInstance(context.Background(), created, createdNow, &agentctl.IdentityInfo{ServerIdentity: "id"}, state, req, log)
	if err == nil {
		t.Fatal("expected forward failure")
	}
	return atomic.LoadInt32(&deletes)
}

func TestAttachNomadAgentctlInstance_RecoveryFailureKeepsExistingInstance(t *testing.T) {
	if got := attachWithTransientForwardFailure(t, false); got != 0 {
		t.Fatalf("recovery deleted a live pre-existing instance on a transient failure (%d deletes)", got)
	}
}

func TestAttachNomadAgentctlInstance_FailureRollsBackInstanceCreatedByThisAttempt(t *testing.T) {
	if got := attachWithTransientForwardFailure(t, true); got != 1 {
		t.Fatalf("created instance deletes = %d, want 1", got)
	}
}
