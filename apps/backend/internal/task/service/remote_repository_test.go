package service

import "testing"

func TestParseRemoteRepositoryURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		raw       string
		provider  string
		owner     string
		repo      string
		canonical string
	}{
		{"github", "https://github.com/acme/api/pull/12", "github", "acme", "api", "https://github.com/acme/api.git"},
		{"gitlab subgroup", "https://gitlab.com/acme/platform/api.git", "gitlab", "acme/platform", "api", "https://gitlab.com/acme/platform/api.git"},
		{"azure devops", "https://dev.azure.com/acme/Platform/_git/api", "azure_devops", "Platform", "api", "https://dev.azure.com/acme/Platform/_git/api"},
		{"github ssh", "git@github.com:acme/api.git", "github", "acme", "api", "git@github.com:acme/api.git"},
		{"azure ssh", "git@ssh.dev.azure.com:v3/acme/Platform/api", "azure_devops", "Platform", "api", "git@ssh.dev.azure.com:v3/acme/Platform/api"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider, owner, repo, canonical, err := parseRemoteRepositoryURL(tc.raw)
			if err != nil {
				t.Fatal(err)
			}
			if provider != tc.provider || owner != tc.owner || repo != tc.repo || canonical != tc.canonical {
				t.Fatalf("got (%q, %q, %q, %q)", provider, owner, repo, canonical)
			}
		})
	}
}

func TestParseRemoteRepositoryURLRejectsUnsupportedHost(t *testing.T) {
	t.Parallel()
	if _, _, _, _, err := parseRemoteRepositoryURL("https://example.com/acme/api"); err == nil {
		t.Fatal("expected unsupported host error")
	}
}
