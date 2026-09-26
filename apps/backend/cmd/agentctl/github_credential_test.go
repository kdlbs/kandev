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
	envGitHubCredentialBrokerURL         = githubauth.CredentialBrokerURLEnv
	envGitHubCredentialLease             = githubauth.CredentialLeaseEnv
	envGitHubCredentialReissueCapability = githubauth.CredentialReissueCapabilityEnv
	envGitHubCredentialTaskID            = githubauth.CredentialTaskIDEnv
	envGitHubCredentialSessionID         = githubauth.CredentialSessionIDEnv
	envGitHubCredentialRepository        = githubauth.CredentialRepositoryEnv
	envGitHubCredentialOwner             = githubauth.CredentialOwnerEnv
	envGitHubCredentialRepo              = githubauth.CredentialRepoEnv
	envGitHubCredentialHost              = githubauth.CredentialHostEnv
	envGitHubCredentialScopes            = githubauth.CredentialScopesEnv
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

func TestGitHubCredentialHelperReissuesInvalidLeaseOnce(t *testing.T) {
	var resolveCalls, reissueCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request githubBrokerResolveRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		switch r.URL.Path {
		case "/resolve":
			resolveCalls++
			if resolveCalls == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"code":"github_credential_lease_revoked"}`)
				return
			}
			if request.Lease != "replacement-lease" {
				t.Errorf("replacement resolve lease = %q", request.Lease)
			}
			_, _ = io.WriteString(w, `{"username":"x-access-token","password":"fresh-token"}`)
		case "/reissue":
			reissueCalls++
			if request.Lease != "" {
				t.Errorf("reissue used old lease %q", request.Lease)
			}
			if request.ReissueCapability != "execution-capability" {
				t.Errorf("reissue capability = %q", request.ReissueCapability)
			}
			_, _ = io.WriteString(w, `{"token":"replacement-lease"}`)
		default:
			t.Errorf("unexpected endpoint %q", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	env := githubCredentialTestEnv(server.URL + "/resolve")
	env[envGitHubCredentialReissueCapability] = "execution-capability"

	err := runGitHubCredentialHelper(
		context.Background(), []string{"get"},
		strings.NewReader("protocol=https\nhost=github.com\npath=acme/widgets.git\n\n"),
		io.Discard, lookupEnv(env), server.Client(),
	)
	if err != nil {
		t.Fatalf("runGitHubCredentialHelper() error = %v", err)
	}
	if resolveCalls != 2 || reissueCalls != 1 {
		t.Fatalf("resolve/reissue calls = %d/%d, want 2/1", resolveCalls, reissueCalls)
	}
}

func TestGitHubCredentialHelperReissuesLeaseInMultiScopeMode(t *testing.T) {
	var resolveCalls, reissueCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request githubBrokerResolveRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		switch r.URL.Path {
		case "/resolve":
			resolveCalls++
			if resolveCalls == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"code":"github_credential_lease_revoked"}`)
				return
			}
			if request.Lease != "replacement-lease" {
				t.Errorf("replacement resolve lease = %q", request.Lease)
			}
			_, _ = io.WriteString(w, `{"username":"x-access-token","password":"fresh-token"}`)
		case "/reissue":
			reissueCalls++
			if request.Lease != "" || request.ReissueCapability != "execution-capability" {
				t.Errorf("reissue request = %+v", request)
			}
			_, _ = io.WriteString(w, `{"token":"replacement-lease"}`)
		default:
			t.Errorf("unexpected endpoint %q", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	env := githubCredentialTestEnv(server.URL + "/resolve")
	env[envGitHubCredentialScopes] = `[
		{"lease":"original-lease","reissue_capability":"execution-capability","task_id":"task-1","session_id":"session-1","repository_id":"repo-1","owner":"acme","repo":"widgets","host":"github.com"}
	]`

	err := runGitHubCredentialHelper(
		context.Background(), []string{"get"},
		strings.NewReader("protocol=https\nhost=github.com\npath=acme/widgets.git\n\n"),
		io.Discard, lookupEnv(env), server.Client(),
	)
	if err != nil {
		t.Fatalf("runGitHubCredentialHelper() error = %v", err)
	}
	if resolveCalls != 2 || reissueCalls != 1 {
		t.Fatalf("resolve/reissue calls = %d/%d, want 2/1", resolveCalls, reissueCalls)
	}
}

func TestGitHubCredentialHelperStopsWhenReissueFails(t *testing.T) {
	resolveCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/resolve":
			resolveCalls++
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"code":"github_credential_lease_invalid"}`)
		case "/reissue":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"code":"github_credential_reissue_denied"}`)
		default:
			t.Errorf("unexpected endpoint %q", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	env := githubCredentialTestEnv(server.URL + "/resolve")
	env[envGitHubCredentialReissueCapability] = "execution-capability"

	err := runGitHubCredentialHelper(
		context.Background(), []string{"get"},
		strings.NewReader("protocol=https\nhost=github.com\npath=acme/widgets.git\n\n"),
		io.Discard, lookupEnv(env), server.Client(),
	)
	if err == nil {
		t.Fatal("runGitHubCredentialHelper() error = nil, want reissue failure")
	}
	if resolveCalls != 1 {
		t.Fatalf("resolve calls = %d, want exactly 1 after reissue failure", resolveCalls)
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

func TestGitHubCredentialHelperRejectsRepositoryPathCaseMismatch(t *testing.T) {
	env := githubCredentialTestEnv("https://broker.example/resolve")
	env[envGitHubCredentialOwner] = "Team"
	env[envGitHubCredentialRepo] = "Repo"
	err := runGitHubCredentialHelper(
		context.Background(),
		[]string{"get"},
		strings.NewReader("protocol=https\nhost=github.com\npath=team/repo.git\n\n"),
		io.Discard,
		lookupEnv(env),
		http.DefaultClient,
	)
	if err == nil || !strings.Contains(err.Error(), "does not match credential lease scope") {
		t.Fatalf("runGitHubCredentialHelper() error = %v, want case-sensitive scope mismatch", err)
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

func TestGitHubCredentialHelperPreservesExactProviderPath(t *testing.T) {
	var got githubBrokerResolveRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{"username":"x-token-auth","password":"fresh-token"}`)
	}))
	t.Cleanup(server.Close)
	env := githubCredentialTestEnv(server.URL)
	env[envGitHubCredentialScopes] = `[
		{"lease":"opaque-lease","task_id":"task-1","session_id":"session-1","repository_id":"repo-1","owner":"context","repo":"scm/ENG/widgets","host":"bitbucket.example","path":"/context/scm/ENG/widgets"}
	]`

	err := runGitHubCredentialHelper(
		context.Background(), []string{"get"},
		strings.NewReader("protocol=https\nhost=bitbucket.example\npath=context/scm/ENG/widgets\n\n"),
		io.Discard, lookupEnv(env), server.Client(),
	)
	if err != nil {
		t.Fatalf("runGitHubCredentialHelper() error = %v", err)
	}
	if got.Path != "/context/scm/ENG/widgets" {
		t.Fatalf("broker path = %q, want exact provider path", got.Path)
	}
}

// git sends the remote path verbatim, so it may or may not carry the ".git"
// suffix the scope path was derived from. Neither spelling may decide whether
// the lease resolves.
func TestGitHubCredentialHelperMatchesScopePathAcrossGitSuffix(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		scopePath string
		inputPath string
	}{
		{name: "scope keeps suffix", scopePath: "/acme/widgets.git", inputPath: "acme/widgets"},
		{name: "request keeps suffix", scopePath: "/acme/widgets", inputPath: "acme/widgets.git"},
		{name: "both keep suffix", scopePath: "/acme/widgets.git", inputPath: "acme/widgets.git"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
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
				{"lease":"scoped-lease","task_id":"task-1","session_id":"session-1","repository_id":"repo-1","owner":"acme","repo":"widgets","host":"github.com","path":"` +
				testCase.scopePath + `"}
			]`

			err := runGitHubCredentialHelper(
				context.Background(), []string{"get"},
				strings.NewReader("protocol=https\nhost=github.com\npath="+testCase.inputPath+"\n\n"),
				io.Discard, lookupEnv(env), server.Client(),
			)
			if err != nil {
				t.Fatalf("runGitHubCredentialHelper() error = %v", err)
			}
			if got.Lease != "scoped-lease" {
				t.Fatalf("selected broker scope = %+v", got)
			}
		})
	}
}

func TestGitHubCredentialHelperRejectsDifferentScopedRepository(t *testing.T) {
	env := githubCredentialTestEnv("https://broker.example/resolve")
	env[envGitHubCredentialScopes] = `[
		{"lease":"scoped-lease","task_id":"task-1","session_id":"session-1","repository_id":"repo-1","owner":"acme","repo":"widgets","host":"github.com","path":"/acme/widgets.git"}
	]`

	err := runGitHubCredentialHelper(
		context.Background(), []string{"get"},
		strings.NewReader("protocol=https\nhost=github.com\npath=acme/widgets-staging\n\n"),
		io.Discard, lookupEnv(env), http.DefaultClient,
	)
	if err == nil || !strings.Contains(err.Error(), "does not match any credential lease scope") {
		t.Fatalf("runGitHubCredentialHelper() error = %v, want scope mismatch", err)
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

func TestGitHubCredentialHelperSurfacesBrokerErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"authentication required"}`)
	}))
	t.Cleanup(server.Close)
	env := githubCredentialTestEnv(server.URL)

	err := runGitHubCredentialHelper(
		context.Background(), []string{"get"},
		strings.NewReader("protocol=https\nhost=github.com\npath=acme/widgets.git\n\n"),
		io.Discard, lookupEnv(env), server.Client(),
	)
	if err == nil {
		t.Fatal("runGitHubCredentialHelper() error = nil, want broker denial")
	}
	want := `resolve GitHub credential: broker returned HTTP 401: {"error":"authentication required"}`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestGitHubCredentialHelperTruncatesAndSanitizesBrokerErrorBody(t *testing.T) {
	rawBody := "prefix\x00\x01\x1f control chars" + strings.Repeat("x", 600)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, rawBody)
	}))
	t.Cleanup(server.Close)
	env := githubCredentialTestEnv(server.URL)

	err := runGitHubCredentialHelper(
		context.Background(), []string{"get"},
		strings.NewReader("protocol=https\nhost=github.com\npath=acme/widgets.git\n\n"),
		io.Discard, lookupEnv(env), server.Client(),
	)
	if err == nil {
		t.Fatal("runGitHubCredentialHelper() error = nil, want broker denial")
	}
	if !strings.HasPrefix(err.Error(), "resolve GitHub credential: broker returned HTTP 500: ") {
		t.Fatalf("error = %q, want HTTP 500 prefix", err.Error())
	}
	appended := strings.TrimPrefix(err.Error(), "resolve GitHub credential: broker returned HTTP 500: ")
	if len(appended) > 512 {
		t.Fatalf("appended body length = %d, want <= 512", len(appended))
	}
	for _, r := range appended {
		if r < 0x20 {
			t.Fatalf("appended body contains control char %q: %q", r, appended)
		}
	}
}

func TestGitHubCredentialHelperOmitsAppendedBodyWhenEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)
	env := githubCredentialTestEnv(server.URL)

	err := runGitHubCredentialHelper(
		context.Background(), []string{"get"},
		strings.NewReader("protocol=https\nhost=github.com\npath=acme/widgets.git\n\n"),
		io.Discard, lookupEnv(env), server.Client(),
	)
	want := "resolve GitHub credential: broker returned HTTP 502"
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
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
