package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

func TestPrepareCursorMCPAuthEligibility(t *testing.T) {
	tests := []struct {
		name         string
		executor     string
		profile      *AgentProfileInfo
		strategy     mcpconfig.PassthroughMCPStrategy
		homeOverride string
		seedLink     bool
		wantLink     bool
		wantErr      bool
	}{
		{
			name:     "local Cursor strategy",
			executor: string(models.ExecutorTypeLocal),
			profile:  &AgentProfileInfo{CursorMCPAuthEnabled: true},
			strategy: mcpconfig.CursorStrategy{},
			wantLink: true,
		},
		{
			name:     "disabled profile removes owned link",
			executor: string(models.ExecutorTypeWorktree),
			profile:  &AgentProfileInfo{CursorMCPAuthEnabled: false},
			strategy: mcpconfig.CursorStrategy{},
			seedLink: true,
		},
		{
			name:     "remote execution is excluded",
			executor: string(models.ExecutorTypeMockRemote),
			profile:  &AgentProfileInfo{CursorMCPAuthEnabled: true},
			strategy: mcpconfig.CursorStrategy{},
		},
		{
			name:     "unknown execution locality is excluded",
			profile:  &AgentProfileInfo{CursorMCPAuthEnabled: true},
			strategy: mcpconfig.CursorStrategy{},
		},
		{
			name:     "other MCP strategy is excluded",
			executor: string(models.ExecutorTypeLocal),
			profile:  &AgentProfileInfo{CursorMCPAuthEnabled: true},
			strategy: mcpconfig.PiStrategy{},
		},
		{
			name:     "unresolved profile fails closed for local Cursor",
			executor: string(models.ExecutorTypeLocal),
			strategy: mcpconfig.CursorStrategy{},
			seedLink: true,
			wantLink: true,
			wantErr:  true,
		},
		{
			name:     "unresolved preference remains a no-op for remote execution",
			executor: string(models.ExecutorTypeMockRemote),
			strategy: mcpconfig.CursorStrategy{},
			seedLink: true,
			wantLink: true,
		},
		{
			name:         "different runtime home is excluded",
			executor:     string(models.ExecutorTypeLocal),
			profile:      &AgentProfileInfo{CursorMCPAuthEnabled: true},
			strategy:     mcpconfig.CursorStrategy{},
			homeOverride: filepath.Join(t.TempDir(), "other-home"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			workspace := t.TempDir()
			execution := &AgentExecution{WorkspacePath: workspace}
			if tc.homeOverride != "" {
				execution.setRuntimeEnvironment(map[string]string{"HOME": tc.homeOverride})
			}
			cursorHome := filepath.Join(home, ".cursor")
			projects := filepath.Join(cursorHome, "projects")
			if err := os.MkdirAll(filepath.Join(projects, "other-project"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(projects, "other-project", "mcp-auth.json"), []byte(`{"mcp":{"token":"secret"}}`), 0o600); err != nil {
				t.Fatal(err)
			}

			destination := cursorMCPAuthDestinationForTest(t, projects, workspace)
			if tc.seedLink {
				if err := mcpconfig.LinkCursorMCPAuth(workspace, cursorHome); err != nil {
					t.Fatalf("create existing link: %v", err)
				}
			}

			mgr := newTestManager(t)
			err := mgr.prepareCursorMCPAuth(execution, tc.profile, tc.executor, tc.strategy)
			if tc.wantErr {
				if err == nil {
					t.Fatal("prepareCursorMCPAuth returned nil for unresolved local Cursor preference")
				}
			} else if err != nil {
				t.Fatalf("prepareCursorMCPAuth: %v", err)
			}

			_, err = os.Lstat(destination)
			gotLink := err == nil
			if gotLink != tc.wantLink {
				t.Fatalf("destination exists = %v, want %v (err=%v)", gotLink, tc.wantLink, err)
			}
		})
	}
}

func cursorMCPAuthDestinationForTest(t *testing.T, projects, workspace string) string {
	t.Helper()
	absPath, err := filepath.Abs(workspace)
	if err != nil {
		t.Fatal(err)
	}
	canonicalPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(projects, mcpconfig.DeriveCursorProjectSlug(canonicalPath), "mcp-auth.json")
}

type cursorMCPAuthProfileResolution struct {
	profile *AgentProfileInfo
	err     error
}

type cursorMCPAuthSequenceResolver struct {
	results []cursorMCPAuthProfileResolution
	calls   int
}

func (r *cursorMCPAuthSequenceResolver) ResolveProfile(context.Context, string) (*AgentProfileInfo, error) {
	if r.calls >= len(r.results) {
		return nil, fmt.Errorf("unexpected profile resolver call %d", r.calls+1)
	}
	result := r.results[r.calls]
	r.calls++
	return result.profile, result.err
}

func TestResumePassthroughSessionAppliesCursorOptOutBeforeStartingProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-backed process assertion is not available on Windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	mgr, runner, execution := newPassthroughRunnerManager(t)
	t.Cleanup(func() {
		if execution.PassthroughProcessID != "" {
			_ = runner.Stop(context.Background(), execution.PassthroughProcessID)
		}
	})
	execution.ExecutorType = string(models.ExecutorTypeLocal)

	workspace := execution.WorkspacePath
	projects := filepath.Join(home, ".cursor", "projects")
	source := filepath.Join(projects, "source-project", "mcp-auth.json")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte(`{"figma":{"token":"opaque"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := mcpconfig.LinkCursorMCPAuth(workspace, filepath.Join(home, ".cursor")); err != nil {
		t.Fatalf("seed existing bridge link: %v", err)
	}
	destination := cursorMCPAuthDestinationForTest(t, projects, workspace)
	if _, err := os.Readlink(destination); err != nil {
		t.Fatalf("expected existing bridge link: %v", err)
	}

	const agentName = "cursor-auth-fail-closed-test"
	if err := mgr.registry.Register(&testAgent{
		id: agentName,
		StandardPassthrough: agents.StandardPassthrough{Cfg: agents.PassthroughConfig{
			Supported:      true,
			PassthroughCmd: agents.NewCommand("sh", "-c", "sleep 30"),
			MCPStrategy:    mcpconfig.CursorStrategy{},
		}},
	}); err != nil {
		t.Fatalf("register Cursor terminal test agent: %v", err)
	}
	profile := &AgentProfileInfo{
		ProfileID:            execution.AgentProfileID,
		AgentName:            agentName,
		CursorMCPAuthEnabled: false,
	}
	// The intermediate error models a transient duplicate launch-profile lookup.
	// The following success lets the old path reach PTY startup after it has
	// discarded the disabled preference; the combined lookup must avoid that.
	resolver := &cursorMCPAuthSequenceResolver{results: []cursorMCPAuthProfileResolution{
		{profile: profile},
		{err: fmt.Errorf("transient second profile lookup failure")},
		{profile: profile},
	}}
	mgr.profileResolver = resolver

	err := mgr.ResumePassthroughSession(context.Background(), execution.SessionID)
	if err == nil || !strings.Contains(err.Error(), "transient second profile lookup failure") {
		t.Fatalf("ResumePassthroughSession error = %v, want profile environment lookup failure", err)
	}
	if resolver.calls != 2 {
		t.Fatalf("profile resolver calls = %d, want 2 (one launch-profile lookup and one environment lookup)", resolver.calls)
	}
	if execution.PassthroughProcessID != "" {
		t.Fatalf("passthrough process started despite profile environment lookup failure: %s", execution.PassthroughProcessID)
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("resolved opt-out was not applied before launch failure: %v", err)
	}
}

func TestPrepareCursorMCPAuthDoesNotCreateMissingCursorHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mgr := newTestManager(t)
	if err := mgr.prepareCursorMCPAuth(
		&AgentExecution{WorkspacePath: t.TempDir()},
		&AgentProfileInfo{CursorMCPAuthEnabled: true},
		string(models.ExecutorTypeLocal),
		mcpconfig.CursorStrategy{},
	); err != nil {
		t.Fatalf("prepareCursorMCPAuth: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".cursor")); !os.IsNotExist(err) {
		t.Fatalf("missing Cursor home was created or produced an unexpected error: %v", err)
	}
}

func TestApplyPassthroughMCPPreparesCursorAuth(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workspace := t.TempDir()
	cursorProjects := filepath.Join(home, ".cursor", "projects")
	if err := os.MkdirAll(filepath.Join(cursorProjects, "other-project"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cursorProjects, "other-project", "mcp-auth.json"), []byte(`{"figma":{"token":"opaque"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	mgr := newTestManager(t)
	execution := &AgentExecution{
		ID:             "exec-terminal-cursor",
		AgentProfileID: "profile-cursor-enabled",
		WorkspacePath:  workspace,
		ExecutorType:   string(models.ExecutorTypeWorktree),
		standalonePort: 45678,
	}
	_, err := mgr.applyPassthroughMCP(
		context.Background(), execution,
		agents.PassthroughConfig{MCPStrategy: mcpconfig.CursorStrategy{}}, nil,
		&AgentProfileInfo{CursorMCPAuthEnabled: true},
	)
	if err != nil {
		t.Fatalf("applyPassthroughMCP: %v", err)
	}

	destination := cursorMCPAuthDestinationForTest(t, cursorProjects, workspace)
	if _, err := os.Readlink(destination); err != nil {
		t.Fatalf("terminal Cursor auth link was not prepared: %v", err)
	}
}

func TestResumePassthroughCommandPreparesCursorAuth(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workspace := t.TempDir()
	cursorProjects := filepath.Join(home, ".cursor", "projects")
	if err := os.MkdirAll(filepath.Join(cursorProjects, "source-project"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cursorProjects, "source-project", "mcp-auth.json"), []byte(`{"figma":{"token":"opaque"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	terminalAgent := &testAgent{
		id: "cursor-terminal",
		StandardPassthrough: agents.StandardPassthrough{Cfg: agents.PassthroughConfig{
			Supported:      true,
			PassthroughCmd: agents.NewCommand("cursor"),
			MCPStrategy:    mcpconfig.CursorStrategy{},
		}},
	}
	execution := &AgentExecution{
		ID:             "exec-cursor-resume",
		WorkspacePath:  workspace,
		ExecutorType:   string(models.ExecutorTypeLocal),
		standalonePort: 45678,
	}
	resolved := &resolvedPassthrough{
		agentConfig: terminalAgent,
		agent:       terminalAgent,
		pt:          terminalAgent.PassthroughConfig(),
		profile:     &AgentProfileInfo{CursorMCPAuthEnabled: true},
	}

	mgr := newTestManager(t)
	if _, err := mgr.resumePassthroughCommand(context.Background(), execution, resolved, true); err != nil {
		t.Fatalf("resumePassthroughCommand: %v", err)
	}
	destination := cursorMCPAuthDestinationForTest(t, cursorProjects, workspace)
	if _, err := os.Readlink(destination); err != nil {
		t.Fatalf("resumed terminal Cursor auth link was not prepared: %v", err)
	}
}

func TestPrepareCursorMCPAuthFailurePolicy(t *testing.T) {
	t.Run("enabled publication failure does not block project MCP setup", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		if err := os.MkdirAll(filepath.Join(home, ".cursor"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, ".cursor", "projects"), []byte("blocked"), 0o600); err != nil {
			t.Fatal(err)
		}
		mgr := newTestManager(t)
		agentConfig, ok := mgr.registry.Get("cursor-acp")
		if !ok {
			t.Fatal("cursor-acp agent missing from test registry")
		}
		err := mgr.materializeRuntimeProjectMCP(
			context.Background(),
			&AgentExecution{ID: "exec-cursor", WorkspacePath: t.TempDir()},
			agentConfig,
			&AgentProfileInfo{CursorMCPAuthEnabled: true},
			string(models.ExecutorTypeLocal),
		)
		if err != nil {
			t.Fatalf("enabled bridge failure blocked normal project MCP setup: %v", err)
		}
	})

	t.Run("disabled cleanup failure is sanitized and blocks launch", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		if err := os.MkdirAll(filepath.Join(home, ".cursor"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, ".cursor", "projects"), []byte("blocked"), 0o600); err != nil {
			t.Fatal(err)
		}
		mgr := newTestManager(t)
		err := mgr.prepareCursorMCPAuth(
			&AgentExecution{WorkspacePath: t.TempDir()},
			&AgentProfileInfo{CursorMCPAuthEnabled: false},
			string(models.ExecutorTypeLocal),
			mcpconfig.CursorStrategy{},
		)
		if err == nil {
			t.Fatal("expected an error when disabled cleanup cannot inspect the Cursor project path")
		}
		if got, want := err.Error(), "failed to remove shared Cursor MCP credentials"; got != want {
			t.Fatalf("error = %q, want sanitized %q", got, want)
		}
		if strings.Contains(err.Error(), home) {
			t.Fatalf("sanitized error contains the user home path: %q", err)
		}
	})
}

func TestMaterializeRuntimeProjectMCPPreparesCursorAuthWithoutServers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workspace := t.TempDir()
	cursorProjects := filepath.Join(home, ".cursor", "projects")
	if err := os.MkdirAll(filepath.Join(cursorProjects, "other-project"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cursorProjects, "other-project", "mcp-auth.json"), []byte(`{"figma":{"token":"opaque"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	mgr := newTestManager(t)
	execution := &AgentExecution{
		ID:             "exec-cursor-empty-mcp",
		WorkspacePath:  workspace,
		RuntimeName:    "standalone",
		ExecutorType:   string(models.ExecutorTypeLocal),
		AgentProfileID: "profile-cursor-enabled",
	}
	agentConfig, ok := mgr.registry.Get("cursor-acp")
	if !ok {
		t.Fatal("cursor-acp agent missing from test registry")
	}

	if err := mgr.materializeRuntimeProjectMCP(
		context.Background(), execution, agentConfig,
		&AgentProfileInfo{CursorMCPAuthEnabled: true}, string(models.ExecutorTypeLocal),
	); err != nil {
		t.Fatalf("materializeRuntimeProjectMCP: %v", err)
	}

	destination := cursorMCPAuthDestinationForTest(t, cursorProjects, workspace)
	target, err := os.Readlink(destination)
	if err != nil {
		t.Fatalf("Cursor auth link was not prepared before the empty-server return: %v", err)
	}
	if target != filepath.Join(home, ".cursor", "kandev-mcp-auth-unified.json") {
		t.Fatalf("link target = %q", target)
	}
}

func TestSameCursorMCPAuthHomeRecognizesSymlinkAlias(t *testing.T) {
	actualHome := t.TempDir()
	alias := filepath.Join(t.TempDir(), "home-alias")
	if err := os.Symlink(actualHome, alias); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	if !sameCursorMCPAuthHome(actualHome, alias) {
		t.Fatalf("sameCursorMCPAuthHome(%q, %q) = false, want true", actualHome, alias)
	}
}

func TestPrepareCursorMCPAuthExcludesConfiguredTaskWorktrees(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cursorProjects := filepath.Join(home, ".cursor", "projects")
	taskRoot := filepath.Join(t.TempDir(), "custom-task-storage")
	worktreeManager, err := worktree.NewManager(worktree.Config{TasksBasePath: taskRoot}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	taskWorkspace := filepath.Join(taskRoot, "task-123", "repository")
	if err := os.MkdirAll(taskWorkspace, 0o755); err != nil {
		t.Fatal(err)
	}
	taskAuthPath := cursorMCPAuthDestinationForTest(t, cursorProjects, taskWorkspace)
	if err := os.MkdirAll(filepath.Dir(taskAuthPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(taskAuthPath, []byte(`{"task-secret":{"token":"excluded"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cursorProjects, "ordinary-project"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cursorProjects, "ordinary-project", "mcp-auth.json"), []byte(`{"ordinary":{"token":"included"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	mgr := newTestManager(t)
	mgr.worktreeMgr = worktreeManager
	if err := mgr.prepareCursorMCPAuth(
		&AgentExecution{WorkspacePath: t.TempDir(), ExecutorType: string(models.ExecutorTypeLocal)},
		&AgentProfileInfo{CursorMCPAuthEnabled: true},
		string(models.ExecutorTypeLocal),
		mcpconfig.CursorStrategy{},
	); err != nil {
		t.Fatalf("prepareCursorMCPAuth: %v", err)
	}

	master, err := os.ReadFile(filepath.Join(home, ".cursor", "kandev-mcp-auth-unified.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(master), "task-secret") {
		t.Fatalf("configured task auth leaked into shared file: %s", master)
	}
	if !strings.Contains(string(master), "ordinary") {
		t.Fatalf("ordinary project auth was excluded: %s", master)
	}
}
