package lifecycle

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	commonlogger "github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// @covers AC-PLATFORM-DIAGNOSTIC-LOGGING-001.12
func TestResolvePreparerSetupScriptDiagnosticSeverity(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := commonlogger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observer logger: %v", err)
	}
	previousLogger := prepareScriptLogger
	prepareScriptLogger = log
	t.Cleanup(func() { prepareScriptLogger = previousLogger })

	cases := []struct {
		name                string
		request             EnvPrepareRequest
		wantExplicit        bool
		wantEmptyDiagnostic bool
		secret              string
	}{
		{
			name: "default comment-only script",
			request: EnvPrepareRequest{
				TaskID:         "task-default",
				ExecutorType:   executor.NameStandalone,
				RepositoryPath: "/tmp/my-repo",
			},
			wantEmptyDiagnostic: true,
		},
		{
			name: "explicit comment-only script",
			request: EnvPrepareRequest{
				TaskID:         "task-explicit",
				ExecutorType:   executor.NameStandalone,
				RepositoryPath: "/tmp/my-repo",
				SetupScript:    "# secret-comment\n# another comment",
			},
			wantExplicit:        true,
			wantEmptyDiagnostic: true,
			secret:              "secret-comment",
		},
		{
			name: "blank and shebang-only script",
			request: EnvPrepareRequest{
				TaskID:         "task-shebang",
				ExecutorType:   executor.NameStandalone,
				RepositoryPath: "/tmp/my-repo",
				SetupScript:    "\n#!/bin/sh\n\n",
			},
			wantExplicit:        true,
			wantEmptyDiagnostic: true,
		},
		{
			name: "executable script",
			request: EnvPrepareRequest{
				TaskID:         "task-command",
				ExecutorType:   executor.NameStandalone,
				RepositoryPath: "/tmp/my-repo",
				SetupScript:    "echo setup-command",
			},
			wantExplicit: true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			before := logs.Len()
			got, err := resolvePreparerSetupScript(&tt.request, "/tmp/my-repo")
			if err != nil {
				t.Fatalf("resolvePreparerSetupScript() error = %v", err)
			}
			if tt.wantEmptyDiagnostic && got != "" {
				t.Fatalf("resolved script = %q, want empty", got)
			}
			if !tt.wantEmptyDiagnostic && got == "" {
				t.Fatal("resolved executable script is empty")
			}

			entries := logs.All()[before:]
			assertSetupScriptDiagnostic(t, entries, tt.request, tt.wantExplicit, tt.wantEmptyDiagnostic, tt.secret)
			if !tt.wantEmptyDiagnostic && len(entries) != 0 {
				t.Fatalf("executable script emitted omission diagnostics: %v", entries)
			}
		})
	}

	for _, entry := range logs.All() {
		if entry.Level == zap.WarnLevel {
			t.Fatalf("comment-only setup emitted warning: %#v", entry)
		}
	}
}

func assertSetupScriptDiagnostic(
	t *testing.T, entries []observer.LoggedEntry, req EnvPrepareRequest, wantExplicit, wantEmpty bool, secret string,
) {
	t.Helper()
	if !wantEmpty {
		return
	}
	if len(entries) != 1 {
		t.Fatalf("diagnostic entries = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Message != "setup script is comment-only after resolution, skipping" {
		t.Fatalf("diagnostic message = %q", entry.Message)
	}
	if entry.Level != zap.DebugLevel {
		t.Fatalf("diagnostic level = %s, want debug", entry.Level)
	}
	fields := entry.ContextMap()
	if fields["task_id"] != req.TaskID || fields["executor_type"] != string(req.ExecutorType) ||
		fields["use_worktree"] != req.UseWorktree || fields["has_explicit_script"] != wantExplicit {
		t.Fatalf("diagnostic fields = %v", fields)
	}
	if secret != "" && (strings.Contains(entry.Message, secret) || strings.Contains(fmt.Sprint(fields), secret)) {
		t.Fatalf("diagnostic exposed script content: %#v", entry)
	}
}

func TestResolvePreparerSetupScript_LocalFallbackCommentOnly(t *testing.T) {
	req := &EnvPrepareRequest{
		ExecutorType:   executor.NameStandalone,
		RepositoryPath: "/tmp/my-repo",
	}

	got, err := resolvePreparerSetupScript(req, "/tmp/my-repo")
	if err != nil {
		t.Fatalf("resolvePreparerSetupScript() error = %v", err)
	}
	if got != "" {
		t.Fatalf("expected comment-only default script to be treated as empty, got %q", got)
	}
}

func TestIsScriptEffectivelyEmpty(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty", "", true},
		{"shebang only", "#!/bin/bash\n", true},
		{"shebang and comments", "#!/bin/bash\n# comment\n# another\n", true},
		{"blank lines and comments", "\n# comment\n\n# more\n\n", true},
		{"has command", "#!/bin/bash\necho hello\n", false},
		{"command after comments", "#!/bin/bash\n# setup\napt-get install git\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isScriptEffectivelyEmpty(tt.input)
			if got != tt.want {
				t.Fatalf("isScriptEffectivelyEmpty(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolvePreparerSetupScript_LocalWithRepoSetupScript(t *testing.T) {
	req := &EnvPrepareRequest{
		ExecutorType:    executor.NameStandalone,
		RepositoryPath:  "/tmp/my-repo",
		RepoSetupScript: "make install",
	}

	got, err := resolvePreparerSetupScript(req, "/tmp/my-repo")
	if err != nil {
		t.Fatalf("resolvePreparerSetupScript() error = %v", err)
	}
	if got == "" {
		t.Fatal("expected non-empty script when repo setup script is set")
	}
	if !strings.Contains(got, "make install") {
		t.Fatalf("expected repo setup script in resolved output, got %q", got)
	}
}

func TestResolvePreparerSetupScript_WorktreeWithRepoSetupScript(t *testing.T) {
	req := &EnvPrepareRequest{
		ExecutorType:    executor.NameStandalone,
		UseWorktree:     true,
		RepositoryPath:  "/tmp/my-repo",
		RepoSetupScript: "npm ci",
	}

	got, err := resolvePreparerSetupScript(req, "/tmp/worktrees/wt-1")
	if err != nil {
		t.Fatalf("resolvePreparerSetupScript() error = %v", err)
	}
	if got == "" {
		t.Fatal("expected non-empty script when repo setup script is set")
	}
	if !strings.Contains(got, "npm ci") {
		t.Fatalf("expected repo setup script in resolved output, got %q", got)
	}
}

func TestResolvePreparerSetupScript_UsesExplicitScript(t *testing.T) {
	req := &EnvPrepareRequest{
		ExecutorType:   executor.NameStandalone,
		RepositoryPath: "/tmp/my-repo",
		SetupScript:    "echo {{repository.path}}",
	}

	got, err := resolvePreparerSetupScript(req, "/tmp/my-repo")
	if err != nil {
		t.Fatalf("resolvePreparerSetupScript() error = %v", err)
	}
	// Data placeholders resolve to a self-contained single-quoted shell token
	// (security: shellQuote) — functionally identical for echo, safe if the
	// value ever carried shell metacharacters.
	if strings.TrimSpace(got) != "echo '/tmp/my-repo'" {
		t.Fatalf("expected explicit script to be used and resolved, got %q", got)
	}
}

func TestResolvePreparerSetupScript_WorktreePlaceholders(t *testing.T) {
	req := &EnvPrepareRequest{
		ExecutorType:   executor.NameStandalone,
		UseWorktree:    true,
		RepositoryPath: "/tmp/main-repo",
		BaseBranch:     "main",
		WorktreeID:     "wt-123",
		WorktreeBranch: "feature/test-abc",
		SetupScript: strings.Join([]string{
			"echo {{worktree.base_path}}",
			"echo {{worktree.path}}",
			"echo {{worktree.id}}",
			"echo {{worktree.branch}}",
			"echo {{worktree.base_branch}}",
		}, "\n"),
	}

	got, err := resolvePreparerSetupScript(req, "/tmp/worktrees/wt-123")
	if err != nil {
		t.Fatalf("resolvePreparerSetupScript() error = %v", err)
	}
	// Data placeholders resolve to self-contained single-quoted tokens
	// (shellQuote); worktree.id is a kandev UUID and stays unquoted.
	expected := []string{
		"echo '/tmp/worktrees'",
		"echo '/tmp/worktrees/wt-123'",
		"echo wt-123",
		"echo 'feature/test-abc'",
		"echo 'main'",
	}
	for _, want := range expected {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in resolved script, got %q", want, got)
		}
	}
	if strings.Contains(got, "{{worktree.path}}") {
		t.Fatalf("expected worktree placeholders to be resolved, got %q", got)
	}
}

func TestResolvePreparerSetupScriptPropagatesContributionDestinationError(t *testing.T) {
	req := &EnvPrepareRequest{
		ExecutorType:            executor.NameStandalone,
		RepositoryPath:          "/tmp/my-repo",
		ContributionDestination: &models.ContributionDestination{},
	}

	if _, err := resolvePreparerSetupScript(req, "/tmp/my-repo"); err == nil {
		t.Fatal("resolvePreparerSetupScript accepted an invalid contribution destination")
	}
}
