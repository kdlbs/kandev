package worktree

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunCleanupScriptWithoutMessageUsesRequestEnvironment(t *testing.T) {
	t.Setenv("GOCACHE", "/home/user/.cache/go-build")
	workDir := t.TempDir()
	handler := NewDefaultScriptMessageHandler(newTestLogger(), nil, time.Minute)
	req := ScriptExecutionRequest{
		Script:     `printf '%s' "$GOCACHE" > gocache.txt`,
		WorkingDir: workDir,
		ScriptType: "cleanup",
		Env:        map[string]string{"GOCACHE": "/opt/kandev/cache/go-build"},
	}

	if err := handler.runScriptWithoutMessage(context.Background(), req, false); err != nil {
		t.Fatalf("runScriptWithoutMessage() error = %v", err)
	}
	got, err := os.ReadFile(filepath.Join(workDir, "gocache.txt"))
	if err != nil {
		t.Fatalf("read cleanup output: %v", err)
	}
	if string(got) != req.Env["GOCACHE"] {
		t.Fatalf("cleanup GOCACHE = %q, want %q", got, req.Env["GOCACHE"])
	}
}
