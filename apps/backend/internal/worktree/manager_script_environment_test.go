package worktree

import (
	"context"
	"testing"
)

type staticScriptEnvironment struct {
	env map[string]string
}

func (p staticScriptEnvironment) ExecutionEnvironment(context.Context) (map[string]string, error) {
	return p.env, nil
}

type cleanupEnvironmentRecorder struct {
	setupReq   ScriptExecutionRequest
	cleanupReq ScriptExecutionRequest
}

func (r *cleanupEnvironmentRecorder) ExecuteSetupScript(_ context.Context, req ScriptExecutionRequest) error {
	r.setupReq = req
	return nil
}

func (r *cleanupEnvironmentRecorder) ExecuteCleanupScript(_ context.Context, req ScriptExecutionRequest) error {
	r.cleanupReq = req
	return nil
}

func TestRunWorktreeCleanupScriptInjectsManagedGoCacheOnly(t *testing.T) {
	provider := &fakeRepoProvider{repo: &Repository{
		ID:            "repo-1",
		SetupScript:   "go build ./...",
		CleanupScript: "go clean -cache",
	}}
	recorder := &cleanupEnvironmentRecorder{}
	mgr := newManagerForSetupTest(t, provider, recorder)
	mgr.SetScriptEnvironmentProvider(staticScriptEnvironment{env: map[string]string{
		"GOCACHE": "/opt/kandev/cache/go-build",
		"TOKEN":   "must-not-be-injected",
	}})

	worktree := &Worktree{
		ID:           "worktree-1",
		TaskID:       "task-1",
		SessionID:    "session-1",
		RepositoryID: "repo-1",
		Path:         t.TempDir(),
	}
	mgr.runWorktreeSetupScript(context.Background(), worktree, nil)
	mgr.runWorktreeCleanupScript(context.Background(), worktree)

	if got := recorder.setupReq.Env["GOCACHE"]; got != "/opt/kandev/cache/go-build" {
		t.Fatalf("setup GOCACHE = %q, want managed path", got)
	}
	if got := recorder.cleanupReq.Env["GOCACHE"]; got != "/opt/kandev/cache/go-build" {
		t.Fatalf("cleanup GOCACHE = %q, want managed path", got)
	}
	if _, exists := recorder.cleanupReq.Env["TOKEN"]; exists {
		t.Fatalf("cleanup script received unrelated provider environment: %#v", recorder.cleanupReq.Env)
	}
}

// TestManagerCreate_SetupScriptReceivesProfileEnv covers the fix for the
// executor-profile env var (e.g. an npm auth token) not reaching the repository
// setup script. Create() runs the per-repo setup script, and CreateRequest.ScriptEnv
// (resolved executor-profile env) must be exported into that script's process
// environment while the managed GOCACHE is preserved. Exercises the single-repo
// path; the multi-repo path builds the same CreateRequest via
// buildWorktreeCreateRequest, so this also guards it (see the lifecycle test).
func TestManagerCreate_SetupScriptReceivesProfileEnv(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)
	provider := &fakeRepoProvider{repo: &Repository{ID: "repo-env", SetupScript: "npm install"}}
	recorder := &cleanupEnvironmentRecorder{}
	mgr := newManagerForSetupTest(t, provider, recorder)
	mgr.SetScriptEnvironmentProvider(staticScriptEnvironment{env: map[string]string{
		"GOCACHE": "/opt/kandev/cache/go-build",
	}})

	_, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-env",
		SessionID:      "session-env",
		TaskTitle:      "Setup script needs token",
		RepositoryID:   "repo-env",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		TaskDirName:    "task-env",
		RepoName:       "repo-env",
		ScriptEnv: map[string]string{
			"FONTAWESOME_NPM_AUTH_TOKEN": "fa-secret-value",
		},
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if got := recorder.setupReq.Env["FONTAWESOME_NPM_AUTH_TOKEN"]; got != "fa-secret-value" {
		t.Fatalf("setup script FONTAWESOME_NPM_AUTH_TOKEN = %q, want profile secret; env=%#v",
			got, recorder.setupReq.Env)
	}
	if got := recorder.setupReq.Env["GOCACHE"]; got != "/opt/kandev/cache/go-build" {
		t.Fatalf("setup script GOCACHE = %q, want managed path preserved", got)
	}
}

func TestMergeScriptEnv(t *testing.T) {
	if got := mergeScriptEnv(nil, nil); got != nil {
		t.Fatalf("mergeScriptEnv(nil, nil) = %#v, want nil (inherit full process env)", got)
	}

	// Managed values win over profile entries so the build cache is never
	// clobbered by a user-supplied variable of the same name.
	merged := mergeScriptEnv(
		map[string]string{"TOKEN": "secret", "GOCACHE": "/profile/override"},
		map[string]string{"GOCACHE": "/managed/cache"},
	)
	if merged["TOKEN"] != "secret" {
		t.Fatalf("merged TOKEN = %q, want profile value", merged["TOKEN"])
	}
	if merged["GOCACHE"] != "/managed/cache" {
		t.Fatalf("merged GOCACHE = %q, want managed value to win", merged["GOCACHE"])
	}

	// Profile-only: managed nil, profile entries survive.
	if got := mergeScriptEnv(map[string]string{"TOKEN": "secret"}, nil); got["TOKEN"] != "secret" {
		t.Fatalf("mergeScriptEnv(profile, nil)[TOKEN] = %q, want profile value", got["TOKEN"])
	}

	// Managed-only: profile nil, managed entries survive.
	if got := mergeScriptEnv(nil, map[string]string{"GOCACHE": "/managed/cache"}); got["GOCACHE"] != "/managed/cache" {
		t.Fatalf("mergeScriptEnv(nil, managed)[GOCACHE] = %q, want managed value", got["GOCACHE"])
	}
}
