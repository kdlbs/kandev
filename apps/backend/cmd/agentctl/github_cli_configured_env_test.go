package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	processconfig "github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/githubauth"
)

// @covers AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.15
func TestConfiguredManagedGitHubCLIEnvironment(t *testing.T) {
	for _, scenario := range []string{"current lease", "foreign repository", "broker failure"} {
		t.Run(scenario, func(t *testing.T) {
			var brokerRequest githubBrokerResolveRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&brokerRequest); err != nil {
					t.Errorf("decode broker request: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if scenario == "broker failure" {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				_, _ = io.WriteString(w, `{"username":"x-access-token","password":"current-synthetic-token"}`)
			}))
			t.Cleanup(server.Close)

			shimDir := filepath.Join(t.TempDir(), "installed github shims")
			startupEnv := filepath.Join(shimDir, githubauth.CLIBashEnvFilename)
			parentEnv := filepath.Join(t.TempDir(), "parent bash env.sh")
			cfg := &processconfig.InstanceConfig{WorkDir: t.TempDir(), AgentEnv: []string{
				"PATH=/usr/bin:/bin",
				"BASH_ENV=" + parentEnv,
				"GH_TOKEN=ambient-host-token",
				"GITHUB_TOKEN=ambient-github-token",
				githubauth.CredentialBrokerURLEnv + "=https://old-broker.example/resolve",
				githubauth.CredentialLeaseEnv + "=old-lease",
				githubauth.CredentialTaskIDEnv + "=old-task",
				githubauth.CredentialSessionIDEnv + "=old-session",
				githubauth.CredentialRepositoryEnv + "=old-repository",
				githubauth.CredentialOwnerEnv + "=old-owner",
				githubauth.CredentialRepoEnv + "=old-repo",
				githubauth.CredentialHostEnv + "=github.com",
				githubauth.CredentialHelperPathEnv + "=/worker/agentctl",
				githubauth.CredentialCLIShimDirEnv + "=" + shimDir,
				githubauth.CredentialCLIBashEnvEnv + "=" + startupEnv,
			}}
			manager := process.NewManager(cfg, newTestLogger(t))
			t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
			leaseScopes := []githubBrokerResolveRequest{{
				Lease:        "current-lease",
				TaskID:       "task-1",
				SessionID:    "session-1",
				RepositoryID: "repository-current",
				Owner:        "acme",
				Repo:         "backend",
				Host:         "github.com",
			}}
			scopeJSON, err := json.Marshal(leaseScopes)
			if err != nil {
				t.Fatalf("marshal lease scope: %v", err)
			}
			request := map[string]string{
				githubauth.CredentialBrokerURLEnv:  server.URL,
				githubauth.CredentialLeaseEnv:      "current-lease",
				githubauth.CredentialTaskIDEnv:     "task-1",
				githubauth.CredentialSessionIDEnv:  "session-1",
				githubauth.CredentialRepositoryEnv: "repository-current",
				githubauth.CredentialOwnerEnv:      "acme",
				githubauth.CredentialRepoEnv:       "backend",
				githubauth.CredentialHostEnv:       "github.com",
				githubauth.CredentialScopesEnv:     string(scopeJSON),
				"GH_REPO":                          "acme/backend",
				// Carry these only as evidence that managed CLI failures do not
				// fall back to ambient host authentication.
				"GH_TOKEN":     "ambient-host-token",
				"GITHUB_TOKEN": "ambient-github-token",
			}
			if err := manager.Configure("echo", nil, false, request, "", nil, false); err != nil {
				t.Fatalf("Configure() error = %v", err)
			}
			effective := environmentMapFromSliceForTest(cfg.AgentEnv)
			if got := effective[githubauth.CredentialLeaseEnv]; got != "current-lease" {
				t.Fatalf("configured lease = %q, want current lease", got)
			}
			if got := effective[githubauth.CredentialCLIShimDirEnv]; got != shimDir {
				t.Fatalf("configured shim directory = %q, want installed directory %q", got, shimDir)
			}
			if got := effective[githubauth.CredentialCLIBashEnvEnv]; got != startupEnv {
				t.Fatalf("configured Bash hook = %q, want installed hook %q", got, startupEnv)
			}
			if strings.Contains(strings.Join(cfg.AgentEnv, "\n"), "old-lease") ||
				strings.Contains(strings.Join(cfg.AgentEnv, "\n"), "old-repository") {
				t.Fatal("old credential identity survived Configure")
			}

			if scenario == "foreign repository" {
				request["GH_REPO"] = "acme/foreign"
				effective["GH_REPO"] = "acme/foreign"
			}
			var cliCalls int
			var childToken string
			runner := func(_ context.Context, executable string, _ []string, childEnv []string, _ io.Reader, _, _ io.Writer) error {
				cliCalls++
				if executable != "/usr/bin/gh" {
					t.Errorf("real CLI executable = %q, want /usr/bin/gh", executable)
				}
				childToken = envValue(childEnv, "GH_TOKEN")
				if got := envValue(childEnv, "GITHUB_TOKEN"); got != "" {
					t.Errorf("child GITHUB_TOKEN = %q, want removed", got)
				}
				return nil
			}
			err = runGitHubCLIShim(
				context.Background(), []string{"pr", "list"}, strings.NewReader(""), io.Discard, io.Discard,
				lookupEnv(effective), func() []string { return envMap(effective) }, server.Client(),
				effective[githubauth.CredentialCLIShimDirEnv],
				func(name, searchPath string) (string, error) {
					if name != "gh" || searchPath != "/usr/bin:/bin" {
						t.Errorf("real CLI lookup = %q in %q", name, searchPath)
					}
					return "/usr/bin/gh", nil
				},
				runner,
			)
			if scenario == "current lease" {
				if err != nil {
					t.Fatalf("runGitHubCLIShim() error = %v", err)
				}
				if cliCalls != 1 || childToken != "current-synthetic-token" {
					t.Fatalf("CLI calls/token = %d/%q, want one call with broker token", cliCalls, childToken)
				}
				if brokerRequest.Lease != "current-lease" || brokerRequest.TaskID != "task-1" ||
					brokerRequest.SessionID != "session-1" || brokerRequest.RepositoryID != "repository-current" {
					t.Fatalf("broker request = %+v, want current configured lease identity", brokerRequest)
				}
				return
			}
			if err == nil {
				t.Fatal("runGitHubCLIShim() unexpectedly succeeded")
			}
			if cliCalls != 0 {
				t.Fatalf("CLI ran %d times after %s", cliCalls, scenario)
			}
			if scenario == "foreign repository" && brokerRequest.Lease != "" {
				t.Fatalf("foreign scope reached the broker with lease %q", brokerRequest.Lease)
			}
		})
	}
}

func environmentMapFromSliceForTest(env []string) map[string]string {
	values := make(map[string]string, len(env))
	for _, entry := range env {
		if key, value, ok := strings.Cut(entry, "="); ok {
			values[key] = value
		}
	}
	return values
}
