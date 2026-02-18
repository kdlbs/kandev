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

	req := InteractiveStartRequest{
		SessionID:      "test-session",
		Command:        []string{"echo", "hello"},
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

	// Wait for process to exit
	time.Sleep(500 * time.Millisecond)

	// Process should have completed
	procInfo, ok := runner.Get(info.ID, false)
	if !ok {
		// Process may have been removed after exit, which is expected
		return
	}
	if procInfo.Status == types.ProcessStatusRunning {
		t.Error("Process should have exited")
	}
}

func TestInteractiveRunner_Start_ValidationErrors(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	tests := []struct {
		name    string
		req     InteractiveStartRequest
		wantErr bool
	}{
		{
			name: "missing session_id",
			req: InteractiveStartRequest{
				Command: []string{"echo"},
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
				Command:        []string{"echo"},
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

	// Start without ImmediateStart - process should be deferred
	// Use 'cat' which blocks waiting for input, giving us time to check status
	req := InteractiveStartRequest{
		SessionID: "deferred-session",
		Command:   []string{"cat"},
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
	req := InteractiveStartRequest{
		SessionID:      "stdin-test",
		Command:        []string{"cat"},
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
	req := InteractiveStartRequest{
		SessionID:      "stop-test",
		Command:        []string{"sleep", "60"},
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
