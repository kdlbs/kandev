package controller

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// installScriptAgent extends testAgent so we can set a non-empty install script.
type installScriptAgent struct {
	testAgent
	script string
}

func (a *installScriptAgent) InstallScript() string { return a.script }

// captureBroadcaster captures all WS messages emitted during the test.
type captureBroadcaster struct {
	mu  sync.Mutex
	msg []*ws.Message
}

func (b *captureBroadcaster) Broadcast(m *ws.Message) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.msg = append(b.msg, m)
}

func (b *captureBroadcaster) actions() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, len(b.msg))
	for i, m := range b.msg {
		out[i] = m.Action
	}
	return out
}

// withStubStreamingRunner swaps streamingInstallRunner for the duration of
// the test. The stub invokes onChunk synchronously to make ordering deterministic.
func withStubStreamingRunner(t *testing.T, fn func(ctx context.Context, script string, onChunk func(string)) error) {
	t.Helper()
	prev := streamingInstallRunner
	streamingInstallRunner = fn
	t.Cleanup(func() { streamingInstallRunner = prev })
}

func newInstallController(t *testing.T, ag agents.Agent) (*Controller, *captureBroadcaster) {
	t.Helper()
	ctrl := newTestController(map[string]agents.Agent{ag.ID(): ag})
	hub := &captureBroadcaster{}
	ctrl.SetJobBroadcaster(hub)
	return ctrl, hub
}

// waitForStatus polls until the job hits one of the terminal statuses or the
// deadline expires.
func waitForStatus(t *testing.T, ctrl *Controller, jobID string, want ...dto.InstallJobStatus) *dto.InstallJobDTO {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap, ok := ctrl.GetInstallJob(jobID)
		if ok {
			for _, w := range want {
				if snap.Status == w {
					return snap
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach status %v in time", jobID, want)
	return nil
}

func TestEnqueueInstall_StreamsAndSucceeds(t *testing.T) {
	ag := &installScriptAgent{
		testAgent: testAgent{id: "test-agent", name: "test-agent", enabled: true},
		script:    "echo ok",
	}
	ctrl, hub := newInstallController(t, ag)

	withStubStreamingRunner(t, func(_ context.Context, _ string, onChunk func(string)) error {
		onChunk("installing...\n")
		onChunk("done\n")
		return nil
	})

	snap, err := ctrl.EnqueueInstall("test-agent")
	if err != nil {
		t.Fatalf("EnqueueInstall() error = %v", err)
	}
	if snap.JobID == "" {
		t.Fatal("expected job_id")
	}

	final := waitForStatus(t, ctrl, snap.JobID, dto.InstallJobStatusSucceeded)
	if !strings.Contains(final.Output, "installing...") || !strings.Contains(final.Output, "done") {
		t.Errorf("output missing stream chunks, got %q", final.Output)
	}
	if final.ExitCode == nil || *final.ExitCode != 0 {
		t.Errorf("ExitCode = %v, want 0", final.ExitCode)
	}

	// Must have broadcast a started and a finished message; output messages
	// land in between but exact count depends on the flush timing.
	actions := hub.actions()
	if len(actions) < 2 {
		t.Fatalf("expected ≥2 broadcasts, got %v", actions)
	}
	if actions[0] != ws.ActionAgentInstallStarted {
		t.Errorf("first action = %s, want %s", actions[0], ws.ActionAgentInstallStarted)
	}
	if actions[len(actions)-1] != ws.ActionAgentInstallFinished {
		t.Errorf("last action = %s, want %s", actions[len(actions)-1], ws.ActionAgentInstallFinished)
	}
}

func TestEnqueueInstall_Failure(t *testing.T) {
	ag := &installScriptAgent{
		testAgent: testAgent{id: "test-agent", name: "test-agent", enabled: true},
		script:    "exit 1",
	}
	ctrl, _ := newInstallController(t, ag)

	withStubStreamingRunner(t, func(_ context.Context, _ string, onChunk func(string)) error {
		onChunk("npm ERR! boom\n")
		return errors.New("exit status 1")
	})

	snap, err := ctrl.EnqueueInstall("test-agent")
	if err != nil {
		t.Fatalf("EnqueueInstall() error = %v", err)
	}

	final := waitForStatus(t, ctrl, snap.JobID, dto.InstallJobStatusFailed)
	if final.Error == "" {
		t.Error("Error empty on failed install")
	}
	if !strings.Contains(final.Output, "npm ERR!") {
		t.Errorf("Output missing stderr, got %q", final.Output)
	}
}

func TestEnqueueInstall_IdempotentWhileRunning(t *testing.T) {
	ag := &installScriptAgent{
		testAgent: testAgent{id: "test-agent", name: "test-agent", enabled: true},
		script:    "sleep",
	}
	ctrl, _ := newInstallController(t, ag)

	// Block the runner so the first job stays in 'running' while we call
	// EnqueueInstall a second time.
	release := make(chan struct{})
	withStubStreamingRunner(t, func(ctx context.Context, _ string, _ func(string)) error {
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	first, err := ctrl.EnqueueInstall("test-agent")
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	second, err := ctrl.EnqueueInstall("test-agent")
	if err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
	if first.JobID != second.JobID {
		t.Errorf("expected same job_id, got %s and %s", first.JobID, second.JobID)
	}

	// Release the runner and wait for the goroutine to finish before the test
	// returns. Otherwise withStubStreamingRunner's restore cleanup races with
	// the still-running goroutine's read of streamingInstallRunner.
	close(release)
	waitForStatus(t, ctrl, first.JobID, dto.InstallJobStatusSucceeded, dto.InstallJobStatusFailed)
}

func TestEnqueueInstall_AgentNotFound(t *testing.T) {
	ag := &installScriptAgent{
		testAgent: testAgent{id: "test-agent", name: "test-agent", enabled: true},
		script:    "echo ok",
	}
	ctrl, _ := newInstallController(t, ag)

	_, err := ctrl.EnqueueInstall("missing")
	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("err = %v, want ErrAgentNotFound", err)
	}
}

func TestEnqueueInstall_EmptyScript(t *testing.T) {
	ag := &installScriptAgent{
		testAgent: testAgent{id: "test-agent", name: "test-agent", enabled: true},
		script:    "   ",
	}
	ctrl, _ := newInstallController(t, ag)

	_, err := ctrl.EnqueueInstall("test-agent")
	if !errors.Is(err, ErrInstallScriptEmpty) {
		t.Fatalf("err = %v, want ErrInstallScriptEmpty", err)
	}
}

func TestEnqueueInstall_NoJobStore(t *testing.T) {
	ag := &installScriptAgent{
		testAgent: testAgent{id: "test-agent", name: "test-agent", enabled: true},
		script:    "echo ok",
	}
	// Construct without calling SetJobBroadcaster.
	ctrl := newTestController(map[string]agents.Agent{ag.ID(): ag})

	_, err := ctrl.EnqueueInstall("test-agent")
	if !errors.Is(err, ErrJobStoreUnavailable) {
		t.Fatalf("err = %v, want ErrJobStoreUnavailable", err)
	}
}

func TestRingBuffer_DropsOldestOnLineBoundary(t *testing.T) {
	rb := newRingBuffer(20)
	_, _ = rb.Write([]byte("first line\n"))
	_, _ = rb.Write([]byte("second line\n"))
	_, _ = rb.Write([]byte("third\n"))
	got := rb.String()
	// "first line\n" must have been evicted; the buffer holds the tail starting
	// after the next newline boundary.
	if strings.Contains(got, "first") {
		t.Errorf("ring buffer should have evicted 'first', got %q", got)
	}
	if !strings.Contains(got, "third") {
		t.Errorf("ring buffer missing newest write, got %q", got)
	}
}
