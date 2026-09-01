package lifecycle

import "testing"

// TestBuildWorktreeCreateRequestForwardsProfileEnv guards that resolved
// executor-profile env vars (already merged into EnvPrepareRequest.Env) are
// forwarded to the worktree CreateRequest so the repository setup script can
// use them (e.g. an npm auth token during install). Both single-repo and
// multi-repo launches build their CreateRequest here — multi-repo copies
// req.Env into each per-repo sub-request — so covering the builder covers both.
func TestBuildWorktreeCreateRequestForwardsProfileEnv(t *testing.T) {
	req := &EnvPrepareRequest{
		TaskID:         "task-1",
		RepositoryPath: "/repo",
		Env: map[string]string{
			"FONTAWESOME_NPM_AUTH_TOKEN": "fa-secret-value",
		},
	}

	got := buildWorktreeCreateRequest(req)

	if got.ScriptEnv["FONTAWESOME_NPM_AUTH_TOKEN"] != "fa-secret-value" {
		t.Fatalf("CreateRequest.ScriptEnv = %#v, want profile env forwarded", got.ScriptEnv)
	}
}

func TestBuildWorktreeCreateRequestAllowsExplicitBranchReplacement(t *testing.T) {
	req := &EnvPrepareRequest{
		WorkspaceReuseRequired: true,
		AllowBranchReplacement: true,
		RepositoryID:           "repo-1",
		RepositoryPath:         "/repo",
	}

	got := buildWorktreeCreateRequest(req)

	if !got.AllowBranchReplacement {
		t.Fatal("CreateRequest.AllowBranchReplacement = false, want true")
	}
	if got.ReuseRequired {
		t.Fatal("explicit branch replacement must not use attach-only ReuseRequired")
	}
}
