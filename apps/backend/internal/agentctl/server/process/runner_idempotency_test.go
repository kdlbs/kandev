package process

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func TestProcessRunnerStableRequestIdentityReusesAndRejectsConflicts(t *testing.T) {
	runner := NewProcessRunner(nil, newTestLogger(t), 2*1024*1024)
	command, env := fixtureShellExec("sleep 30")
	request := StartProcessRequest{
		RequestID: "workflow-run-1", SessionID: "session-1", Kind: types.ProcessKindCustom,
		Command: command, Env: env,
	}
	first, err := runner.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	defer func() { _ = runner.Stop(context.Background(), StopProcessRequest{ProcessID: first.ID}) }()

	second, err := runner.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("duplicate start: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("duplicate start returned process %q, want %q", second.ID, first.ID)
	}

	request.Command = strings.TrimSpace(command) + " conflicting"
	if _, err := runner.Start(context.Background(), request); err == nil || !strings.Contains(err.Error(), "request identity") {
		t.Fatalf("conflicting request error = %v, want request identity conflict", err)
	}
}

func TestProcessRunnerGetByRequestIDFindsAdmittedProcess(t *testing.T) {
	runner := NewProcessRunner(nil, newTestLogger(t), 2*1024*1024)
	command, env := fixtureShellExec("sleep 30")
	request := StartProcessRequest{
		RequestID: "workflow-run-recovery", SessionID: "session-recovery", Kind: types.ProcessKindCustom,
		Command: command, Env: env,
	}
	started, err := runner.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = runner.Stop(context.Background(), StopProcessRequest{ProcessID: started.ID}) }()

	got, ok := runner.GetByRequestID(request.RequestID, true)
	if !ok {
		t.Fatal("GetByRequestID did not find admitted process")
	}
	if got.ID != started.ID || got.SessionID != request.SessionID {
		t.Fatalf("lookup = %+v, started = %+v", got, started)
	}
}

func TestProcessRunnerTimeoutReturnsTypedTerminalResultAndRetainsOutput(t *testing.T) {
	runner := NewProcessRunner(nil, newTestLogger(t), 2*1024*1024)
	command, env := fixtureShellExec("echo-then-sleep timeout 30")
	info, err := runner.Start(context.Background(), StartProcessRequest{
		RequestID: "workflow-timeout-1", SessionID: "session-timeout", Kind: types.ProcessKindCustom,
		Command: command, Env: env, Timeout: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("start timeout process: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, ok := runner.Get(info.ID, true)
		if ok && got.Status == types.ProcessStatusTimedOut {
			if got.ExitCode == nil {
				t.Fatal("timed-out process has no exit code")
			}
			combined := ""
			for _, chunk := range got.Output {
				combined += chunk.Data
			}
			if !strings.Contains(combined, "timeout") {
				t.Fatalf("timed-out output = %q, want captured prefix", combined)
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("timed-out process did not produce a retained terminal result")
}
