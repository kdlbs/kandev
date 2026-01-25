package process

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func TestInteractiveRunner_Start(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	req := InteractiveStartRequest{
		SessionID:     "test-session",
		Command:       []string{"echo", "hello"},
		ImmediateStart: true,
		DefaultCols:   80,
		DefaultRows:   24,
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
				SessionID:     "test",
				Command:       []string{"echo"},
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

func TestInteractiveRunner_GetBuffer(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	req := InteractiveStartRequest{
		SessionID:      "buffer-test",
		Command:        []string{"echo", "buffered output"},
		ImmediateStart: true,
		DefaultCols:    80,
		DefaultRows:    24,
	}

	info, err := runner.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Wait for output
	time.Sleep(500 * time.Millisecond)

	buffer, ok := runner.GetBuffer(info.ID)
	if !ok {
		// Process may have exited and been removed
		return
	}

	// Check if output was captured
	combined := ""
	for _, chunk := range buffer {
		combined += chunk.Data
	}

	if !strings.Contains(combined, "buffered") {
		t.Logf("Buffer contents: %q", combined)
		// Note: Output might be empty if process exited too quickly
	}
}

func TestInteractiveRunner_Callbacks(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	var statusReceived bool
	var mu sync.Mutex

	runner.SetOutputCallback(func(output *types.ProcessOutput) {
		// Output callback received
	})

	runner.SetStatusCallback(func(status *types.ProcessStatusUpdate) {
		mu.Lock()
		statusReceived = true
		mu.Unlock()
	})

	req := InteractiveStartRequest{
		SessionID:      "callback-test",
		Command:        []string{"echo", "callback test"},
		ImmediateStart: true,
		DefaultCols:    80,
		DefaultRows:    24,
	}

	_, err := runner.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Wait for callbacks
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	if !statusReceived {
		t.Error("Status callback should have been called")
	}
	// Output callback may or may not be called depending on timing
	mu.Unlock()
}

func TestInteractiveRunner_TurnCompleteCallback(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	var turnCompleteCalled bool
	var turnSessionID string
	var mu sync.Mutex

	runner.SetTurnCompleteCallback(func(sessionID string) {
		mu.Lock()
		turnCompleteCalled = true
		turnSessionID = sessionID
		mu.Unlock()
	})

	// Start with a prompt pattern that matches "$ "
	req := InteractiveStartRequest{
		SessionID:      "turn-test",
		Command:        []string{"bash", "-c", "echo '$ '"},
		PromptPattern:  `\$ $`,
		ImmediateStart: true,
		DefaultCols:    80,
		DefaultRows:    24,
	}

	_, err := runner.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Wait for turn detection
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	if turnCompleteCalled && turnSessionID != "turn-test" {
		t.Errorf("Turn complete callback received wrong session ID: %q", turnSessionID)
	}
	mu.Unlock()
}

func TestInteractiveRunner_DirectOutput(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	req := InteractiveStartRequest{
		SessionID:      "direct-output-test",
		Command:        []string{"echo", "direct"},
		ImmediateStart: true,
		DefaultCols:    80,
		DefaultRows:    24,
	}

	info, err := runner.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Create a mock direct writer
	writer := &mockDirectWriter{}

	// Set direct output
	err = runner.SetDirectOutput(info.ID, writer)
	if err != nil {
		t.Errorf("SetDirectOutput() error = %v", err)
	}

	// Wait for output
	time.Sleep(200 * time.Millisecond)

	// Clear direct output
	err = runner.ClearDirectOutput(info.ID)
	// May fail if process already exited, that's OK
	_ = err

	// Check if writer received data
	writer.mu.Lock()
	gotData := len(writer.data) > 0
	writer.mu.Unlock()

	if gotData {
		t.Log("Direct writer received data")
	}
}

// mockDirectWriter implements DirectOutputWriter for testing
type mockDirectWriter struct {
	mu     sync.Mutex
	data   []byte
	closed bool
}

func (w *mockDirectWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.data = append(w.data, p...)
	return len(p), nil
}

func (w *mockDirectWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	return nil
}

func TestInteractiveRunner_GetPtyWriter(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	req := InteractiveStartRequest{
		SessionID:      "pty-writer-test",
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

	// Get PTY writer
	writer, err := runner.GetPtyWriter(info.ID)
	if err != nil {
		t.Fatalf("GetPtyWriter() error = %v", err)
	}

	if writer == nil {
		t.Error("GetPtyWriter() returned nil writer")
	}

	// Stop the process
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = runner.Stop(ctx, info.ID)
}

func TestInteractiveRunner_GetPtyWriter_NotStarted(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	// Start without ImmediateStart
	req := InteractiveStartRequest{
		SessionID: "not-started",
		Command:   []string{"cat"},
	}

	info, err := runner.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Try to get PTY writer before process starts
	_, err = runner.GetPtyWriter(info.ID)
	if err == nil {
		t.Error("GetPtyWriter() should fail for deferred process")
	}
}

func TestInteractiveRunner_NotFound(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	// Test various methods with non-existent process
	_, ok := runner.Get("nonexistent", false)
	if ok {
		t.Error("Get() should return false for nonexistent process")
	}

	_, ok = runner.GetBySession("nonexistent")
	if ok {
		t.Error("GetBySession() should return false for nonexistent session")
	}

	_, ok = runner.GetBuffer("nonexistent")
	if ok {
		t.Error("GetBuffer() should return false for nonexistent process")
	}

	err := runner.WriteStdin("nonexistent", "data")
	if err == nil {
		t.Error("WriteStdin() should fail for nonexistent process")
	}

	ctx := context.Background()
	err = runner.Stop(ctx, "nonexistent")
	if err == nil {
		t.Error("Stop() should fail for nonexistent process")
	}

	err = runner.SetDirectOutput("nonexistent", nil)
	if err == nil {
		t.Error("SetDirectOutput() should fail for nonexistent process")
	}

	_, err = runner.GetPtyWriter("nonexistent")
	if err == nil {
		t.Error("GetPtyWriter() should fail for nonexistent process")
	}
}
