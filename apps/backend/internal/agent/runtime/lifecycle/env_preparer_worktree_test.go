package lifecycle

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/worktree"
)

func TestCopyFilesStep_SingleFile(t *testing.T) {
	wt := &worktree.Worktree{CopiedFiles: []string{".env"}}
	step := copyFilesStep(wt, "")
	if step.Name != "Copy 1 ignored file" {
		t.Errorf("name = %q, want %q", step.Name, "Copy 1 ignored file")
	}
	if step.Output != ".env" {
		t.Errorf("output = %q, want %q", step.Output, ".env")
	}
	if step.Status != PrepareStepCompleted {
		t.Errorf("status = %q, want completed", step.Status)
	}
	if step.Warning != "" {
		t.Errorf("unexpected warning: %q", step.Warning)
	}
}

func TestCopyFilesStep_MultipleFilesWithRepoLabel(t *testing.T) {
	wt := &worktree.Worktree{CopiedFiles: []string{".env", "config/local.yml", "data.json"}}
	step := copyFilesStep(wt, "frontend")
	if step.Name != "Copy 3 ignored files (frontend)" {
		t.Errorf("name = %q, want %q", step.Name, "Copy 3 ignored files (frontend)")
	}
	lines := strings.Split(step.Output, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 output lines, got %d: %q", len(lines), step.Output)
	}
}

func TestCopyFilesStep_WarningsAttached(t *testing.T) {
	wt := &worktree.Worktree{
		CopiedFiles:       []string{".env"},
		CopyFilesWarnings: []string{`no matches for pattern "*.local"`, `invalid pattern "["`},
	}
	step := copyFilesStep(wt, "")
	if step.Warning == "" {
		t.Fatal("expected warning to be set when CopyFilesWarnings non-empty")
	}
	if !strings.Contains(step.WarningDetail, "*.local") || !strings.Contains(step.WarningDetail, "invalid pattern") {
		t.Errorf("warningDetail = %q, expected both warnings joined", step.WarningDetail)
	}
}

func TestCopyFilesStep_WarningsOnlyNoCopies(t *testing.T) {
	wt := &worktree.Worktree{CopyFilesWarnings: []string{`no matches for pattern ".env"`}}
	step := copyFilesStep(wt, "")
	if step.Name != "Copy ignored files" {
		t.Errorf("name = %q, want %q for zero-file warning case", step.Name, "Copy ignored files")
	}
	if step.Output != "" {
		t.Errorf("unexpected output for zero copies: %q", step.Output)
	}
	if step.Warning == "" {
		t.Error("expected warning to be set")
	}
}
