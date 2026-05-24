package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/worktree/copyfiles"
)

func TestEmitCopyFilesStep_Pluralization(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		copied   []string
		expected string
	}{
		{"none", nil, "Copy ignored files"},
		{"one", []string{".env"}, "Copy 1 ignored file"},
		{"two", []string{".env", "config.yml"}, "Copy 2 ignored files"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got PrepareStep
			cb := func(step PrepareStep, _, _ int) { got = step }
			cb(buildCopyFilesStep(tc.copied, nil, ""), 0, 1)
			if got.Name != tc.expected {
				t.Errorf("name = %q, want %q", got.Name, tc.expected)
			}
			if got.Status != PrepareStepCompleted {
				t.Errorf("status = %q, want completed", got.Status)
			}
		})
	}
}

func TestBuildCopyFilesStep_RepoLabelAppended(t *testing.T) {
	t.Parallel()
	got := buildCopyFilesStep([]string{".env"}, nil, "frontend")
	if got.Name != "Copy 1 ignored file (frontend)" {
		t.Errorf("name = %q, want %q", got.Name, "Copy 1 ignored file (frontend)")
	}
}

func TestBuildCopyFilesStep_WarningsAttached(t *testing.T) {
	t.Parallel()
	got := buildCopyFilesStep([]string{".env"},
		[]string{"no matches for pattern \".local\"", "skipped \"big.bin\""}, "")
	if got.Warning == "" {
		t.Fatal("expected warning")
	}
	if !strings.Contains(got.WarningDetail, "big.bin") {
		t.Errorf("warningDetail = %q, expected joined warnings", got.WarningDetail)
	}
}

// stubCopyFilesServer spins up an httptest server speaking the agentctl
// POST /workspace/copy-files contract. Returns the URL the agentctl
// client should target.
func stubCopyFilesServer(t *testing.T, respond func(req client.CopyFilesRequest) client.CopyFilesResponse) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workspace/copy-files" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var req client.CopyFilesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp := respond(req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestRunRemoteCopyfiles_HappyPath_EmitsStep(t *testing.T) {
	// Source repo with files matching the spec.
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, ".env"), []byte("X=1"), 0o600); err != nil {
		t.Fatalf("write src .env: %v", err)
	}

	// Stub agentctl that mirrors the entries back as "copied" — verifies
	// the helper actually shipped bytes rather than no-opping.
	var received client.CopyFilesRequest
	url := stubCopyFilesServer(t, func(req client.CopyFilesRequest) client.CopyFilesResponse {
		received = req
		out := make([]string, len(req.Entries))
		for i, e := range req.Entries {
			out[i] = e.RelPath
		}
		return client.CopyFilesResponse{Copied: out}
	})

	cli := clientForStub(t, url)
	var got PrepareStep
	cb := func(step PrepareStep, _, _ int) { got = step }

	runRemoteCopyfiles(context.Background(), logger.Default(), remoteCopyfilesRequest{
		SourceRepoPath: src,
		CopyFilesSpec:  ".env",
		Client:         cli,
		OnProgress:     cb,
	})

	if len(received.Entries) != 1 || received.Entries[0].RelPath != ".env" {
		t.Fatalf("received entries = %+v, expected [.env]", received.Entries)
	}
	if string(received.Entries[0].Content) != "X=1" {
		t.Errorf("payload content = %q, want X=1", received.Entries[0].Content)
	}
	if got.Name != "Copy 1 ignored file" {
		t.Errorf("step name = %q, want %q", got.Name, "Copy 1 ignored file")
	}
	if got.Output != ".env" {
		t.Errorf("step output = %q, want %q", got.Output, ".env")
	}
}

func TestRunRemoteCopyfiles_NoSpec_NoOp(t *testing.T) {
	emitted := false
	cb := func(_ PrepareStep, _, _ int) { emitted = true }
	runRemoteCopyfiles(context.Background(), logger.Default(), remoteCopyfilesRequest{
		SourceRepoPath: t.TempDir(),
		CopyFilesSpec:  "",
		OnProgress:     cb,
	})
	if emitted {
		t.Error("expected no step when CopyFilesSpec is empty")
	}
}

func TestRunRemoteCopyfiles_ShipFailure_StillEmitsWarning(t *testing.T) {
	src := t.TempDir()
	_ = os.WriteFile(filepath.Join(src, ".env"), []byte("X=1"), 0o600)

	// Agentctl returns an HTTP 500 with an error payload.
	url := stubCopyFilesServerErr(t)
	cli := clientForStub(t, url)
	var got PrepareStep
	cb := func(step PrepareStep, _, _ int) { got = step }

	runRemoteCopyfiles(context.Background(), logger.Default(), remoteCopyfilesRequest{
		SourceRepoPath: src,
		CopyFilesSpec:  ".env",
		Client:         cli,
		OnProgress:     cb,
	})

	if got.Name == "" {
		t.Fatal("expected step to be emitted on ship failure")
	}
	if got.Warning == "" {
		t.Error("expected warning on ship failure")
	}
}

func TestShipRemoteCopyfilesForLaunch_SingleRepoUsesTopLevelSpec(t *testing.T) {
	src := t.TempDir()
	_ = os.WriteFile(filepath.Join(src, ".env"), []byte("X=1"), 0o600)

	var received []client.CopyFilesRequest
	url := stubCopyFilesServer(t, func(req client.CopyFilesRequest) client.CopyFilesResponse {
		received = append(received, req)
		out := make([]string, len(req.Entries))
		for i, e := range req.Entries {
			out[i] = e.RelPath
		}
		return client.CopyFilesResponse{Copied: out}
	})

	rec := newPrepareProgressRecorder(nil)
	cli := clientForStub(t, url)
	shipRemoteCopyfilesForLaunch(context.Background(), logger.Default(),
		&LaunchRequest{RepositoryPath: src, CopyFiles: ".env"}, cli,
		rec.Callback(0), rec)

	if len(received) != 1 {
		t.Fatalf("expected 1 ship call, got %d", len(received))
	}
	if received[0].Repo != "" {
		t.Errorf("single-repo Repo subpath = %q, want empty", received[0].Repo)
	}
	if len(received[0].Entries) != 1 || received[0].Entries[0].RelPath != ".env" {
		t.Errorf("entries = %+v, expected [.env]", received[0].Entries)
	}
	steps := rec.Steps()
	if len(steps) != 1 || steps[0].Name != "Copy 1 ignored file" {
		t.Errorf("steps = %+v, want one 'Copy 1 ignored file'", steps)
	}
}

func TestShipRemoteCopyfilesForLaunch_MultiRepoSkipsEmptySpecs(t *testing.T) {
	srcA := t.TempDir()
	srcB := t.TempDir()
	_ = os.WriteFile(filepath.Join(srcA, ".env.a"), []byte("A"), 0o600)
	_ = os.WriteFile(filepath.Join(srcB, ".env.b"), []byte("B"), 0o600)

	var received []client.CopyFilesRequest
	url := stubCopyFilesServer(t, func(req client.CopyFilesRequest) client.CopyFilesResponse {
		received = append(received, req)
		out := make([]string, len(req.Entries))
		for i, e := range req.Entries {
			out[i] = e.RelPath
		}
		return client.CopyFilesResponse{Copied: out}
	})

	rec := newPrepareProgressRecorder(nil)
	cli := clientForStub(t, url)
	shipRemoteCopyfilesForLaunch(context.Background(), logger.Default(),
		&LaunchRequest{
			// Top-level fields should be ignored when Repositories is set —
			// the multi-repo loop is the source of truth.
			RepositoryPath: "/should/not/be/used",
			CopyFiles:      "ignored",
			Repositories: []RepoLaunchSpec{
				{RepositoryPath: srcA, RepoName: "alpha", CopyFiles: ".env.a"},
				{RepositoryPath: srcB, RepoName: "beta", CopyFiles: ""}, // empty spec → skip
				{RepositoryPath: srcB, RepoName: "gamma", CopyFiles: ".env.b"},
			},
		}, cli, rec.Callback(0), rec)

	if len(received) != 2 {
		t.Fatalf("expected 2 ship calls (alpha + gamma), got %d", len(received))
	}
	if received[0].Repo != "alpha" || received[1].Repo != "gamma" {
		t.Errorf("subpaths = [%q, %q], want [alpha, gamma]", received[0].Repo, received[1].Repo)
	}

	// Each per-repo job should land in its own recorder slot — verifies the
	// per-job Callback(progressRecorder.Len()) wiring (the original
	// double-offset bug emitted both steps at the same index, clobbering one).
	steps := rec.Steps()
	if len(steps) != 2 {
		t.Fatalf("expected 2 distinct steps, got %d (steps=%+v)", len(steps), steps)
	}
	if steps[0].Name != "Copy 1 ignored file (alpha)" || steps[1].Name != "Copy 1 ignored file (gamma)" {
		t.Errorf("step names = [%q, %q], want [Copy 1 ignored file (alpha), Copy 1 ignored file (gamma)]",
			steps[0].Name, steps[1].Name)
	}
}

func TestShipRemoteCopyfilesForLaunch_NoSpec_NoOp(t *testing.T) {
	rec := newPrepareProgressRecorder(nil)
	shipRemoteCopyfilesForLaunch(context.Background(), logger.Default(),
		&LaunchRequest{RepositoryPath: t.TempDir(), CopyFiles: ""}, nil,
		rec.Callback(0), rec)
	if got := len(rec.Steps()); got != 0 {
		t.Errorf("expected zero steps for empty spec, got %d", got)
	}
}

func stubCopyFilesServerErr(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(client.CopyFilesResponse{Error: "disk full"})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// clientForStub builds an agentctl Client pointed at a test URL. Parses
// host:port from the httptest server URL.
func clientForStub(t *testing.T, baseURL string) *client.Client {
	t.Helper()
	// Strip the http:// scheme; client.NewClient takes host+port.
	stripped := strings.TrimPrefix(baseURL, "http://")
	host, port, ok := strings.Cut(stripped, ":")
	if !ok {
		t.Fatalf("malformed test server URL: %s", baseURL)
	}
	var p int
	for _, ch := range port {
		p = p*10 + int(ch-'0')
	}
	// Verify we can satisfy the package interface — keep imports honest.
	_ = copyfiles.MaxEntryBytes
	return client.NewClient(host, p, logger.Default())
}
