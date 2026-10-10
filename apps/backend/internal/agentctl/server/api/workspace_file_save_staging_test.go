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
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func newAPIStagingSaveFixture(t *testing.T, scoped bool) (*gitAPIFixture, streams.FileUpdateRequest, string) {
	t.Helper()
	fixture := newFileUpdateSaveFixture(t)
	req := fileUpdateSaveRequests()[0]
	if scoped {
		req.Repo = "beta"
		if err := os.Mkdir(filepath.Join(fixture.repo, req.Repo), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFileAPI(t, fixture.repo, filepath.Join(req.Repo, req.Path), "selected beta original\n")
		writeFileAPI(t, fixture.repo, "alpha.txt", "nonselected root original\n")
		desired := "selected beta saved\n"
		req.Diff = "Index: alpha.txt\n===================================================================\n" +
			"--- alpha.txt\t\n+++ alpha.txt\t\n@@ -1 +1 @@\n-selected beta original\n+selected beta saved\n"
		req.OriginalHash = fmt.Sprintf("%x", sha256.Sum256([]byte("selected beta original\n")))
		req.DesiredContent = &desired
		runGitAPI(t, fixture.repo, "add", ".")
		runGitAPI(t, fixture.repo, "commit", "-m", "seed scoped save")
	}
	original := string(mustReadAPIStagingFile(t, filepath.Join(fixture.repo, req.Repo, req.Path)))
	writeFileAPI(t, fixture.repo, ".kandev-patch-user", "user-owned patch-like file\n")
	writeFileAPI(t, fixture.repo, "addition.txt", "ordinary addition\n")
	if err := os.Remove(filepath.Join(fixture.repo, "beta.txt")); err != nil {
		t.Fatal(err)
	}
	return fixture, req, original
}

func mustReadAPIStagingFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func beginAPIStagingRequest(ctx context.Context, t *testing.T, server *Server, path string, body any, pending *sync.WaitGroup) <-chan *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan *httptest.ResponseRecorder, 1)
	pending.Add(1)
	go func() {
		defer pending.Done()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		server.Router().ServeHTTP(rec, req.WithContext(ctx))
		result <- rec
	}()
	return result
}

func assertAPIStagingSave(t *testing.T, fixture *gitAPIFixture, req streams.FileUpdateRequest, saved, staged *httptest.ResponseRecorder, indexed string) {
	t.Helper()
	got := decodeWorkspaceBody[streams.FileUpdateResponse](t, saved)
	wantHash := fmt.Sprintf("%x", sha256.Sum256([]byte(*req.DesiredContent)))
	if saved.Code != http.StatusOK || !got.Success || got.Path != req.Path || got.NewHash != wantHash || got.Resolution != "applied" || got.Error != "" {
		t.Errorf("save ACK = status %d %+v; want applied hash %s", saved.Code, got, wantHash)
	}
	stage := decodeGitOperationResult(t, staged)
	if staged.Code != http.StatusOK || !stage.Success || stage.Operation != "stage" || stage.Error != "" {
		t.Errorf("Stage All = status %d %+v; want success", staged.Code, stage)
	}
	assertFileUpdateSaveBytes(t, fixture.repo, filepath.Join(req.Repo, req.Path), *req.DesiredContent)
	assertFileUpdateSaveBytes(t, fixture.repo, "neighbor.txt", "neighbor unchanged\n")
	want := map[string]string{
		".kandev-patch-user": "user-owned patch-like file\n", "README.md": "base\n",
		"one.txt": "one\n", "two.txt": "two\n", "addition.txt": "ordinary addition\n",
		"alpha.txt": indexed, "neighbor.txt": "neighbor unchanged\n",
	}
	if req.Repo != "" {
		want["alpha.txt"] = "nonselected root original\n"
		want[req.Repo+"/"+req.Path] = indexed
		assertFileUpdateSaveBytes(t, fixture.repo, "alpha.txt", want["alpha.txt"])
	}
	var expected []string
	for path, content := range want {
		expected = append(expected, path)
		if actual := runGitAPI(t, fixture.repo, "show", ":"+path); actual != content {
			t.Errorf("indexed blob %s = %q; want %q", path, actual, content)
		}
	}
	slices.Sort(expected)
	actual := strings.Split(strings.TrimSuffix(runGitAPI(t, fixture.repo, "ls-files", "-z"), "\x00"), "\x00")
	slices.Sort(actual)
	if !slices.Equal(actual, expected) {
		t.Errorf("real index paths = %q; want exactly %q", actual, expected)
	}
	assertFileUpdateSaveBytes(t, fixture.repo, ".kandev-patch-user", want[".kandev-patch-user"])
}

// @covers AC-WORKSPACES-SAVED-FILE-CONTENT-001.5
func TestHandleFileUpdate_StageAllOverlap(t *testing.T) {
	for _, scoped := range []bool{false, true} {
		t.Run(fmt.Sprintf("scoped=%t", scoped), func(t *testing.T) {
			fixture, req, original := newAPIStagingSaveFixture(t, scoped)
			ctx, release, pending := holdFileUpdateAdmission(t)
			staged := beginAPIStagingRequest(ctx, t, fixture.server, "/api/v1/git/stage", GitStageRequest{}, pending)
			awaitQueuedFileUpdates(t, ctx, 1)
			saved := beginAPIStagingRequest(ctx, t, fixture.server, "/api/v1/workspace/file/content", req, pending)
			awaitQueuedFileUpdates(t, ctx, 2)
			release()
			stageReply := awaitFileUpdateSave(t, ctx, staged)
			saveReply := awaitFileUpdateSave(t, ctx, saved)
			pending.Wait()
			assertAPIStagingSave(t, fixture, req, saveReply, stageReply, original)
		})
	}
}

// @covers AC-WORKSPACES-SAVED-FILE-CONTENT-001.5
func TestHandleFileUpdate_StageAllSequentialControl(t *testing.T) {
	fixture, req, _ := newAPIStagingSaveFixture(t, false)
	saved := workspaceRequest(t, fixture.server, http.MethodPost, "/api/v1/workspace/file/content", req)
	staged := postGitAPI(t, fixture.server, "/api/v1/git/stage", GitStageRequest{})
	assertAPIStagingSave(t, fixture, req, saved, staged, *req.DesiredContent)
}
