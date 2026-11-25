package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGitHubCredentialHelperReportsLeaseRelaunch(t *testing.T) {
	assertLeaseRelaunchGuidance(t, "git")
}

func TestGitHubCLIShimReportsLeaseRelaunch(t *testing.T) {
	assertLeaseRelaunchGuidance(t, "gh")
}

func assertLeaseRelaunchGuidance(t *testing.T, entrypoint string) {
	t.Helper()
	codes := []string{
		"git_credential_lease_invalid",
		"git_credential_lease_expired",
		"github_credential_lease_invalid",
		"github_credential_lease_expired",
	}
	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			var resolveCalls, reissueCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/resolve":
					resolveCalls.Add(1)
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = io.WriteString(w, `{"code":"`+code+`"}`)
				case "/reissue":
					reissueCalls.Add(1)
					w.WriteHeader(http.StatusOK)
					_, _ = io.WriteString(w, `{"token":"unexpected-lease"}`)
				default:
					t.Errorf("unexpected broker endpoint %q", r.URL.Path)
				}
			}))
			t.Cleanup(server.Close)
			env := githubCredentialTestEnv(server.URL + "/resolve")
			var err error
			if entrypoint == "git" {
				err = runGitHubCredentialHelper(
					context.Background(), []string{"get"},
					strings.NewReader("protocol=https\nhost=github.com\npath=acme/widgets.git\n\n"),
					io.Discard, lookupEnv(env), server.Client(),
				)
			} else {
				env["PATH"] = "/shim:/usr/bin"
				err = runGitHubCLIShim(
					context.Background(), []string{"pr", "list"}, strings.NewReader(""), io.Discard, io.Discard,
					lookupEnv(env), func() []string { return envMap(env) }, server.Client(), "/shim",
					func(string, string) (string, error) { return "/usr/bin/gh", nil },
					func(context.Context, string, []string, []string, io.Reader, io.Writer, io.Writer) error {
						t.Error("gh command ran without a broker credential")
						return nil
					},
				)
			}

			if err == nil || !strings.Contains(err.Error(), "Restart session") ||
				!strings.Contains(err.Error(), "HTTP 401") || !strings.Contains(err.Error(), code) {
				t.Fatalf("entrypoint error = %v, want original HTTP 401 and Restart session guidance", err)
			}
			var brokerErr *githubBrokerHTTPError
			if !errors.As(err, &brokerErr) {
				t.Fatalf("error %T does not preserve the typed broker error", err)
			}
			if resolveCalls.Load() != 1 || reissueCalls.Load() != 0 {
				t.Fatalf("resolve/reissue calls = %d/%d, want 1/0", resolveCalls.Load(), reissueCalls.Load())
			}
		})
	}
}

func TestGitHubCredentialHelperKeepsOtherLeaseErrorsUnchanged(t *testing.T) {
	cases := []struct {
		name   string
		code   string
		status int
	}{
		{name: "git revoked", code: "git_credential_lease_revoked", status: http.StatusUnauthorized},
		{name: "github revoked", code: "github_credential_lease_revoked", status: http.StatusUnauthorized},
		{name: "reissue denied", code: "github_credential_reissue_denied", status: http.StatusUnauthorized},
		{name: "expired with non-401 status", code: "git_credential_lease_expired", status: http.StatusForbidden},
	}
	for _, entrypoint := range []string{"git", "gh"} {
		for _, tc := range cases {
			t.Run(entrypoint+"/"+tc.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(tc.status)
					_ = json.NewEncoder(w).Encode(map[string]string{"code": tc.code})
				}))
				t.Cleanup(server.Close)
				env := githubCredentialTestEnv(server.URL)
				var err error
				if entrypoint == "git" {
					err = runGitHubCredentialHelper(
						context.Background(), []string{"get"},
						strings.NewReader("protocol=https\nhost=github.com\npath=acme/widgets.git\n\n"),
						io.Discard, lookupEnv(env), server.Client(),
					)
				} else {
					env["PATH"] = "/shim:/usr/bin"
					err = runGitHubCLIShim(
						context.Background(), []string{"pr", "list"}, strings.NewReader(""), io.Discard, io.Discard,
						lookupEnv(env), func() []string { return envMap(env) }, server.Client(), "/shim",
						func(string, string) (string, error) { return "/usr/bin/gh", nil },
						func(context.Context, string, []string, []string, io.Reader, io.Writer, io.Writer) error {
							t.Error("gh command ran without a broker credential")
							return nil
						},
					)
				}
				if err == nil || !strings.Contains(err.Error(), tc.code) ||
					!strings.Contains(err.Error(), "HTTP "+strconv.Itoa(tc.status)) ||
					strings.Contains(err.Error(), "Restart session") {
					t.Fatalf("lease error = %v, want original HTTP %d diagnostic without relaunch guidance", err, tc.status)
				}
			})
		}
	}
}
