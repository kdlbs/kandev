package lifecycle

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestRunSetupScript_StreamsOutputWhileRunning verifies that the streaming
// callback fires with partial output as the script runs, not only once at the
// end. This is what makes the UI show `npm install` progress live instead of
// sitting on a silent spinner.
func TestRunSetupScript_StreamsOutputWhileRunning(t *testing.T) {
	// Script emits three lines with 200ms gaps. With the 100ms stream interval,
	// we expect at least two intermediate flushes before completion.
	script := `echo one; sleep 0.2; echo two; sleep 0.2; echo three`

	var mu sync.Mutex
	var snapshots []string
	cb := func(current string) {
		mu.Lock()
		defer mu.Unlock()
		snapshots = append(snapshots, current)
	}

	output, err := runSetupScript(context.Background(), script, t.TempDir(), nil, cb)
	if err != nil {
		t.Fatalf("runSetupScript error: %v", err)
	}
	if output != "one\ntwo\nthree" {
		t.Fatalf("unexpected final output: %q", output)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(snapshots) < 2 {
		t.Fatalf("expected at least 2 streaming snapshots, got %d: %v", len(snapshots), snapshots)
	}
	// Snapshots are monotonically growing — each should be a prefix of the next.
	for i := 1; i < len(snapshots); i++ {
		if !strings.HasPrefix(snapshots[i], strings.TrimSpace(snapshots[i-1])) {
			t.Fatalf("snapshot %d (%q) is not a growth of snapshot %d (%q)", i, snapshots[i], i-1, snapshots[i-1])
		}
	}
	// First snapshot should contain "one" (seen at ~0ms after first echo).
	if !strings.Contains(snapshots[0], "one") {
		t.Fatalf("first snapshot should contain 'one', got %q", snapshots[0])
	}
}

// TestRunSetupScript_CapturesOutputOnFailure verifies that when the script
// exits non-zero, both the output and the error are returned so the UI can
// display what the failing command printed.
func TestRunSetupScript_CapturesOutputOnFailure(t *testing.T) {
	script := `echo installing; echo "ENOENT: package-lock.json" 1>&2; exit 1`

	output, err := runSetupScript(context.Background(), script, t.TempDir(), nil, nil)
	if err == nil {
		t.Fatal("expected error for non-zero exit, got nil")
	}
	if !strings.Contains(output, "installing") {
		t.Fatalf("output missing stdout line: %q", output)
	}
	if !strings.Contains(output, "ENOENT") {
		t.Fatalf("output missing stderr line: %q", output)
	}
}

// TestRunSetupScript_NilCallbackIsSafe verifies callers can skip streaming.
func TestRunSetupScript_NilCallbackIsSafe(t *testing.T) {
	output, err := runSetupScript(context.Background(), `echo hi`, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("runSetupScript error: %v", err)
	}
	if output != "hi" {
		t.Fatalf("unexpected output: %q", output)
	}
}

// TestLocalPreparer_StreamsSetupScriptProgress verifies the LocalPreparer wires
// the streaming callback into PrepareProgressCallback — the setup script step
// is reported multiple times with growing Output before completion.
func TestLocalPreparer_StreamsSetupScriptProgress(t *testing.T) {
	isolateGitEnv(t)
	log := newTestLocalLogger()
	preparer := NewLocalPreparer(log)
	repoDir := initGitRepo(t)

	req := &EnvPrepareRequest{
		TaskID:         "task-1",
		RepositoryPath: repoDir,
		SetupScript:    `echo first; sleep 0.2; echo second; sleep 0.2; echo third`,
	}

	var mu sync.Mutex
	var setupEvents []PrepareStep
	cb := func(step PrepareStep, _, _ int) {
		if step.Name != "Run setup script" {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		setupEvents = append(setupEvents, step)
	}

	result, err := preparer.Prepare(context.Background(), req, cb)
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Prepare() failed: %s", result.ErrorMessage)
	}

	mu.Lock()
	defer mu.Unlock()
	// Expect: 1 begin (running, no output) + >=1 streaming update (running, partial output) + 1 final (completed, full output).
	if len(setupEvents) < 3 {
		t.Fatalf("expected >=3 setup step events (begin + stream + complete), got %d", len(setupEvents))
	}

	first := setupEvents[0]
	last := setupEvents[len(setupEvents)-1]
	if first.Status != PrepareStepRunning {
		t.Errorf("first setup event should be running, got %q", first.Status)
	}
	if first.Output != "" {
		t.Errorf("first setup event should have no output yet, got %q", first.Output)
	}
	if last.Status != PrepareStepCompleted {
		t.Errorf("last setup event should be completed, got %q", last.Status)
	}
	if !strings.Contains(last.Output, "first") || !strings.Contains(last.Output, "third") {
		t.Errorf("final output should contain all lines, got %q", last.Output)
	}

	// At least one intermediate event should have partial (but non-empty) output.
	sawPartial := false
	for _, ev := range setupEvents[1 : len(setupEvents)-1] {
		if ev.Status == PrepareStepRunning && ev.Output != "" {
			sawPartial = true
			break
		}
	}
	if !sawPartial {
		t.Errorf("expected at least one intermediate streaming event with partial output; events: %+v", setupEvents)
	}
}

// TestLocalPreparer_SetupScriptFailurePreservesOutputAndError verifies that
// a failing setup script marks the step as failed and keeps both the output
// and error attached — this is what lets the UI display the failure banner
// with the tail of stdout/stderr.
func TestLocalPreparer_SetupScriptFailurePreservesOutputAndError(t *testing.T) {
	isolateGitEnv(t)
	log := newTestLocalLogger()
	preparer := NewLocalPreparer(log)
	repoDir := initGitRepo(t)

	req := &EnvPrepareRequest{
		TaskID:         "task-1",
		RepositoryPath: repoDir,
		SetupScript:    `echo running install; echo "fatal: not a git repository" 1>&2; exit 1`,
	}

	result, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("Prepare() error: %v (setup script failure should be non-fatal)", err)
	}
	// Per design, setup script failure is non-fatal — overall result succeeds
	// so the agent can still start and the user can retry.
	if !result.Success {
		t.Fatalf("Prepare() should succeed even when setup script fails: %s", result.ErrorMessage)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 steps (validate + setup), got %d", len(result.Steps))
	}
	setup := result.Steps[1]
	if setup.Name != "Run setup script" {
		t.Fatalf("second step should be setup script, got %q", setup.Name)
	}
	if setup.Status != PrepareStepFailed {
		t.Errorf("setup step should be failed, got %q", setup.Status)
	}
	if setup.Error == "" {
		t.Errorf("setup step should have an error message")
	}
	if !strings.Contains(setup.Output, "running install") {
		t.Errorf("setup step output should contain stdout, got %q", setup.Output)
	}
	if !strings.Contains(setup.Output, "fatal: not a git repository") {
		t.Errorf("setup step output should contain stderr, got %q", setup.Output)
	}
}

// TestStreamingWriter_ThrottlesToMinGap verifies the writer does not fire the
// callback more often than minGap — burst writes should coalesce.
func TestStreamingWriter_ThrottlesToMinGap(t *testing.T) {
	var mu sync.Mutex
	var calls int
	w := newStreamingWriter(func(_ string) {
		mu.Lock()
		calls++
		mu.Unlock()
	}, 50*time.Millisecond)

	// Burst 10 writes in a tight loop — throttled callback should fire once
	// (or at most twice if the test scheduler stalls across the 50ms window).
	for i := 0; i < 10; i++ {
		if _, err := w.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if calls > 2 {
		t.Errorf("expected <=2 throttled callback invocations for a tight burst, got %d", calls)
	}
}
