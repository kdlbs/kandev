package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
)

func apiSaveTargetRequest(t *testing.T, server *Server, ctx context.Context, body streams.FileUpdateRequest) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/file/content", bytes.NewReader(payload)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)
	return rec
}

func apiSaveTargetOriginal(scope string) string {
	return "draft\ncontext one\ncontext two\ncontext three\nowner=" + scope + "\n"
}

func runAPISaveTargetPath(t *testing.T, path string, symlink bool) {
	t.Helper()
	server, dir, _ := newAPISaveTargetFixture(t, path, true)
	if symlink {
		link := filepath.Join(dir, "beta", path)
		writeFileAPI(t, dir, "beta/target.txt", apiSaveTargetOriginal("beta"))
		if err := os.Remove(link); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("target.txt", link); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("native symlink creation unavailable: %v", err)
			}
			t.Fatal(err)
		}
	}
	original := apiSaveTargetOriginal("beta")
	desired := strings.Replace(original, "draft", "saved", 1)
	req := streams.FileUpdateRequest{Repo: "beta", Path: path, Diff: apiSaveTargetPatch(path, true),
		OriginalHash: fmt.Sprintf("%x", sha256.Sum256([]byte(original)))}
	rec := workspaceRequest(t, server, http.MethodPost, "/api/v1/workspace/file/content", req)
	got := decodeWorkspaceBody[streams.FileUpdateResponse](t, rec)
	if rec.Code != http.StatusOK || !got.Success || got.Path != path || got.Error != "" || got.Resolution != "applied" || got.NewHash != fmt.Sprintf("%x", sha256.Sum256([]byte(desired))) {
		t.Errorf("save ACK = status%d %+v; want applied desired hash", rec.Code, got)
	}
	assertAPISaveTargetDisk(t, dir, path, "beta", desired)
	if symlink {
		assertFileUpdateSaveBytes(t, dir, "beta/target.txt", desired)
		info, err := os.Lstat(filepath.Join(dir, "beta", path))
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("selected symlink was replaced: %v", err)
		}
	}
}

func runAPISaveTargetHunk(t *testing.T) {
	t.Helper()
	server, dir, _ := newAPISaveTargetFixture(t, "one.txt", false)
	for _, scope := range []string{"", "alpha", "beta"} {
		writeFileAPI(t, dir, filepath.Join(scope, "one.txt"), "-- one.txt\nkeep\nowner="+scope+"\n")
	}
	req := streams.FileUpdateRequest{Repo: "beta", Path: "one.txt",
		Diff: "--- one.txt\t\n+++ one.txt\t\n@@ -1,2 +1,2 @@\n--- one.txt\n+++ one.txt\n keep\n"}
	desired := "++ one.txt\nkeep\nowner=beta\n"
	rec := workspaceRequest(t, server, http.MethodPost, "/api/v1/workspace/file/content", req)
	got := decodeWorkspaceBody[streams.FileUpdateResponse](t, rec)
	if rec.Code != http.StatusOK || !got.Success || got.Path != req.Path || got.Resolution != "applied" || got.NewHash != fmt.Sprintf("%x", sha256.Sum256([]byte(desired))) {
		t.Errorf("header-like hunk ACK = status%d %+v", rec.Code, got)
	}
	assertFileUpdateSaveBytes(t, dir, "beta/one.txt", desired)
	for _, scope := range []string{"", "alpha"} {
		assertFileUpdateSaveBytes(t, dir, filepath.Join(scope, "one.txt"), "-- one.txt\nkeep\nowner="+scope+"\n")
	}
}

// @covers AC-WORKSPACES-SAVED-FILE-CONTENT-001.2
func TestHandleFileUpdate_RepositoryTargetCompatibility(t *testing.T) {
	t.Run("nested", func(t *testing.T) { runAPISaveTargetPath(t, "src/one.txt", false) })
	t.Run("literal_a_directory", func(t *testing.T) { runAPISaveTargetPath(t, "a/one.txt", false) })
	t.Run("symlink", func(t *testing.T) { runAPISaveTargetPath(t, "one.txt", true) })
	t.Run("header_like_hunk", runAPISaveTargetHunk)
	t.Run("fallbacks", runAPISaveTargetFallbacks)
	t.Run("cancelled", runAPISaveTargetCancellation)
	t.Run("invalid_scope", runAPISaveTargetInvalidScope)
}

func runAPISaveTargetFallbacks(t *testing.T) {
	t.Helper()
	for _, conflict := range []bool{false, true} {
		for _, content := range []string{"nil", "empty", "saved"} {
			t.Run(fmt.Sprintf("conflict=%t/content=%s", conflict, content), func(t *testing.T) {
				server, dir, _ := newAPISaveTargetFixture(t, "one.txt", false)
				original := apiSaveTargetOriginal("beta")
				req := streams.FileUpdateRequest{Repo: "beta", Path: "one.txt", Diff: "not a patch\n",
					OriginalHash: fmt.Sprintf("%x", sha256.Sum256([]byte(original)))}
				if conflict {
					req.OriginalHash, req.Diff = "stale hash", apiSaveTargetPatch("one.txt", true)
				}
				desired := ""
				if content != "nil" {
					req.DesiredContent = &desired
				}
				if content == "saved" {
					desired = strings.Replace(original, "draft", "saved", 1)
				}
				rec := workspaceRequest(t, server, http.MethodPost, "/api/v1/workspace/file/content", req)
				got := decodeWorkspaceBody[streams.FileUpdateResponse](t, rec)
				if req.DesiredContent == nil {
					desired = original
					if rec.Code != http.StatusBadRequest || got.Success || got.Path != req.Path || got.Error == "" || got.NewHash != "" || got.Resolution != "" {
						t.Errorf("rejected ACK = status%d %+v", rec.Code, got)
					}
				} else if rec.Code != http.StatusOK || !got.Success || got.Path != req.Path || got.Error != "" || got.Resolution != "overwritten" || got.NewHash != fmt.Sprintf("%x", sha256.Sum256([]byte(desired))) {
					t.Errorf("overwrite ACK = status%d %+v", rec.Code, got)
				}
				assertAPISaveTargetDisk(t, dir, "one.txt", "beta", desired)
			})
		}
	}
}

func runAPISaveTargetInvalidScope(t *testing.T) {
	t.Helper()
	for _, req := range []streams.FileUpdateRequest{
		{Repo: "missing", Path: "one.txt", Diff: apiSaveTargetPatch("one.txt", true)},
		{Repo: "beta", Path: "../../one.txt", Diff: apiSaveTargetPatch("one.txt", true)},
	} {
		server, dir, _ := newAPISaveTargetFixture(t, "one.txt", false)
		rec := workspaceRequest(t, server, http.MethodPost, "/api/v1/workspace/file/content", req)
		got := decodeWorkspaceBody[streams.FileUpdateResponse](t, rec)
		if rec.Code != http.StatusBadRequest || got.Success || got.Error == "" || got.NewHash != "" || got.Resolution != "" || got.Path != req.Path {
			t.Errorf("invalid scope ACK = status%d %+v", rec.Code, got)
		}
		assertAPISaveTargetDisk(t, dir, "one.txt", "beta", apiSaveTargetOriginal("beta"))
	}
}

func runAPISaveTargetCancellation(t *testing.T) {
	t.Helper()
	server, dir, _ := newAPISaveTargetFixture(t, "one.txt", false)
	ctx, release, pending := holdFileUpdateAdmission(t)
	cancelledCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	start := func(scope string, requestCtx context.Context) <-chan *httptest.ResponseRecorder {
		result := make(chan *httptest.ResponseRecorder, 1)
		pending.Add(1)
		go func() {
			defer pending.Done()
			original := apiSaveTargetOriginal(scope)
			desired := strings.Replace(original, "draft", "saved", 1)
			req := streams.FileUpdateRequest{Repo: scope, Path: "one.txt", Diff: apiSaveTargetPatch("one.txt", true),
				OriginalHash: fmt.Sprintf("%x", sha256.Sum256([]byte(original))), DesiredContent: &desired}
			result <- apiSaveTargetRequest(t, server, requestCtx, req)
		}()
		return result
	}
	beta := start("beta", cancelledCtx)
	awaitQueuedFileUpdates(t, ctx, 1)
	alpha := start("alpha", ctx)
	awaitQueuedFileUpdates(t, ctx, 2)
	cancel()
	b := awaitFileUpdateSave(t, ctx, beta)
	got := decodeWorkspaceBody[streams.FileUpdateResponse](t, b)
	if b.Code != http.StatusBadRequest || got.Success || got.Path != "one.txt" || !strings.Contains(got.Error, "cancel") || got.NewHash != "" || got.Resolution != "" {
		t.Errorf("cancelled ACK = status%d %+v", b.Code, got)
	}
	release()
	a := awaitFileUpdateSave(t, ctx, alpha)
	pending.Wait()
	peer := decodeWorkspaceBody[streams.FileUpdateResponse](t, a)
	desired := strings.Replace(apiSaveTargetOriginal("alpha"), "draft", "saved", 1)
	if a.Code != http.StatusOK || !peer.Success || peer.Path != "one.txt" || peer.Resolution != "applied" || peer.NewHash != fmt.Sprintf("%x", sha256.Sum256([]byte(desired))) {
		t.Errorf("peer ACK = status%d %+v", a.Code, peer)
	}
	assertAPISaveTargetDisk(t, dir, "one.txt", "alpha", desired)
	patches, err := filepath.Glob(filepath.Join(dir, ".kandev-patch-*"))
	if err != nil || len(patches) != 0 {
		t.Errorf("patch cleanup = %v, %v", patches, err)
	}
}

func apiSaveTargetPatch(path string, editor bool) string {
	header := "--- " + path + "\n+++ " + path + "\n"
	if editor {
		header = "Index: " + path + "\n===================================================================\n--- " + path + "\t\n+++ " + path + "\t\n"
	}
	return header + "@@ -1,4 +1,4 @@\n-draft\n+saved\n context one\n context two\n context three\n"
}

func newAPISaveTargetFixture(t *testing.T, path string, initialized bool) (*Server, string, *process.WorkspaceTracker) {
	t.Helper()
	gitConfig := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(gitConfig, []byte("[core]\n\tautocrlf = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", gitConfig)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"", "alpha", "beta"} {
		repo := filepath.Join(dir, scope)
		if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, path)), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFileAPI(t, repo, path, apiSaveTargetOriginal(scope))
		if initialized && scope != "" {
			runGitAPI(t, repo, "init", "--initial-branch=main")
		}
	}
	writeFileAPI(t, dir, "neighbor.txt", "unchanged neighbor\n")
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.InstanceConfig{WorkDir: dir, AgentEnv: os.Environ()}
	manager := process.NewManager(cfg, log)
	tracker := manager.GetWorkspaceTracker()
	t.Cleanup(tracker.Stop)
	return NewServer(cfg, manager, nil, nil, log), dir, tracker
}

func assertAPISaveTargetDisk(t *testing.T, dir, path, selected, desired string) {
	t.Helper()
	for _, scope := range []string{"", "alpha", "beta"} {
		want := apiSaveTargetOriginal(scope)
		if scope == selected {
			want = desired
		}
		assertFileUpdateSaveBytes(t, dir, filepath.Join(scope, path), want)
	}
	assertFileUpdateSaveBytes(t, dir, "neighbor.txt", "unchanged neighbor\n")
}

func assertAPISaveTargetEvent(t *testing.T, sub types.WorkspaceStreamSubscriber, path string) {
	t.Helper()
	select {
	case msg := <-sub:
		got := msg.FileChange
		if got == nil || filepath.ToSlash(got.Path) != filepath.ToSlash(path) || got.Operation != types.FileOpWrite || got.RepositoryName != "" {
			t.Errorf("write event fields = %+v; envelope = %+v; want root tracker write for %s", got, msg, path)
		}
	default:
		t.Error("save returned without the immediate write event")
	}
}

// @covers AC-WORKSPACES-SAVED-FILE-CONTENT-001.2
func TestHandleFileUpdate_RepositoryTarget(t *testing.T) {
	for _, initialized := range []bool{false, true} {
		for _, editor := range []bool{false, true} {
			for _, fallback := range []bool{false, true} {
				for _, scope := range []string{"", "alpha", "beta"} {
					name := fmt.Sprintf("git=%t/editor=%t/fallback=%t/repo=%s", initialized, editor, fallback, scope)
					t.Run(name, func(t *testing.T) {
						server, dir, tracker := newAPISaveTargetFixture(t, "one.txt", initialized)
						original := apiSaveTargetOriginal(scope)
						desired := strings.Replace(original, "draft", "saved", 1)
						req := streams.FileUpdateRequest{Repo: scope, Path: "one.txt", Diff: apiSaveTargetPatch("one.txt", editor),
							OriginalHash: fmt.Sprintf("%x", sha256.Sum256([]byte(original)))}
						if fallback {
							req.DesiredContent = &desired
						}
						sub := tracker.SubscribeWorkspaceStream()
						t.Cleanup(func() { tracker.UnsubscribeWorkspaceStream(sub) })
						rec := workspaceRequest(t, server, http.MethodPost, "/api/v1/workspace/file/content", req)
						got := decodeWorkspaceBody[streams.FileUpdateResponse](t, rec)
						wantHash := fmt.Sprintf("%x", sha256.Sum256([]byte(desired)))
						if rec.Code != http.StatusOK || !got.Success || got.Path != req.Path || got.NewHash != wantHash || got.Resolution != "applied" || got.Error != "" {
							t.Errorf("selected %s/%s ACK = status%d %+v; want applied hash %s", scope, req.Path, rec.Code, got, wantHash)
						}
						assertAPISaveTargetDisk(t, dir, "one.txt", scope, desired)
						assertAPISaveTargetEvent(t, sub, filepath.Join(scope, "one.txt"))
					})
				}
			}
		}
	}
}
