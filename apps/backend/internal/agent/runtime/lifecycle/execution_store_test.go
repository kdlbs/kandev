package lifecycle

import (
	"errors"
	"testing"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestExecutionStore_AddRejectsDuplicateSession is the regression test for the
// process-leak bug where two paths created executions for the same session
// concurrently and Add silently overwrote the bySession index, orphaning the
// first execution's agent subprocess.
func TestExecutionStore_AddRejectsDuplicateSession(t *testing.T) {
	store := NewExecutionStore()

	first := &AgentExecution{ID: "exec-1", SessionID: "session-1"}
	if err := store.Add(first); err != nil {
		t.Fatalf("first Add: unexpected error: %v", err)
	}

	second := &AgentExecution{ID: "exec-2", SessionID: "session-1"}
	err := store.Add(second)
	if !errors.Is(err, ErrExecutionAlreadyExistsForSession) {
		t.Fatalf("second Add: want ErrExecutionAlreadyExistsForSession, got %v", err)
	}

	got, ok := store.GetBySessionID("session-1")
	if !ok {
		t.Fatalf("GetBySessionID: not found")
	}
	if got.ID != "exec-1" {
		t.Errorf("bySession index: want exec-1, got %s (overwrite was supposed to be rejected)", got.ID)
	}
	// Second execution must not be in the executions map either — otherwise
	// it'd live as an unreachable orphan.
	if _, ok := store.Get("exec-2"); ok {
		t.Errorf("Get(exec-2): rejected execution must not be tracked")
	}
}

// TestExecutionStore_ActivePromptGenerationGating pins the predicate a mid-turn
// steer relies on: a generation is steerable only while it is dispatched and
// still in flight — never merely admitted (buffers mid-reset), completed, or
// superseded by a newer prompt.
func TestExecutionStore_ActivePromptGenerationGating(t *testing.T) {
	store := NewExecutionStore()
	exec := &AgentExecution{ID: "exec-1", SessionID: "session-1"}
	if err := store.Add(exec); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if got := store.ActivePromptGeneration("exec-1"); got != 0 {
		t.Fatalf("no prompt begun: want 0, got %d", got)
	}

	gen, err := store.BeginPrompt("exec-1")
	if err != nil {
		t.Fatalf("BeginPrompt: %v", err)
	}
	// Admitted but not dispatched yet — a steer must not reuse it (it could
	// overtake the predecessor and race its buffer reset).
	if got := store.ActivePromptGeneration("exec-1"); got != 0 {
		t.Fatalf("admitted-not-dispatched: want 0, got %d", got)
	}

	store.MarkPromptDispatched("exec-1", gen)
	if got := store.ActivePromptGeneration("exec-1"); got != gen {
		t.Fatalf("dispatched in-flight: want %d, got %d", gen, got)
	}

	// Completion of that generation makes it non-steerable (the turn is over).
	if err := store.WithLock("exec-1", func(e *AgentExecution) { e.promptCompletionGeneration = gen }); err != nil {
		t.Fatalf("WithLock: %v", err)
	}
	if got := store.ActivePromptGeneration("exec-1"); got != 0 {
		t.Fatalf("completed generation: want 0, got %d", got)
	}

	// A newer generation that begins (and is not yet dispatched) is likewise not
	// steerable, and MarkPromptDispatched for the stale generation is ignored.
	newGen, err := store.BeginPrompt("exec-1")
	if err != nil {
		t.Fatalf("BeginPrompt (2): %v", err)
	}
	store.MarkPromptDispatched("exec-1", gen) // stale — must be ignored
	if got := store.ActivePromptGeneration("exec-1"); got != 0 {
		t.Fatalf("superseded, new gen not dispatched: want 0, got %d", got)
	}
	store.MarkPromptDispatched("exec-1", newGen)
	if got := store.ActivePromptGeneration("exec-1"); got != newGen {
		t.Fatalf("new gen dispatched: want %d, got %d", newGen, got)
	}
}

func TestExecutionStore_AddSameExecutionTwiceIsIdempotent(t *testing.T) {
	store := NewExecutionStore()

	exec := &AgentExecution{ID: "exec-1", SessionID: "session-1"}
	if err := store.Add(exec); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	if err := store.Add(exec); err != nil {
		t.Errorf("re-adding the SAME execution must be a no-op, got %v", err)
	}
}

func TestExecutionStore_AddReplaceAfterRemove(t *testing.T) {
	store := NewExecutionStore()

	first := &AgentExecution{ID: "exec-1", SessionID: "session-1"}
	if err := store.Add(first); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	store.Remove("exec-1")

	second := &AgentExecution{ID: "exec-2", SessionID: "session-1"}
	if err := store.Add(second); err != nil {
		t.Errorf("Add after Remove must succeed, got %v", err)
	}
	got, _ := store.GetBySessionID("session-1")
	if got == nil || got.ID != "exec-2" {
		t.Errorf("after Remove+Add: want exec-2, got %v", got)
	}
}

func TestExecutionStore_RemoveClearsRuntimeEnvironment(t *testing.T) {
	store := NewExecutionStore()
	execution := &AgentExecution{ID: "exec-1"}
	execution.setRuntimeEnvironment(map[string]string{"BROKER": "runtime-only"})
	if err := store.Add(execution); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	store.Remove(execution.ID)
	if got := execution.RuntimeEnvironment(); got != nil {
		t.Fatalf("RuntimeEnvironment() after Remove = %#v, want nil", got)
	}
}

func TestExecutionStore_AddNoSessionIDAlwaysSucceeds(t *testing.T) {
	store := NewExecutionStore()

	if err := store.Add(&AgentExecution{ID: "exec-a"}); err != nil {
		t.Errorf("Add without SessionID: %v", err)
	}
	if err := store.Add(&AgentExecution{ID: "exec-b"}); err != nil {
		t.Errorf("Add without SessionID (second): %v", err)
	}
}

func TestExecutionStore_TaskHostHasDedicatedEnvironmentOwnership(t *testing.T) {
	store := NewExecutionStore()
	normal := &AgentExecution{
		ID: "exec-session", SessionID: "session-1", TaskEnvironmentID: "env-1",
	}
	host := &AgentExecution{
		ID: "exec-task-host", SessionID: "task-host-env-1", TaskEnvironmentID: "env-1", IsTaskHost: true,
	}
	if err := store.Add(normal); err != nil {
		t.Fatalf("add normal execution: %v", err)
	}
	if err := store.Add(host); err != nil {
		t.Fatalf("add task host: %v", err)
	}

	if got, ok := store.GetBySessionID(host.SessionID); ok || got != nil {
		t.Fatalf("task host leaked into session index: %#v, %v", got, ok)
	}
	if got, ok := store.GetByTaskEnvironmentID("env-1"); !ok || got.ID != normal.ID {
		t.Fatalf("normal environment lookup = %#v, %v; want %s", got, ok, normal.ID)
	}
	if got, ok := store.GetTaskHostByEnvironmentID("env-1"); !ok || got.ID != host.ID {
		t.Fatalf("task host lookup = %#v, %v; want %s", got, ok, host.ID)
	}

	store.Remove(normal.ID)
	if got, ok := store.GetTaskHostByEnvironmentID("env-1"); !ok || got.ID != host.ID {
		t.Fatalf("task host after sibling removal = %#v, %v; want %s", got, ok, host.ID)
	}
	store.Remove(host.ID)
	if got, ok := store.GetTaskHostByEnvironmentID("env-1"); ok || got != nil {
		t.Fatalf("task host index after removal = %#v, %v", got, ok)
	}
}

func TestExecutionStore_TaskHostRequiresUniqueEnvironment(t *testing.T) {
	store := NewExecutionStore()
	if err := store.Add(&AgentExecution{ID: "missing-env", IsTaskHost: true}); err == nil {
		t.Fatal("task host without environment: expected error")
	}
	first := &AgentExecution{ID: "host-1", TaskEnvironmentID: "env-1", IsTaskHost: true}
	if err := store.Add(first); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	err := store.Add(&AgentExecution{ID: "host-2", TaskEnvironmentID: "env-1", IsTaskHost: true})
	if !errors.Is(err, ErrExecutionAlreadyExistsForTaskHost) {
		t.Fatalf("second Add: want ErrExecutionAlreadyExistsForTaskHost, got %v", err)
	}
	if _, ok := store.Get("host-2"); ok {
		t.Fatal("rejected task host must not be tracked")
	}
}

func TestExecutionStore_BeginPromptAlwaysAdvancesGeneration(t *testing.T) {
	store := NewExecutionStore()
	exec := &AgentExecution{
		ID:        "exec-1",
		SessionID: "session-1",
		Status:    v1.AgentStatusRunning,
	}
	if err := store.Add(exec); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if _, err := store.BeginPrompt(exec.ID); err != nil {
		t.Fatalf("BeginPrompt: %v", err)
	}
	if !store.OwnsPromptGeneration(exec.SessionID, exec.ID, 1) {
		t.Fatal("first prompt must create generation 1")
	}
	if _, err := store.BeginPrompt(exec.ID); err != nil {
		t.Fatalf("BeginPrompt replacement: %v", err)
	}
	if !store.OwnsPromptGeneration(exec.SessionID, exec.ID, 2) {
		t.Fatal("replacement prompt must create generation 2 while already running")
	}
}

func TestExecutionStore_BeginPromptClearsActiveTopLevelTool(t *testing.T) {
	store := NewExecutionStore()
	exec := &AgentExecution{ID: "exec-1", SessionID: "session-1"}
	if err := store.Add(exec); err != nil {
		t.Fatalf("Add: %v", err)
	}
	exec.setActiveTool(activeTopLevelTool{ToolCallID: "tool-1", Name: "shell"})

	if _, err := store.BeginPrompt(exec.ID); err != nil {
		t.Fatalf("BeginPrompt: %v", err)
	}
	if got := exec.activeToolSnapshot(); got != nil {
		t.Fatalf("active tool after BeginPrompt = %#v, want nil", got)
	}
}

func TestExecutionStore_OwnsPromptActivityRejectsChangedEpoch(t *testing.T) {
	store := NewExecutionStore()
	exec := &AgentExecution{
		ID:               "exec-1",
		SessionID:        "session-1",
		promptGeneration: 4,
	}
	if err := store.Add(exec); err != nil {
		t.Fatalf("Add: %v", err)
	}
	exec.armPromptActivity()

	if !store.OwnsPromptActivity(exec.SessionID, exec.ID, 4, 1) {
		t.Fatal("current activity epoch should own the prompt")
	}
	exec.markAgentActivity()
	if store.OwnsPromptActivity(exec.SessionID, exec.ID, 4, 1) {
		t.Fatal("changed activity epoch should invalidate the snapshot")
	}
}

func TestExecutionStore_ClaimPromptActivityRequiresCurrentIdentity(t *testing.T) {
	store := NewExecutionStore()
	exec := &AgentExecution{ID: "exec-1", SessionID: "session-1", promptGeneration: 4}
	if err := store.Add(exec); err != nil {
		t.Fatalf("Add: %v", err)
	}
	exec.armPromptActivity()

	claimed, err := store.ClaimPromptActivity(exec.SessionID, exec.ID, 4, 1)
	if err != nil {
		t.Fatalf("ClaimPromptActivity: %v", err)
	}
	if claimed != exec {
		t.Fatal("ClaimPromptActivity returned a different execution pointer")
	}

	exec.markAgentActivity()
	if _, err := store.ClaimPromptActivity(exec.SessionID, exec.ID, 4, 1); !errors.Is(err, ErrPromptActivityNotOwned) {
		t.Fatalf("ClaimPromptActivity after activity change = %v, want ErrPromptActivityNotOwned", err)
	}

	store.Remove(exec.ID)
	if _, err := store.ClaimPromptActivity(exec.SessionID, exec.ID, 4, 2); !errors.Is(err, ErrExecutionNotFound) {
		t.Fatalf("ClaimPromptActivity after removal = %v, want ErrExecutionNotFound", err)
	}
}
