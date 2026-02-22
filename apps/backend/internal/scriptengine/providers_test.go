package scriptengine

import "testing"

func TestRepositoryProvider_UsesRepositorySetupScriptKey(t *testing.T) {
	provider := RepositoryProvider(map[string]any{
		"repository_path":         "/tmp/repo",
		"base_branch":             "main",
		"repository_setup_script": "npm ci",
		"setup_script":            "should-not-be-used",
	}, nil, nil, nil)

	vars := provider()
	if got := vars["repository.setup_script"]; got != "npm ci" {
		t.Fatalf("repository.setup_script = %q, want %q", got, "npm ci")
	}
}
