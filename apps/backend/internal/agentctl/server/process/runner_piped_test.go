package process

import (
	"bufio"
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func TestProcessRunnerStartPipedRoundTripAndStop(t *testing.T) {
	runner := NewProcessRunner(nil, newTestLogger(t), 2*1024*1024)
	command, env := fixtureExec("cat")
	proc, err := runner.StartPiped(PipedStartRequest{
		SessionID:  "session-1",
		Kind:       types.ProcessKindCustom,
		ScriptName: "test-lsp",
		Command:    command[0],
		Args:       command[1:],
		Env:        env,
	})
	if err != nil {
		t.Fatalf("StartPiped() error = %v", err)
	}

	if _, err := proc.Stdin.Write([]byte("hello\n")); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	line, err := bufio.NewReader(proc.Stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if line != "hello\n" {
		t.Fatalf("stdout = %q, want %q", line, "hello\\n")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runner.Stop(ctx, StopProcessRequest{ProcessID: proc.ID}); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	select {
	case <-proc.Done:
	case <-ctx.Done():
		t.Fatal("piped process was not reaped before timeout")
	}
	if _, ok := runner.Get(proc.ID, false); ok {
		t.Fatal("piped process remains tracked after stop")
	}
}
