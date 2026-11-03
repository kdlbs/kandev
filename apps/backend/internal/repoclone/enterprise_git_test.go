package repoclone

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-004.1
func TestEnterpriseGitCredentialLookup(t *testing.T) {
	for _, host := range []string{"github.com", "acme.ghe.com", "git.company.example:8443"} {
		t.Run(host, func(t *testing.T) {
			for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN"} {
				t.Setenv(key, "ambient")
			}
			for _, target := range []string{host, "foreign.company.example"} {
				cmd := exec.CommandContext(context.Background(), "git", "credential", "fill")
				configureTestGitCommand(t, cmd, &cloneAuth{origin: "https://" + host, username: "workspace-user", password: "workspace-secret"})
				for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN"} {
					if envValue(cmd.Env, key) != "" {
						t.Errorf("%s reached managed Git", key)
					}
				}
				cmd.Stdin = strings.NewReader("protocol=https\nhost=" + target + "\npath=acme/repo.git\n\n")
				output, err := cmd.CombinedOutput()
				if target == host {
					if err != nil || !strings.Contains(string(output), "password=workspace-secret") {
						t.Fatalf("scoped Git lookup: %s %v", output, err)
					}
				} else if err == nil || strings.Contains(string(output), "workspace-secret") {
					t.Fatalf("foreign Git lookup returned credential: %s %v", output, err)
				}
			}
		})
	}
}
