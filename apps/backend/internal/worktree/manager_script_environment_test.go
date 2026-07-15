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
	mgr.runWorktreeSetupScript(context.Background(), worktree)
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
