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
	mgr := newTestManager()
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
	mgr := newTestManager()
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
	mgr := newTestManager()
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
	mgr := newTestManager()
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

func TestAutoInject_times_out_gracefully(t *testing.T) {
	mgr := newTestManager()
	// Use a runner that blocks on WaitForFirstIdle, but shorten the timeout
	// to avoid making the test take 60s. We test the gracefulness via the
	// inner helper directly so the real 60s ctx never matters: we wrap with
	// a small context here.
	runner := &fakePassthroughRunner{waitBlock: true}

	// Drive the inner with a manually canceled context by simulating the
	// wait error path: call WaitForFirstIdle with a tight context to ensure
	// the runner returns ctx.Err() quickly. We invoke autoInjectInitialPromptWith
	// in a goroutine and assert no panic / no write within a reasonable window.
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Inject a wait error and unblock so we don't actually wait 60s.
		runner.mu.Lock()
		runner.waitBlock = false
		runner.waitErr = context.DeadlineExceeded
		runner.mu.Unlock()

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

func TestAutoInject_skipped_when_process_id_missing(t *testing.T) {
	mgr := newTestManager()
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
