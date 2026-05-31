package lifecycle

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
)

// fakePassthroughRunner is a minimal stub of the passthroughRunner seam used
// to drive autoInjectInitialPromptWith in tests without spawning a real PTY.
type fakePassthroughRunner struct {
	mu sync.Mutex

	// waitErr is returned from WaitForFirstIdle. If waitBlock is true, the call
	// blocks until ctx is canceled regardless of waitErr.
	waitErr   error
	waitBlock bool

	// writeErr is returned from WriteStdin.
	writeErr error

	// captured data passed to WriteStdin (last call).
	writtenProcessID string
	writtenData      string
	writeCalled      bool

	// every chunk passed to WriteStdin, in order. Used by SubmitDelay tests that
	// need to assert the body/submit split (the single-string captures above only
	// hold the most recent write).
	writes []string
	// writeTimes records the wall-clock time of each WriteStdin call so tests can
	// assert that SubmitDelay was honored between chunks without coupling to a
	// specific clock.
	writeTimes []time.Time
}

func (f *fakePassthroughRunner) WaitForFirstIdle(ctx context.Context, processID string) error {
	if f.waitBlock {
		<-ctx.Done()
		return ctx.Err()
	}
	return f.waitErr
}

func (f *fakePassthroughRunner) WriteStdin(processID string, data string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writeCalled = true
	f.writtenProcessID = processID
	f.writtenData = data
	f.writes = append(f.writes, data)
	f.writeTimes = append(f.writeTimes, time.Now())
	return f.writeErr
}

func newAutoInjectExecution(description string) *AgentExecution {
	exec := &AgentExecution{
		ID:                   "exec-1",
		SessionID:            "sess-1",
		PassthroughProcessID: "proc-abc",
	}
	if description != "" {
		exec.Metadata = map[string]interface{}{
			"task_description": description,
		}
	}
	return exec
}

func TestAutoInject_disabled_does_nothing(t *testing.T) {
	mgr := newTestManager(t)
	runner := &fakePassthroughRunner{}

	mgr.autoInjectInitialPromptWith(runner, newAutoInjectExecution("do a thing"), agents.PassthroughConfig{
		AutoInjectPrompt: false,
		SubmitSequence:   "\r",
	})

	if runner.writeCalled {
		t.Fatalf("expected no stdin write when AutoInjectPrompt is false, got data=%q", runner.writtenData)
	}
}

func TestAutoInject_skipped_when_PromptFlag_set(t *testing.T) {
	mgr := newTestManager(t)
	runner := &fakePassthroughRunner{}

	mgr.autoInjectInitialPromptWith(runner, newAutoInjectExecution("do a thing"), agents.PassthroughConfig{
		AutoInjectPrompt: true,
		SubmitSequence:   "\r",
		PromptFlag:       agents.NewParam("--prompt", "{prompt}"),
	})

	if runner.writeCalled {
		t.Fatalf("expected no stdin write when PromptFlag is set, got data=%q", runner.writtenData)
	}
}

func TestAutoInject_skipped_when_description_empty(t *testing.T) {
	mgr := newTestManager(t)
	runner := &fakePassthroughRunner{}

	mgr.autoInjectInitialPromptWith(runner, newAutoInjectExecution(""), agents.PassthroughConfig{
		AutoInjectPrompt: true,
		SubmitSequence:   "\r",
	})

	if runner.writeCalled {
		t.Fatalf("expected no stdin write when task description is empty, got data=%q", runner.writtenData)
	}
}

func TestAutoInject_writes_description_plus_submit(t *testing.T) {
	mgr := newTestManager(t)
	runner := &fakePassthroughRunner{}

	mgr.autoInjectInitialPromptWith(runner, newAutoInjectExecution("hello world"), agents.PassthroughConfig{
		AutoInjectPrompt: true,
		SubmitSequence:   "\r",
	})

	if !runner.writeCalled {
		t.Fatalf("expected WriteStdin to be called")
	}
	if runner.writtenProcessID != "proc-abc" {
		t.Errorf("WriteStdin processID = %q, want %q", runner.writtenProcessID, "proc-abc")
	}
	if runner.writtenData != "hello world\r" {
		t.Errorf("WriteStdin data = %q, want %q", runner.writtenData, "hello world\r")
	}
}

// TestAutoInject_returns_when_wait_errors exercises the WaitForFirstIdle
// error-return path: the helper must skip the stdin write and exit cleanly
// rather than panic or loop.
func TestAutoInject_returns_when_wait_errors(t *testing.T) {
	mgr := newTestManager(t)
	runner := &fakePassthroughRunner{waitErr: context.DeadlineExceeded}

	done := make(chan struct{})
	go func() {
		defer close(done)
		mgr.autoInjectInitialPromptWith(runner, newAutoInjectExecution("hello"), agents.PassthroughConfig{
			AutoInjectPrompt: true,
			SubmitSequence:   "\r",
		})
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("autoInjectInitialPromptWith did not return within 2s")
	}

	if runner.writeCalled {
		t.Fatalf("expected no stdin write on wait timeout, got data=%q", runner.writtenData)
	}
}

// TestAutoInject_SubmitDelay_splits_writes_with_pause is the Claude regression:
// when SubmitDelay > 0 the helper must write the body chunk first, then sleep
// SubmitDelay, then write the submit byte alone, so Ink's paste-burst detector
// sees the trailing \r as a discrete keystroke instead of absorbing it into the
// pasted text.
func TestAutoInject_SubmitDelay_splits_writes_with_pause(t *testing.T) {
	mgr := newTestManager(t)
	runner := &fakePassthroughRunner{}

	const delay = 40 * time.Millisecond
	mgr.autoInjectInitialPromptWith(runner, newAutoInjectExecution("hello world"), agents.PassthroughConfig{
		AutoInjectPrompt:      true,
		SubmitSequence:        "\r",
		DisableBracketedPaste: true,
		SubmitDelay:           delay,
	})

	if got := len(runner.writes); got != 2 {
		t.Fatalf("expected 2 writes (body, submit), got %d: %#v", got, runner.writes)
	}
	if runner.writes[0] != "hello world" {
		t.Errorf("body write = %q, want %q", runner.writes[0], "hello world")
	}
	if runner.writes[1] != "\r" {
		t.Errorf("submit write = %q, want %q", runner.writes[1], "\r")
	}
	// Allow generous slack — CI clocks are noisy but a 40ms gap is still easy
	// to detect. The submit write must arrive at least ~SubmitDelay after the
	// body write; if the two were back-to-back this delta would be sub-ms.
	gap := runner.writeTimes[1].Sub(runner.writeTimes[0])
	if gap < delay/2 {
		t.Errorf("submit write came too soon after body: gap=%v, want >= %v", gap, delay/2)
	}
}

func TestAutoInject_skipped_when_process_id_missing(t *testing.T) {
	mgr := newTestManager(t)
	runner := &fakePassthroughRunner{}

	exec := newAutoInjectExecution("hello")
	exec.PassthroughProcessID = ""

	mgr.autoInjectInitialPromptWith(runner, exec, agents.PassthroughConfig{
		AutoInjectPrompt: true,
		SubmitSequence:   "\r",
	})

	if runner.writeCalled {
		t.Fatalf("expected no stdin write when PassthroughProcessID empty, got data=%q", runner.writtenData)
	}
}
