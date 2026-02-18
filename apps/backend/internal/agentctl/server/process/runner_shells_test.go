package process

import (
	"context"
	"strings"
	"testing"
	"time"
)

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
