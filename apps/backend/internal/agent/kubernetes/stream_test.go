package kubernetes

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type recordingPodExecutor struct {
	request ExecRequest
	err     error
	calls   int
}

func (e *recordingPodExecutor) Exec(_ context.Context, request ExecRequest) error {
	e.calls++
	e.request = request
	return e.err
}

type recordingPortForwarder struct {
	request PortForwardRequest
	session PortForwardSession
	err     error
	calls   int
}

func (f *recordingPortForwarder) Forward(_ context.Context, request PortForwardRequest) (PortForwardSession, error) {
	f.calls++
	f.request = request
	return f.session, f.err
}

type fakeForwardSession struct{}

func (fakeForwardSession) LocalPort() uint16      { return 43123 }
func (fakeForwardSession) Ready() <-chan struct{} { return make(chan struct{}) }
func (fakeForwardSession) Done() <-chan error     { return make(chan error) }
func (fakeForwardSession) Close() error           { return nil }

func TestStreamOperationsExecDelegatesValidatedRequest(t *testing.T) {
	t.Parallel()

	executor := &recordingPodExecutor{}
	streams := NewStreamOperations(executor, nil)
	request := ExecRequest{Namespace: "agents", Pod: "pod-1", Container: "kandev-agent", Command: []string{"mkdir", "-p", "/run/kandev"}}
	if err := streams.Exec(context.Background(), request); err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if executor.calls != 1 || executor.request.Pod != request.Pod || executor.request.Container != request.Container {
		t.Fatalf("executor request = %#v, calls %d", executor.request, executor.calls)
	}
}

func TestStreamOperationsForwardDefaultsLoopbackAndDelegates(t *testing.T) {
	t.Parallel()

	forwarder := &recordingPortForwarder{session: fakeForwardSession{}}
	streams := NewStreamOperations(nil, forwarder)
	session, err := streams.Forward(context.Background(), PortForwardRequest{
		Namespace: "agents", Pod: "pod-1", RemotePort: uint16(DefaultAgentctlPort),
	})
	if err != nil {
		t.Fatalf("Forward() error = %v", err)
	}
	if session.LocalPort() != 43123 || forwarder.request.LocalAddress != "127.0.0.1" || forwarder.calls != 1 {
		t.Fatalf("forward = session %v request %#v calls %d", session, forwarder.request, forwarder.calls)
	}
}

func TestStreamOperationsRejectsNonLoopbackForwardBeforeDelegation(t *testing.T) {
	t.Parallel()

	forwarder := &recordingPortForwarder{}
	streams := NewStreamOperations(nil, forwarder)
	_, err := streams.Forward(context.Background(), PortForwardRequest{
		Namespace: "agents", Pod: "pod-1", LocalAddress: "0.0.0.0", RemotePort: uint16(DefaultAgentctlPort),
	})
	assertFieldPath(t, err, "port_forward.local_address")
	if forwarder.calls != 0 {
		t.Fatal("Forward() delegated an unsafe bind address")
	}
}

func TestStreamOperationsRedactsBoundaryErrorsAndPreservesCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("exec forbidden at https://cluster.example/private Authorization: Bearer super-secret-token-value")
	executor := &recordingPodExecutor{err: cause}
	err := NewStreamOperations(executor, nil).Exec(context.Background(), ExecRequest{
		Namespace: "agents", Pod: "pod-1", Container: "kandev-agent", Command: []string{"true"},
	})
	if !errors.Is(err, cause) {
		t.Fatalf("Exec() error = %v, want wrapped cause", err)
	}
	if strings.Contains(err.Error(), "super-secret-token-value") {
		t.Fatalf("Exec() exposed credential: %q", err)
	}
	if !strings.Contains(err.Error(), "exec forbidden") || !strings.Contains(err.Error(), "https://cluster.example") {
		t.Fatalf("Exec() discarded safe causal diagnostics: %q", err)
	}
	if strings.Contains(err.Error(), "/private") {
		t.Fatalf("Exec() exposed URL path: %q", err)
	}
}

func TestStreamOperationsRedactsPortForwardErrorsAndPreservesCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("port forward forbidden at https://cluster.example/private?token=kandev_pat_super-secret-value")
	forwarder := &recordingPortForwarder{err: cause}
	_, err := NewStreamOperations(nil, forwarder).Forward(context.Background(), PortForwardRequest{
		Namespace: "agents", Pod: "pod-1", RemotePort: uint16(DefaultAgentctlPort),
	})
	if !errors.Is(err, cause) {
		t.Fatalf("Forward() error = %v, want wrapped cause", err)
	}
	if !strings.Contains(err.Error(), "port forward forbidden") || strings.Contains(err.Error(), "/private") ||
		strings.Contains(err.Error(), "super-secret-value") {
		t.Fatalf("Forward() diagnostics were not safely preserved: %q", err)
	}
}
