package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/githubauth"
)

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-004.2
func TestEnterpriseCLIShimRouting(t *testing.T) {
	for _, tc := range []struct{ host, tokenKey string }{
		{"github.com", "GH_TOKEN"},
		{"acme.ghe.com", "GH_TOKEN"},
		{"git.company.example", "GH_ENTERPRISE_TOKEN"},
	} {
		t.Run(tc.host, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req githubBrokerResolveRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				if req.Host != tc.host {
					t.Errorf("redeemed host = %q", req.Host)
				}
				_ = json.NewEncoder(w).Encode(map[string]string{"username": "x-access-token", "password": "scoped-token"})
			}))
			t.Cleanup(server.Close)
			env := githubCredentialTestEnv(server.URL)
			env[githubauth.CredentialHostEnv] = tc.host
			env["PATH"] = "/shim:/usr/bin"
			env["GH_HOST"] = "foreign.example"
			for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN"} {
				env[key] = "ambient"
			}
			called := false
			runner := func(_ context.Context, _ string, _ []string, child []string, _ io.Reader, _, _ io.Writer) error {
				called = true
				if got := envValue(child, "GH_HOST"); got != tc.host {
					t.Errorf("GH_HOST = %q, want %q", got, tc.host)
				}
				for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN"} {
					want := ""
					if key == tc.tokenKey {
						want = "scoped-token"
					}
					if got := envValue(child, key); got != want {
						t.Errorf("%s = %q, want %q", key, got, want)
					}
				}
				return nil
			}
			err := runGitHubCLIShim(context.Background(), []string{"pr", "list"}, strings.NewReader(""), io.Discard, io.Discard,
				lookupEnv(env), func() []string { return envMap(env) }, server.Client(), "/shim",
				func(string, string) (string, error) { return "/usr/bin/gh", nil }, runner)
			if err != nil {
				t.Fatal(err)
			}
			if !called {
				t.Fatal("real CLI not launched")
			}
		})
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-004.2
func TestEnterpriseCLIShimTargetScope(t *testing.T) {
	for _, tc := range []struct {
		args    []string
		allowed bool
	}{
		{[]string{"api", "user", "--hostname", "foreign.example"}, false},
		{[]string{"api", "https://api.other.ghe.com/user"}, false},
		{[]string{"pr", "view", "https://foreign.example/acme/widgets/pull/1"}, false},
		{[]string{"pr", "list", "--repo", "foreign.example/acme/widgets"}, false},
		{[]string{"api", "repos/other/private"}, false},
		{[]string{"api", "/repos/other/private"}, false},
		{[]string{"--hostname=git.company.example", "api", "repos/other/private"}, false},
		{[]string{"--hostname", "git.company.example", "api", "repos/other/private"}, false},
		{[]string{"pr", "list", "--repo", "acme/widgets", "--repo", "other/private"}, false},
		{[]string{"api", "repos/acme/widgets"}, true},
		{[]string{"api", "/repos/acme/widgets"}, true},
		{[]string{"--hostname", "git.company.example", "api", "repos/acme/widgets"}, true},
		{[]string{"pr", "list", "--repo", "other/private", "--repo", "acme/widgets"}, true},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			redeemed, launched := false, false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				redeemed = true
				_ = json.NewEncoder(w).Encode(map[string]string{"username": "x-access-token", "password": "scoped-token"})
			}))
			t.Cleanup(server.Close)
			env := githubCredentialTestEnv(server.URL)
			env[githubauth.CredentialHostEnv] = "git.company.example"
			env["PATH"] = "/shim:/usr/bin"
			err := runGitHubCLIShim(context.Background(), tc.args, strings.NewReader(""), io.Discard, io.Discard,
				lookupEnv(env), func() []string { return envMap(env) }, server.Client(), "/shim",
				func(string, string) (string, error) { return "/usr/bin/gh", nil },
				func(context.Context, string, []string, []string, io.Reader, io.Writer, io.Writer) error {
					launched = true
					return nil
				})
			if (err == nil) != tc.allowed || redeemed != tc.allowed || launched != tc.allowed {
				t.Fatalf("target allowed=%v: err=%v redeemed=%v launched=%v", tc.allowed, err, redeemed, launched)
			}
		})
	}
}
