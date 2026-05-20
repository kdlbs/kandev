package process

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func TestInteractiveRunner_Start(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	cmd, env := fixtureExec("echo hello")
	req := InteractiveStartRequest{
		SessionID:      "test-session",
		Command:        cmd,
		Env:            env,
		ImmediateStart: true,
		DefaultCols:    80,
		DefaultRows:    24,
	}

	info, err := runner.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if info.ID == "" {
		t.Error("Start() returned empty ID")
	}
	if info.SessionID != "test-session" {
		t.Errorf("Start() SessionID = %q, want %q", info.SessionID, "test-session")
	}
	if info.Status != types.ProcessStatusRunning {
		t.Errorf("Start() Status = %v, want %v", info.Status, types.ProcessStatusRunning)
	}

	// Poll for exit. A fixed sleep was flaky on slow CI runners — the test
	// binary spawned via PTY can take longer than half a second to print
	// `echo hello` and exit on Windows GitHub Actions hosts.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		procInfo, ok := runner.Get(info.ID, false)
		if !ok || procInfo.Status != types.ProcessStatusRunning {
			return // exited and (optionally) cleaned up
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Error("Process should have exited within 5s")
}

func TestInteractiveRunner_Start_ValidationErrors(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	validCmd, validEnv := fixtureExec("echo")
	tests := []struct {
		name    string
		req     InteractiveStartRequest
		wantErr bool
	}{
		{
			name: "missing session_id",
			req: InteractiveStartRequest{
				Command: validCmd,
				Env:     validEnv,
			},
			wantErr: true,
		},
		{
			name: "missing command",
			req: InteractiveStartRequest{
				SessionID: "test",
			},
			wantErr: true,
		},
		{
			name: "valid request",
			req: InteractiveStartRequest{
				SessionID:      "test",
				Command:        validCmd,
				Env:            validEnv,
				ImmediateStart: true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runner.Start(context.Background(), tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("Start() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestInteractiveRunner_DeferredStart(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	// Start without ImmediateStart - process should be deferred.
	// `cat` blocks waiting for input, giving us time to check status.
	cmd, env := fixtureExec("cat")
	req := InteractiveStartRequest{
		SessionID: "deferred-session",
		Command:   cmd,
		Env:       env,
	}

	info, err := runner.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Try to write - should fail because process not started
	err = runner.WriteStdin(info.ID, "test")
	if err == nil {
		t.Error("WriteStdin() should fail for deferred process")
	}

	// Trigger start via resize
	err = runner.ResizeBySession("deferred-session", 80, 24)
	if err != nil {
		t.Fatalf("ResizeBySession() error = %v", err)
	}

	// Give process time to start
	time.Sleep(100 * time.Millisecond)

	// Now get process info - process should exist and be running
	procInfo, ok := runner.GetBySession("deferred-session")
	if !ok {
		t.Fatal("GetBySession() should find process after resize")
	}
	if procInfo.Status != types.ProcessStatusRunning {
		t.Errorf("Process status = %v, want running", procInfo.Status)
	}

	// Clean up - stop the process
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = runner.Stop(ctx, info.ID)
}

func TestInteractiveRunner_WriteStdin(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	// Start cat process that echoes input
	cmd, env := fixtureExec("cat")
	req := InteractiveStartRequest{
		SessionID:      "stdin-test",
		Command:        cmd,
		Env:            env,
		ImmediateStart: true,
		DefaultCols:    80,
		DefaultRows:    24,
	}

	info, err := runner.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Give process time to start
	time.Sleep(100 * time.Millisecond)

	// Write to stdin
	err = runner.WriteStdin(info.ID, "hello\n")
	if err != nil {
		t.Errorf("WriteStdin() error = %v", err)
	}

	// Stop the process
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = runner.Stop(ctx, info.ID)
}

func TestInteractiveRunner_Stop(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	// Start a long-running process
	cmd, env := fixtureExec("sleep 60")
	req := InteractiveStartRequest{
		SessionID:      "stop-test",
		Command:        cmd,
		Env:            env,
		ImmediateStart: true,
		DefaultCols:    80,
		DefaultRows:    24,
	}

	info, err := runner.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Give process time to start
	time.Sleep(100 * time.Millisecond)

	// Stop the process
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = runner.Stop(ctx, info.ID)
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	// Process should be removed after stop
	time.Sleep(200 * time.Millisecond)
	_, ok := runner.Get(info.ID, false)
	if ok {
		t.Error("Process should be removed after stop")
	}
}
