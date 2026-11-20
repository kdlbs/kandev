package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/improvekandev"
	"github.com/stretchr/testify/require"
)

type sequencedImproveKandevCloner struct {
	paths []string
	calls atomic.Int32
}

func (c *sequencedImproveKandevCloner) EnsureWorkspaceCloned(context.Context, string, string, string, string, string) (string, error) {
	index := int(c.calls.Add(1)) - 1
	if index >= len(c.paths) {
		index = len(c.paths) - 1
	}
	return c.paths[index], nil
}

func TestImproveKandevBootstrapRepairsRepositoryWithMissingCheckout(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	t.Cleanup(ts.Close)
	oldPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", oldPath).Run())
	newPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", newPath).Run())
	cloner := &sequencedImproveKandevCloner{paths: []string{oldPath, newPath}}
	handler := newImproveKandevBootstrapHandler(ts, cloner)
	t.Cleanup(func() { _ = handler.Stop() })
	router := newImproveKandevTestRouter(handler)

	first := postImproveKandevBootstrapSync(t, router)
	require.NotEmpty(t, first.RepositoryID)
	require.EqualValues(t, 1, cloner.calls.Load())
	require.NoError(t, os.RemoveAll(oldPath))

	second := postImproveKandevBootstrapSync(t, router)
	require.Equal(t, first.RepositoryID, second.RepositoryID)
	require.EqualValues(t, 2, cloner.calls.Load())

	repository, err := ts.TaskSvc.GetRepository(context.Background(), first.RepositoryID)
	require.NoError(t, err)
	require.Equal(t, newPath, repository.LocalPath)
	_, err = os.Stat(newPath + "/.git")
	require.NoError(t, err)
}

func newImproveKandevBootstrapHandler(ts *OrchestratorTestServer, cloner *sequencedImproveKandevCloner) *improvekandev.Handler {
	return improvekandev.NewHandler(ts.TaskSvc, cloner, nil, nil, "test", ts.Logger)
}

func postImproveKandevBootstrapSync(t *testing.T, router http.Handler) improvekandev.BootstrapResponse {
	t.Helper()
	body, err := json.Marshal(improvekandev.BootstrapRequest{CreateWorkspace: true})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/system/improve-kandev/bootstrap", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	var result improvekandev.BootstrapResponse
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	return result
}
