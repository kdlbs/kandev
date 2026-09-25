package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/githubauth"
)

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
		IntegrationRef: "develop",
		Env: map[string]string{
			"FONTAWESOME_NPM_AUTH_TOKEN": "fa-secret-value",
		},
	}

	got := buildWorktreeCreateRequest(req)

	if got.ScriptEnv["FONTAWESOME_NPM_AUTH_TOKEN"] != "fa-secret-value" {
		t.Fatalf("CreateRequest.ScriptEnv = %#v, want profile env forwarded", got.ScriptEnv)
	}
	if got.IntegrationRef != "develop" {
		t.Fatalf("CreateRequest.IntegrationRef = %q, want develop", got.IntegrationRef)
	}
}

func TestBuildWorktreeCreateRequestExcludesManagedGitCredentialsFromSetupScript(t *testing.T) {
	req := &EnvPrepareRequest{
		Env: map[string]string{
			"PROFILE_NPM_TOKEN":                       "profile-secret",
			githubauth.CredentialBrokerURLEnv:         "https://kandev.example/resolve",
			githubauth.CredentialLeaseEnv:             "opaque-lease",
			githubauth.CredentialReissueCapabilityEnv: "reissue-capability",
			githubauth.CredentialTaskIDEnv:            "task-1",
			githubauth.CredentialSessionIDEnv:         "session-1",
			githubauth.CredentialRepositoryEnv:        "repo-1",
			githubauth.CredentialHelperPathEnv:        "/opt/kandev/agentctl",
			"GIT_CONFIG_COUNT":                        "5",
			"GIT_CONFIG_KEY_0":                        "core.hooksPath",
			"GIT_CONFIG_VALUE_0":                      "/profile/hooks",
			"GIT_CONFIG_KEY_1":                        "credential.https://github.com.helper",
			"GIT_CONFIG_VALUE_1":                      "",
			"GIT_CONFIG_KEY_2":                        "credential.https://github.com.helper",
			"GIT_CONFIG_VALUE_2":                      githubauth.ManagedGitCredentialHelper,
			"GIT_CONFIG_KEY_3":                        "credential.https://gitlab.example.helper",
			"GIT_CONFIG_VALUE_3":                      "!/usr/local/bin/custom-credential-helper",
			"GIT_CONFIG_KEY_4":                        "credential.https://github.com.helper",
			"GIT_CONFIG_VALUE_4":                      "!f() { : " + githubauth.HostGitHubCredentialHelperMarker + "; gh auth git-credential \"$@\"; }; f",
		},
	}

	got := buildWorktreeCreateRequest(req).ScriptEnv

	if got["PROFILE_NPM_TOKEN"] != "profile-secret" {
		t.Fatalf("profile environment missing from setup script: %#v", got)
	}
	for _, key := range []string{
		githubauth.CredentialBrokerURLEnv,
		githubauth.CredentialLeaseEnv,
		githubauth.CredentialReissueCapabilityEnv,
		githubauth.CredentialTaskIDEnv,
		githubauth.CredentialSessionIDEnv,
		githubauth.CredentialRepositoryEnv,
		githubauth.CredentialHelperPathEnv,
	} {
		if _, exists := got[key]; exists {
			t.Fatalf("setup script received managed Git credential %q: %#v", key, got)
		}
	}
	if got["GIT_CONFIG_COUNT"] != "2" ||
		got["GIT_CONFIG_KEY_0"] != "core.hooksPath" || got["GIT_CONFIG_VALUE_0"] != "/profile/hooks" ||
		got["GIT_CONFIG_KEY_1"] != "credential.https://gitlab.example.helper" || got["GIT_CONFIG_VALUE_1"] != "!/usr/local/bin/custom-credential-helper" {
		t.Fatalf("setup script Git config = %#v, want only user-owned entries", got)
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

func TestBuildWorktreeCreateRequestScopesCheckoutCredentials(t *testing.T) {
	env := map[string]string{
		"GIT_DIR": "/profile/git", "PROFILE_SECRET": "not-for-git",
		githubauth.CredentialBrokerURLEnv: "https://broker.example/resolve",
		githubauth.CredentialLeaseEnv:     "lease", githubauth.CredentialHelperPathEnv: "/opt/agentctl",
		"GIT_CONFIG_COUNT": "4",
		"GIT_CONFIG_KEY_0": "core.hooksPath", "GIT_CONFIG_VALUE_0": "/profile/hooks",
		"GIT_CONFIG_KEY_1": "credential.https://github.com.helper", "GIT_CONFIG_VALUE_1": "",
		"GIT_CONFIG_KEY_2": "credential.https://github.com.helper", "GIT_CONFIG_VALUE_2": githubauth.ManagedGitCredentialHelper,
		"GIT_CONFIG_KEY_3": "credential.https://github.com.useHttpPath", "GIT_CONFIG_VALUE_3": "true",
	}
	got := buildWorktreeCreateRequest(&EnvPrepareRequest{Env: env}).CheckoutEnv
	if got["GIT_DIR"] != "" || got["PROFILE_SECRET"] != "" {
		t.Fatal("profile environment reached checkout")
	}
	if got[githubauth.CredentialLeaseEnv] != "lease" || got[githubauth.CredentialHelperPathEnv] != "/opt/agentctl" {
		t.Fatal("managed authentication lost")
	}
	if got["GIT_CONFIG_COUNT"] != "3" || got["GIT_CONFIG_KEY_0"] != "credential.https://github.com.helper" || got["GIT_CONFIG_VALUE_1"] != githubauth.ManagedGitCredentialHelper || got["GIT_CONFIG_KEY_2"] != "credential.https://github.com.useHttpPath" {
		t.Fatalf("checkout config = %#v", got)
	}
}
