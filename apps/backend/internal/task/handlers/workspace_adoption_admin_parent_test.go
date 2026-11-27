package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func redirectAdoptionAdminParent(t *testing.T, f adoptionHTTPFixture) {
	t.Helper()
	parent := filepath.Join(f.source, ".git", "worktrees")
	external := filepath.Join(f.root, "external-administration")
	require.NoError(t, os.Rename(parent, external))
	if err := os.Symlink(external, parent); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
}

func TestWorkspaceAdoptionRefusesSymlinkedRetiredAdminParent(t *testing.T) {
	for _, boundary := range []string{"preview", "apply", "receipt"} {
		t.Run(boundary, func(t *testing.T) {
			f := newAdoptionHTTPFixture(t)
			if boundary == "preview" {
				redirectAdoptionAdminParent(t, f)
				require.Equal(t, http.StatusConflict, adoptionPreviewResponse(f))
				return
			}
			p := adoptionPreview(t, f)
			const operation = "11111111-1111-4111-8111-111111111111"
			request := map[string]any{"preview": p, "operation_id": operation}
			if boundary == "apply" {
				redirectAdoptionAdminParent(t, f)
				require.Equal(t, http.StatusConflict, adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/apply", request).Code)
				var count int
				require.NoError(t, f.db.Get(&count, `SELECT COUNT(*) FROM task_environment_checkout_adoptions`))
				require.Zero(t, count)
				return
			}
			require.Equal(t, http.StatusOK, adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/apply", request).Code)
			redirectAdoptionAdminParent(t, f)
			recorder := httptest.NewRecorder()
			f.router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-adoption/workspace-adoption/"+operation, nil))
			require.Equal(t, http.StatusConflict, recorder.Code)
			require.Error(t, f.manager.AdmitTaskRecovery(context.Background(), "task-adoption"))
		})
	}
}

func TestWorkspaceAdoptionPinnedRetiredAdminParentRejectsReplacement(t *testing.T) {
	f := newAdoptionHTTPFixture(t)
	p := adoptionPreview(t, f)
	inspection, err := f.manager.InspectWorkspaceAdoption(context.Background(), p.Selection, p.SlotID, p.Proof.CandidatePath)
	require.NoError(t, err)
	defer inspection.Close()
	parent := filepath.Join(f.source, ".git", "worktrees")
	require.NoError(t, os.Rename(parent, filepath.Join(f.root, "retained-administration")))
	require.NoError(t, os.Mkdir(parent, 0700))
	require.Error(t, inspection.Verify(context.Background()))
}
