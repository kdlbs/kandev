package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/githubauth"
)

const (
	envGitHubCredentialBrokerURL  = githubauth.CredentialBrokerURLEnv
	envGitHubCredentialLease      = githubauth.CredentialLeaseEnv
	envGitHubCredentialTaskID     = githubauth.CredentialTaskIDEnv
	envGitHubCredentialSessionID  = githubauth.CredentialSessionIDEnv
	envGitHubCredentialRepository = githubauth.CredentialRepositoryEnv
	envGitHubCredentialOwner      = githubauth.CredentialOwnerEnv
	envGitHubCredentialRepo       = githubauth.CredentialRepoEnv
	envGitHubCredentialHost       = githubauth.CredentialHostEnv
	envGitHubCredentialScopes     = githubauth.CredentialScopesEnv
)

func TestGitHubCredentialHelperGetsFreshCredential(t *testing.T) {
	var got githubBrokerResolveRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"username":"x-access-token","password":"fresh-token"}`)
	}))
	t.Cleanup(server.Close)
	env := githubCredentialTestEnv(server.URL)
	var stdout bytes.Buffer

	err := runGitHubCredentialHelper(
		context.Background(),
		[]string{"get"},
		strings.NewReader("protocol=https\nhost=github.com\npath=acme/widgets.git\n\n"),
		&stdout,
		lookupEnv(env),
		server.Client(),
	)
	if err != nil {
		t.Fatalf("runGitHubCredentialHelper() error = %v", err)
	}
	if got.Lease != "opaque-lease" || got.TaskID != "task-1" || got.SessionID != "session-1" {
		t.Fatalf("broker scope = %+v", got)
	}
	if got.RepositoryID != "repo-1" || got.Owner != "acme" || got.Repo != "widgets" || got.Host != "github.com" {
		t.Fatalf("broker repository scope = %+v", got)
	}
	if want := "username=x-access-token\npassword=fresh-token\n\n"; stdout.String() != want {
		t.Fatalf("credential helper output = %q, want %q", stdout.String(), want)
	}
}

func TestGitHubCredentialHelperRejectsRepositoryScopeMismatch(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	env := githubCredentialTestEnv(server.URL)

	err := runGitHubCredentialHelper(
		context.Background(),
		[]string{"get"},
		strings.NewReader("protocol=https\nhost=github.com\npath=other/repository.git\n\n"),
		io.Discard,
		lookupEnv(env),
		server.Client(),
	)
	if err == nil || !strings.Contains(err.Error(), "does not match credential lease scope") {
		t.Fatalf("runGitHubCredentialHelper() error = %v, want scope mismatch", err)
	}
	if requests != 0 {
		t.Fatalf("broker requests = %d, want 0", requests)
	}
}

func TestGitHubCredentialHelperRequiresCompleteScope(t *testing.T) {
	for name, input := range map[string]string{
		"protocol": "host=github.com\npath=acme/widgets.git\n\n",
		"host":     "protocol=https\npath=acme/widgets.git\n\n",
		"path":     "protocol=https\nhost=github.com\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			env := githubCredentialTestEnv("https://broker.example/resolve")
			err := runGitHubCredentialHelper(
				context.Background(), []string{"get"}, strings.NewReader(input), io.Discard,
				lookupEnv(env), http.DefaultClient,
			)
			if err == nil {
				t.Fatal("runGitHubCredentialHelper() error = nil, want incomplete scope rejection")
			}
		})
	}
}

func TestGitHubCredentialHelperSelectsRepositoryLease(t *testing.T) {
	var got githubBrokerResolveRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"username":"x-access-token","password":"fresh-token"}`)
	}))
	t.Cleanup(server.Close)
	env := githubCredentialTestEnv(server.URL)
	env[envGitHubCredentialScopes] = `[
		{"lease":"frontend-lease","task_id":"task-1","session_id":"session-1","repository_id":"repo-1","owner":"acme","repo":"frontend","host":"github.com"},
		{"lease":"backend-lease","task_id":"task-1","session_id":"session-1","repository_id":"repo-2","owner":"acme","repo":"backend","host":"github.com"}
	]`

	err := runGitHubCredentialHelper(
		context.Background(), []string{"get"},
		strings.NewReader("protocol=https\nhost=github.com\npath=acme/backend.git\n\n"),
		io.Discard, lookupEnv(env), server.Client(),
	)
	if err != nil {
		t.Fatalf("runGitHubCredentialHelper() error = %v", err)
	}
	if got.Lease != "backend-lease" || got.RepositoryID != "repo-2" {
		t.Fatalf("selected broker scope = %+v", got)
	}
}

func TestGitHubCredentialHelperIgnoresStoreAndErase(t *testing.T) {
	for _, operation := range []string{"store", "erase"} {
		t.Run(operation, func(t *testing.T) {
			if err := runGitHubCredentialHelper(
				context.Background(), []string{operation}, strings.NewReader("password=secret\n\n"),
				io.Discard, lookupEnv(nil), http.DefaultClient,
			); err != nil {
				t.Fatalf("runGitHubCredentialHelper(%q) error = %v", operation, err)
			}
		})
	}
}

func githubCredentialTestEnv(url string) map[string]string {
	return map[string]string{
		envGitHubCredentialBrokerURL:  url,
		envGitHubCredentialLease:      "opaque-lease",
		envGitHubCredentialTaskID:     "task-1",
		envGitHubCredentialSessionID:  "session-1",
		envGitHubCredentialRepository: "repo-1",
		envGitHubCredentialOwner:      "acme",
		envGitHubCredentialRepo:       "widgets",
		envGitHubCredentialHost:       "github.com",
	}
}

func lookupEnv(env map[string]string) func(string) string {
	return func(key string) string { return env[key] }
}
