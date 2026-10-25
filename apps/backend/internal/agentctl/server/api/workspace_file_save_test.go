package api

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/subproc"
)

func fileUpdateSaveRequests() []streams.FileUpdateRequest {
	alpha, beta := "alpha saved\n", "beta saved\n"
	return []streams.FileUpdateRequest{
		{Path: "alpha.txt", Diff: "--- alpha.txt\n+++ alpha.txt\n@@ -1 +1 @@\n-alpha original\n+alpha saved\n",
			OriginalHash: fmt.Sprintf("%x", sha256.Sum256([]byte("alpha original\n"))), DesiredContent: &alpha},
		{Path: "beta.txt", Diff: "--- beta.txt\n+++ beta.txt\n@@ -1 +1 @@\n-beta original\n+beta saved\n",
			OriginalHash: fmt.Sprintf("%x", sha256.Sum256([]byte("beta original\n"))), DesiredContent: &beta},
	}
}

func newFileUpdateSaveFixture(t *testing.T) *gitAPIFixture {
	t.Helper()
	fixture := newGitAPIFixture(t)
	writeFileAPI(t, fixture.repo, "alpha.txt", "alpha original\n")
	writeFileAPI(t, fixture.repo, "beta.txt", "beta original\n")
	writeFileAPI(t, fixture.repo, "neighbor.txt", "neighbor unchanged\n")
	runGitAPI(t, fixture.repo, "add", ".")
	runGitAPI(t, fixture.repo, "commit", "-m", "seed file saves")
	return fixture
}

func assertFileUpdateSaved(t *testing.T, fixture *gitAPIFixture, req streams.FileUpdateRequest, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Errorf("%s status=%d, body=%s", req.Path, rec.Code, rec.Body.String())
		return
	}
	got := decodeWorkspaceBody[streams.FileUpdateResponse](t, rec)
	wantHash := fmt.Sprintf("%x", sha256.Sum256([]byte(*req.DesiredContent)))
	if !got.Success || got.Path != req.Path || got.NewHash != wantHash || got.Resolution != "applied" || got.Error != "" {
		t.Errorf("%s response=%+v; want successful applied save with hash=%s", req.Path, got, wantHash)
	}
	assertFileUpdateSaveBytes(t, fixture.repo, req.Path, *req.DesiredContent)
}

func assertFileUpdateSaveBytes(t *testing.T, dir, path, want string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(dir, path))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("%s disk=%q; want %q", path, got, want)
	}
}

func awaitQueuedFileUpdates(t *testing.T, ctx context.Context, want int) {
	t.Helper()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for subproc.AdmissionSnapshot().Waiters != want {
		select {
		case <-ctx.Done():
			t.Fatalf("expected %d queued file updates: %v", want, ctx.Err())
		case <-ticker.C:
		}
	}
}

func holdFileUpdateAdmission(t *testing.T) (context.Context, func(), *sync.WaitGroup) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	restore := subproc.Git().SetCapForTest(1)
	t.Cleanup(restore)
	release, err := subproc.AcquireGit(ctx, subproc.GitInteractive)
	if err != nil {
		t.Fatal(err)
	}
	requests := &sync.WaitGroup{}
	t.Cleanup(func() {
		cancel()
		release()
		requests.Wait()
	})
	return ctx, release, requests
}

func startFileUpdateSave(t *testing.T, server *Server, req streams.FileUpdateRequest, requests *sync.WaitGroup) <-chan *httptest.ResponseRecorder {
	t.Helper()
	result := make(chan *httptest.ResponseRecorder, 1)
	requests.Add(1)
	go func() {
		defer requests.Done()
		result <- workspaceRequest(t, server, http.MethodPost, "/api/v1/workspace/file/content", req)
	}()
	return result
}

func awaitFileUpdateSave(t *testing.T, ctx context.Context, result <-chan *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	t.Helper()
	select {
	case got := <-result:
		return got
	case <-ctx.Done():
		t.Fatalf("file update did not settle: %v", ctx.Err())
		return nil
	}
}

func TestHandleFileUpdate_ConcurrentDistinctFiles(t *testing.T) {
	fixture := newFileUpdateSaveFixture(t)
	ctx, release, pending := holdFileUpdateAdmission(t)
	requests := fileUpdateSaveRequests()
	alpha := startFileUpdateSave(t, fixture.server, requests[0], pending)
	awaitQueuedFileUpdates(t, ctx, 1)
	beta := startFileUpdateSave(t, fixture.server, requests[1], pending)
	awaitQueuedFileUpdates(t, ctx, 2)
	release()
	a := awaitFileUpdateSave(t, ctx, alpha)
	b := awaitFileUpdateSave(t, ctx, beta)
	pending.Wait()
	assertFileUpdateSaved(t, fixture, requests[0], a)
	assertFileUpdateSaved(t, fixture, requests[1], b)
	assertFileUpdateSaveBytes(t, fixture.repo, "neighbor.txt", "neighbor unchanged\n")
}

func TestHandleFileUpdate_SequentialDistinctFiles(t *testing.T) {
	fixture := newFileUpdateSaveFixture(t)
	for _, req := range fileUpdateSaveRequests() {
		t.Run(req.Path, func(t *testing.T) {
			rec := workspaceRequest(t, fixture.server, http.MethodPost, "/api/v1/workspace/file/content", req)
			assertFileUpdateSaved(t, fixture, req, rec)
		})
	}
	assertFileUpdateSaveBytes(t, fixture.repo, "neighbor.txt", "neighbor unchanged\n")
}
