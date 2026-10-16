package main

import (
	"github.com/kandev/kandev/internal/githubhost"
	"testing"
)

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-004.2
func TestEnterpriseCLITargetBoundaries(t *testing.T) {
	host, _ := githubhost.Parse("git.company.example")
	scope := githubBrokerResolveRequest{Host: host.String(), Owner: "acme", Repo: "repo"}
	for _, tc := range []struct {
		name    string
		args    []string
		repo    string
		invalid bool
	}{
		{"same host", []string{"api", "user", "--hostname=git.company.example"}, "", false},
		{"same API", []string{"api", "https://git.company.example/api/v3/repos/acme/repo/pulls"}, "", false},
		{"same GraphQL", []string{"api", "https://git.company.example/api/graphql"}, "", false},
		{"same PR", []string{"pr", "view", "https://git.company.example/acme/repo/pull/1"}, "", false},
		{"foreign content is harmless", []string{"pr", "comment", "1", "--body", "https://foreign.example/docs"}, "", false},
		{"foreign repo", []string{"pr", "view", "https://git.company.example/other/repo/pull/1"}, "", true},
		{"foreign API repo", []string{"api", "https://git.company.example/api/v3/repos/other/repo/pulls"}, "", true},
		{"environment repo", []string{"pr", "list"}, "foreign.example/acme/repo", true},
		{"malformed hostname", []string{"api", "user", "--hostname"}, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateGitHubCLITargets(tc.args, func(key string) string {
				if key == "GH_REPO" {
					return tc.repo
				}
				return ""
			}, scope, host)
			if (err != nil) != tc.invalid {
				t.Fatalf("target validation = %v; invalid=%v", err, tc.invalid)
			}
		})
	}
}
