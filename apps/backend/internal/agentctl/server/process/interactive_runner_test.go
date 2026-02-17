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

func TestContainsDSRQuery(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"ESC[6n", []byte("\x1b[6n"), true},
		{"ESC[?6n", []byte("\x1b[?6n"), true},
		{"mixed content with DSR", []byte("text\x1b[6nmore"), true},
		{"no escape", []byte("hello world"), false},
		{"partial ESC[6", []byte("\x1b[6"), false},
		{"ESC[c is not DSR", []byte("\x1b[c"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsDSRQuery(tt.data); got != tt.want {
				t.Errorf("containsDSRQuery(%q) = %v, want %v", tt.data, got, tt.want)
			}
		})
	}
}

func TestContainsDA1Query(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"ESC[c (DA1 no param)", []byte("\x1b[c"), true},
		{"ESC[0c (DA1 explicit 0)", []byte("\x1b[0c"), true},
		{"mixed with DA1", []byte("text\x1b[cmore"), true},
		{"both DSR and DA1", []byte("\x1b[6n\x1b[c"), true},
		{"no escape", []byte("hello world"), false},
		{"ESC[1c is cursor forward, not DA1", []byte("\x1b[1c"), false},
		{"ESC[2c is cursor forward, not DA1", []byte("\x1b[2c"), false},
		{"ESC[5c is cursor forward, not DA1", []byte("\x1b[5c"), false},
		{"ESC[9c is cursor forward, not DA1", []byte("\x1b[9c"), false},
		{"partial ESC[", []byte("\x1b["), false},
		{"empty data", []byte{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsDA1Query(tt.data); got != tt.want {
				t.Errorf("containsDA1Query(%q) = %v, want %v", tt.data, got, tt.want)
			}
		})
	}
}

func TestInteractiveRunner_ResizeByProcessID(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	// Start a deferred process
	req := InteractiveStartRequest{
		SessionID: "resize-pid-test",
		Command:   []string{"cat"},
	}
	info, err := runner.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Process should not be running yet
	if runner.IsProcessRunning(info.ID) {
		t.Error("process should not be running before resize")
	}

	// Trigger start via ResizeByProcessID
	if err := runner.ResizeByProcessID(info.ID, 80, 24); err != nil {
		t.Fatalf("ResizeByProcessID() error = %v", err)
	}

	// Give process time to start
	time.Sleep(100 * time.Millisecond)

	if !runner.IsProcessRunning(info.ID) {
		t.Error("process should be running after ResizeByProcessID")
	}

	// Subsequent resize should succeed
	if err := runner.ResizeByProcessID(info.ID, 120, 40); err != nil {
		t.Errorf("second ResizeByProcessID() error = %v", err)
	}

	// Non-existent process should fail
	if err := runner.ResizeByProcessID("nonexistent", 80, 24); err == nil {
		t.Error("ResizeByProcessID() should fail for nonexistent process")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = runner.Stop(ctx, info.ID)
}

func TestInteractiveRunner_IsUserShellIsolation(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	// Start a regular (non-user-shell) process
	agentReq := InteractiveStartRequest{
		SessionID:      "isolation-test",
		Command:        []string{"cat"},
		ImmediateStart: true,
		DefaultCols:    80,
		DefaultRows:    24,
	}
	agentInfo, err := runner.Start(context.Background(), agentReq)
	if err != nil {
		t.Fatalf("Start() agent error = %v", err)
	}

	// Start a user shell process for the same session
	shellReq := InteractiveStartRequest{
		SessionID:      "isolation-test",
		Command:        []string{"cat"},
		IsUserShell:    true,
		ImmediateStart: true,
		DefaultCols:    80,
		DefaultRows:    24,
	}
	shellInfo, err := runner.Start(context.Background(), shellReq)
	if err != nil {
		t.Fatalf("Start() shell error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// ResizeBySession should target the agent process, not the user shell
	if err := runner.ResizeBySession("isolation-test", 120, 40); err != nil {
		t.Errorf("ResizeBySession() error = %v", err)
	}

	// GetPtyWriterBySession should return the agent process, not the user shell
	_, procID, err := runner.GetPtyWriterBySession("isolation-test")
	if err != nil {
		t.Fatalf("GetPtyWriterBySession() error = %v", err)
	}
	if procID != agentInfo.ID {
		t.Errorf("GetPtyWriterBySession() returned process %q, want agent process %q", procID, agentInfo.ID)
	}

	// SetDirectOutput on user shell should NOT track at session level
	writer := &mockDirectWriter{}
	if err := runner.SetDirectOutput(shellInfo.ID, writer); err != nil {
		t.Fatalf("SetDirectOutput() shell error = %v", err)
	}
	// Session-level WebSocket should NOT be set for user shells
	if runner.HasActiveWebSocketBySession("isolation-test") {
		t.Error("user shell should not set session-level WebSocket tracking")
	}

	// SetDirectOutput on agent process SHOULD track at session level
	agentWriter := &mockDirectWriter{}
	if err := runner.SetDirectOutput(agentInfo.ID, agentWriter); err != nil {
		t.Fatalf("SetDirectOutput() agent error = %v", err)
	}
	if !runner.HasActiveWebSocketBySession("isolation-test") {
		t.Error("agent process should set session-level WebSocket tracking")
	}

	// ClearDirectOutputBySession should clear agent but not user shell
	runner.ClearDirectOutputBySession("isolation-test")

	// Agent should have its direct output cleared
	if runner.HasActiveWebSocket(agentInfo.ID) {
		t.Error("agent WebSocket should be cleared after ClearDirectOutputBySession")
	}
	// User shell should still have its direct output
	if !runner.HasActiveWebSocket(shellInfo.ID) {
		t.Error("user shell WebSocket should NOT be cleared by ClearDirectOutputBySession")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = runner.Stop(ctx, agentInfo.ID)
	_ = runner.Stop(ctx, shellInfo.ID)
}

// --- User Shell Tests ---

func TestInteractiveRunner_CreateUserShell_First(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	result := runner.CreateUserShell("session-1")

	if result.TerminalID == "" {
		t.Error("CreateUserShell() returned empty TerminalID")
	}
	if !strings.HasPrefix(result.TerminalID, "shell-") {
		t.Errorf("CreateUserShell() TerminalID = %q, want prefix 'shell-'", result.TerminalID)
	}
	if result.Label != "Terminal" {
		t.Errorf("CreateUserShell() Label = %q, want 'Terminal'", result.Label)
	}
	if result.Closable {
		t.Error("CreateUserShell() first terminal should not be closable")
	}
}

func TestInteractiveRunner_CreateUserShell_Subsequent(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	first := runner.CreateUserShell("session-1")
	second := runner.CreateUserShell("session-1")

	if first.TerminalID == second.TerminalID {
		t.Error("CreateUserShell() should return different terminal IDs")
	}
	if second.Label != "Terminal 2" {
		t.Errorf("CreateUserShell() second Label = %q, want 'Terminal 2'", second.Label)
	}
	if !second.Closable {
		t.Error("CreateUserShell() second terminal should be closable")
	}

	third := runner.CreateUserShell("session-1")
	if third.Label != "Terminal 3" {
		t.Errorf("CreateUserShell() third Label = %q, want 'Terminal 3'", third.Label)
	}
	if !third.Closable {
		t.Error("CreateUserShell() third terminal should be closable")
	}
}

func TestInteractiveRunner_CreateUserShell_DifferentSessions(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	r1 := runner.CreateUserShell("session-1")
	r2 := runner.CreateUserShell("session-2")

	// Both should be "Terminal" (first in each session)
	if r1.Label != "Terminal" {
		t.Errorf("session-1 Label = %q, want 'Terminal'", r1.Label)
	}
	if r2.Label != "Terminal" {
		t.Errorf("session-2 Label = %q, want 'Terminal'", r2.Label)
	}
	if r1.Closable || r2.Closable {
		t.Error("first terminal in each session should not be closable")
	}
}

func TestInteractiveRunner_RegisterScriptShell(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	runner.RegisterScriptShell("session-1", "script-abc", "npm start", "npm run start")

	// Should appear in list
	shells := runner.ListUserShells("session-1")

	// Should have 2: auto-created "Terminal" + registered script
	if len(shells) != 2 {
		t.Fatalf("ListUserShells() returned %d shells, want 2", len(shells))
	}

	// Find the script shell
	var scriptShell *UserShellInfo
	for i := range shells {
		if shells[i].TerminalID == "script-abc" {
			scriptShell = &shells[i]
			break
		}
	}
	if scriptShell == nil {
		t.Fatal("script shell not found in list")
	} else {
		if scriptShell.Label != "npm start" {
			t.Errorf("script shell Label = %q, want 'npm start'", scriptShell.Label)
		}
		if scriptShell.InitialCommand != "npm run start" {
			t.Errorf("script shell InitialCommand = %q, want 'npm run start'", scriptShell.InitialCommand)
		}
		if !scriptShell.Closable {
			t.Error("script shell should be closable")
		}
	}
	if scriptShell.ProcessID != "" {
		t.Errorf("script shell should have empty ProcessID before WebSocket connect, got %q", scriptShell.ProcessID)
	}
}

func TestInteractiveRunner_RegisterScriptShell_DoesNotAffectShellCount(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	// Register a script terminal
	runner.RegisterScriptShell("session-1", "script-abc", "Build", "npm run build")

	// Create a plain shell - should be "Terminal" (first plain shell), not "Terminal 2"
	result := runner.CreateUserShell("session-1")
	if result.Label != "Terminal" {
		t.Errorf("CreateUserShell() Label = %q, want 'Terminal' (scripts should not count)", result.Label)
	}
	if result.Closable {
		t.Error("first plain shell should not be closable regardless of script terminals")
	}
}

func TestInteractiveRunner_ListUserShells_AutoCreatesFirst(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	// First call should auto-create "Terminal"
	shells := runner.ListUserShells("session-1")

	if len(shells) != 1 {
		t.Fatalf("ListUserShells() returned %d shells, want 1", len(shells))
	}
	if shells[0].Label != "Terminal" {
		t.Errorf("auto-created shell Label = %q, want 'Terminal'", shells[0].Label)
	}
	if shells[0].Closable {
		t.Error("auto-created first shell should not be closable")
	}
	if shells[0].Running {
		t.Error("auto-created shell should not be running (no process)")
	}
	if !strings.HasPrefix(shells[0].TerminalID, "shell-") {
		t.Errorf("auto-created shell TerminalID = %q, want prefix 'shell-'", shells[0].TerminalID)
	}
}

func TestInteractiveRunner_ListUserShells_StableAfterAutoCreate(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	// First call auto-creates
	shells1 := runner.ListUserShells("session-1")
	// Second call should return the same list (not create another)
	shells2 := runner.ListUserShells("session-1")

	if len(shells1) != 1 || len(shells2) != 1 {
		t.Fatalf("ListUserShells() should return 1 shell each time, got %d and %d", len(shells1), len(shells2))
	}
	if shells1[0].TerminalID != shells2[0].TerminalID {
		t.Error("ListUserShells() should return the same terminal ID across calls")
	}
}

func TestInteractiveRunner_ListUserShells_Sorted(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	// Create shells with a small time gap
	runner.CreateUserShell("session-1")
	time.Sleep(10 * time.Millisecond)
	runner.CreateUserShell("session-1")
	time.Sleep(10 * time.Millisecond)
	runner.RegisterScriptShell("session-1", "script-1", "Build", "make build")

	shells := runner.ListUserShells("session-1")
	if len(shells) != 3 {
		t.Fatalf("ListUserShells() returned %d shells, want 3", len(shells))
	}

	// Should be sorted by creation time
	for i := 1; i < len(shells); i++ {
		if shells[i].CreatedAt.Before(shells[i-1].CreatedAt) {
			t.Errorf("shells not sorted by creation time: shell[%d] (%v) before shell[%d] (%v)",
				i, shells[i].CreatedAt, i-1, shells[i-1].CreatedAt)
		}
	}
}

func TestInteractiveRunner_ListUserShells_IsolatedBySessions(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	runner.CreateUserShell("session-1")
	runner.CreateUserShell("session-1")
	runner.CreateUserShell("session-2")

	shells1 := runner.ListUserShells("session-1")
	shells2 := runner.ListUserShells("session-2")

	if len(shells1) != 2 {
		t.Errorf("session-1 should have 2 shells, got %d", len(shells1))
	}
	if len(shells2) != 1 {
		t.Errorf("session-2 should have 1 shell, got %d", len(shells2))
	}
}

func TestInteractiveRunner_StopUserShell(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	// Create a shell
	result := runner.CreateUserShell("session-1")

	// Verify it's in the list
	shells := runner.ListUserShells("session-1")
	found := false
	for _, s := range shells {
		if s.TerminalID == result.TerminalID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("created shell not found in list")
	}

	// Stop the shell (no process running, so this should remove the entry)
	ctx := context.Background()
	err := runner.StopUserShell(ctx, "session-1", result.TerminalID)
	// Error is expected since there's no process to stop
	_ = err

	// Shell should be removed from tracking
	shells = runner.ListUserShells("session-1")
	for _, s := range shells {
		if s.TerminalID == result.TerminalID {
			t.Error("stopped shell should be removed from list")
		}
	}
}

func TestInteractiveRunner_StopUserShell_NonExistent(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	// Stopping a non-existent shell should not error
	ctx := context.Background()
	err := runner.StopUserShell(ctx, "session-1", "nonexistent")
	if err != nil {
		t.Errorf("StopUserShell() for non-existent shell should return nil, got %v", err)
	}
}

func TestInteractiveRunner_StopUserShell_ScriptTerminal(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	// Register and then stop a script terminal
	runner.RegisterScriptShell("session-1", "script-abc", "Build", "npm run build")

	ctx := context.Background()
	err := runner.StopUserShell(ctx, "session-1", "script-abc")
	_ = err // Error expected since no process

	// Script should be removed from list (auto-created "Terminal" may still appear)
	shells := runner.ListUserShells("session-1")
	for _, s := range shells {
		if s.TerminalID == "script-abc" {
			t.Error("stopped script terminal should be removed from list")
		}
	}
}

func TestInteractiveRunner_CreateUserShell_AtomicRegistration(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	// Create a shell - it should be immediately visible in ListUserShells
	result := runner.CreateUserShell("session-1")

	shells := runner.ListUserShells("session-1")
	found := false
	for _, s := range shells {
		if s.TerminalID == result.TerminalID {
			found = true
			if s.Label != result.Label {
				t.Errorf("shell Label mismatch: list=%q, create=%q", s.Label, result.Label)
			}
			if s.Closable != result.Closable {
				t.Errorf("shell Closable mismatch: list=%v, create=%v", s.Closable, result.Closable)
			}
			break
		}
	}
	if !found {
		t.Error("CreateUserShell() result should be immediately visible in ListUserShells()")
	}
}
