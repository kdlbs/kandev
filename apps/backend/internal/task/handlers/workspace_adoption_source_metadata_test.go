package handlers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func escapeAdoptionSourceMetadata(t *testing.T, f adoptionHTTPFixture, kind string) {
	t.Helper()
	metadata := filepath.Join(f.source, ".git")
	switch kind {
	case "alternates":
		require.NoError(t, os.WriteFile(filepath.Join(metadata, "objects", "info", "alternates"), []byte(filepath.Join(f.clone, ".git", "objects")+"\n"), 0600))
	case "promisor":
		require.NoError(t, os.WriteFile(filepath.Join(metadata, "objects", "pack", "external.promisor"), nil, 0600))
	default:
		path := filepath.Join(metadata, filepath.FromSlash(kind))
		external := filepath.Join(f.root, "external-source-metadata")
		require.NoError(t, os.Rename(path, external))
		if err := os.Symlink(external, path); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
	}
}

func TestWorkspaceAdoptionRefusesEscapedSourceMetadata(t *testing.T) {
	for _, kind := range []string{"config", "HEAD", "objects", "objects/pack", "alternates", "promisor"} {
		for _, boundary := range []string{"preview", "apply", "receipt and reuse"} {
			t.Run(fmt.Sprintf("%s/%s", kind, boundary), func(t *testing.T) {
				f := newAdoptionHTTPFixture(t)
				if boundary == "preview" {
					escapeAdoptionSourceMetadata(t, f, kind)
					require.Equal(t, http.StatusConflict, adoptionPreviewResponse(f))
					return
				}
				p := adoptionPreview(t, f)
				const operation = "11111111-1111-4111-8111-111111111111"
				request := map[string]any{"preview": p, "operation_id": operation}
				if boundary == "apply" {
					escapeAdoptionSourceMetadata(t, f, kind)
					require.Equal(t, http.StatusConflict, adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/apply", request).Code)
					var count int
					require.NoError(t, f.db.Get(&count, `SELECT COUNT(*) FROM task_environment_checkout_adoptions`))
					require.Zero(t, count)
					return
				}
				require.Equal(t, http.StatusOK, adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/apply", request).Code)
				escapeAdoptionSourceMetadata(t, f, kind)
				require.Equal(t, http.StatusConflict, adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/apply", request).Code)
				require.Error(t, f.manager.AdmitTaskRecovery(context.Background(), "task-adoption"))
			})
		}
	}
}
