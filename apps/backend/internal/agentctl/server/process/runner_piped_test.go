package process

import (
	"bufio"
	"context"
	"io"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func TestProcessRunnerStartPipedOutputRemainsReadableAfterDone(t *testing.T) {
	runner := NewProcessRunner(nil, newTestLogger(t), 2*1024*1024)
	command, env := fixtureExec("write-both stdout-after-done stderr-after-done")
	proc, err := runner.StartPiped(PipedStartRequest{
		SessionID:  "session-fast-exit",
		Kind:       types.ProcessKindCustom,
		ScriptName: "test-fast-exit",
		Command:    command[0],
		Args:       command[1:],
		Env:        env,
		PipeStderr: true,
	})
	if err != nil {
		t.Fatalf("StartPiped() error = %v", err)
	}
	t.Cleanup(func() {
		_ = proc.Stdin.Close()
		_ = proc.Stdout.Close()
		_ = proc.Stderr.Close()
	})
	if err := proc.Stdin.Close(); err != nil {
		t.Fatalf("close stdin: %v", err)
	}

	select {
	case <-proc.Done:
	case <-time.After(5 * time.Second):
		t.Fatal("fast-exit process was not reaped before timeout")
	}
	stdout, err := io.ReadAll(proc.Stdout)
	if err != nil {
		t.Fatalf("read stdout after Done: %v", err)
	}
	stderr, err := io.ReadAll(proc.Stderr)
	if err != nil {
		t.Fatalf("read stderr after Done: %v", err)
	}
	if string(stdout) != "stdout-after-done" {
		t.Fatalf("stdout = %q, want %q", stdout, "stdout-after-done")
	}
	if string(stderr) != "stderr-after-done" {
		t.Fatalf("stderr = %q, want %q", stderr, "stderr-after-done")
	}
}

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
