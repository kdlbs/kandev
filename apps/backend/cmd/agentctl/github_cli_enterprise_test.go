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
func TestEnterpriseCLIShimRejectsForeignTarget(t *testing.T) {
	for _, args := range [][]string{
		{"api", "user", "--hostname", "foreign.example"},
		{"api", "https://api.other.ghe.com/user"},
		{"pr", "view", "https://foreign.example/acme/repo/pull/1"},
		{"pr", "list", "--repo", "foreign.example/acme/repo"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			redeemed, launched := false, false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				redeemed = true
				_ = json.NewEncoder(w).Encode(map[string]string{"username": "x-access-token", "password": "scoped-token"})
			}))
			t.Cleanup(server.Close)
			env := githubCredentialTestEnv(server.URL)
			env[githubauth.CredentialHostEnv] = "git.company.example"
			env["PATH"] = "/shim:/usr/bin"
			err := runGitHubCLIShim(context.Background(), args, strings.NewReader(""), io.Discard, io.Discard,
				lookupEnv(env), func() []string { return envMap(env) }, server.Client(), "/shim",
				func(string, string) (string, error) { return "/usr/bin/gh", nil },
				func(context.Context, string, []string, []string, io.Reader, io.Writer, io.Writer) error {
					launched = true
					return nil
				})
			if err == nil || redeemed || launched {
				t.Fatalf("foreign target: err=%v redeemed=%v launched=%v", err, redeemed, launched)
			}
		})
	}
}
