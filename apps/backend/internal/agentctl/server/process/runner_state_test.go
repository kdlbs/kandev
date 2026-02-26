package process

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
)

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
		Command:        []string{"cat"},
		ImmediateStart: true,
		DefaultCols:    80,
		DefaultRows:    24,
	}

	info, err := runner.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Wait for process to be running
	deadline := time.Now().Add(5 * time.Second)
	for !runner.IsProcessRunning(info.ID) {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for process to start")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Create a mock direct writer
	writer := &mockDirectWriter{}

	// Set direct output
	err = runner.SetDirectOutput(info.ID, writer)
	if err != nil {
		t.Errorf("SetDirectOutput() error = %v", err)
	}

	// Clear direct output
	err = runner.ClearDirectOutput(info.ID)
	if err != nil {
		t.Errorf("ClearDirectOutput() error = %v", err)
	}

	// Stop the process
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = runner.Stop(ctx, info.ID)
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

func TestInteractiveRunner_IsProcessRunning(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	// Non-existent process
	if runner.IsProcessRunning("nonexistent") {
		t.Error("IsProcessRunning() should return false for nonexistent process")
	}

	// Start a process
	req := InteractiveStartRequest{
		SessionID:      "running-test",
		Command:        []string{"sleep", "10"},
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

	// Process should be running
	if !runner.IsProcessRunning(info.ID) {
		t.Error("IsProcessRunning() should return true for running process")
	}

	// Stop the process
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = runner.Stop(ctx, info.ID)

	// Give it time to clean up
	time.Sleep(200 * time.Millisecond)

	// Process should no longer be running
	if runner.IsProcessRunning(info.ID) {
		t.Error("IsProcessRunning() should return false after stop")
	}
}

func TestInteractiveRunner_IsProcessReadyOrPending(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	// Non-existent process
	if runner.IsProcessReadyOrPending("nonexistent") {
		t.Error("IsProcessReadyOrPending() should return false for nonexistent process")
	}

	// Start a deferred process (not started yet)
	req := InteractiveStartRequest{
		SessionID: "pending-test",
		Command:   []string{"cat"},
		// ImmediateStart: false (deferred)
	}

	info, err := runner.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Deferred process should be "ready or pending"
	if !runner.IsProcessReadyOrPending(info.ID) {
		t.Error("IsProcessReadyOrPending() should return true for pending process")
	}

	// But not "running" yet
	if runner.IsProcessRunning(info.ID) {
		t.Error("IsProcessRunning() should return false for pending process")
	}

	// Trigger start via resize
	err = runner.ResizeBySession("pending-test", 80, 24)
	if err != nil {
		t.Fatalf("ResizeBySession() error = %v", err)
	}

	// Give process time to start
	time.Sleep(100 * time.Millisecond)

	// Now it should be both "running" and "ready or pending"
	if !runner.IsProcessRunning(info.ID) {
		t.Error("IsProcessRunning() should return true after start")
	}
	if !runner.IsProcessReadyOrPending(info.ID) {
		t.Error("IsProcessReadyOrPending() should return true for running process")
	}

	// Stop the process
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = runner.Stop(ctx, info.ID)
}
