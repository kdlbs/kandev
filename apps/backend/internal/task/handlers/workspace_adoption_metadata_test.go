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

func adoptionOrphan(t *testing.T, f adoptionHTTPFixture) string {
	t.Helper()
	sourceHead := adoptionGit(t, f.source, "rev-parse", "feature/adoption")
	adoptionGit(t, f.clone, "config", "user.email", "fixture@example.invalid")
	adoptionGit(t, f.clone, "config", "user.name", "Fixture")
	adoptionGit(t, f.clone, "config", "commit.gpgsign", "false")
	adoptionGit(t, f.clone, "checkout", "--orphan", "unrelated-history")
	adoptionGit(t, f.clone, "add", "tracked", "untracked")
	adoptionGit(t, f.clone, "commit", "-m", "unrelated root")
	adoptionGit(t, f.clone, "branch", "-D", "feature/adoption")
	adoptionGit(t, f.clone, "branch", "-m", "feature/adoption")
	return sourceHead
}

func adoptionPreviewResponse(f adoptionHTTPFixture) int {
	return adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/preview", map[string]any{"session_id": "session-adoption", "slot_id": "slot-adoption", "checkout_path": f.clone}).Code
}

func TestWorkspaceAdoptionIgnoresInheritedGitRepositorySelection(t *testing.T) {
	for _, key := range []string{"GIT_DIR", "GIT_OBJECT_DIRECTORY"} {
		t.Run(key, func(t *testing.T) {
			f := newAdoptionHTTPFixture(t)
			foreign := filepath.Join(f.root, "unrelated-metadata")
			t.Setenv(key, foreign)
			p := adoptionPreview(t, f)
			rec := adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/apply", map[string]any{"preview": p, "operation_id": "11111111-1111-4111-8111-111111111111"})
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.NoError(t, f.manager.AdmitTaskRecovery(context.Background(), "task-adoption"))
		})
	}
}

func TestWorkspaceAdoptionRefusesReplacementAncestryAndLegacyGrafts(t *testing.T) {
	for _, kind := range []string{"replacement", "legacy graft"} {
		t.Run(kind, func(t *testing.T) {
			f := newAdoptionHTTPFixture(t)
			sourceHead := adoptionOrphan(t, f)
			if kind == "replacement" {
				adoptionGit(t, f.clone, "replace", "--graft", "HEAD", sourceHead)
			} else {
				head := adoptionGit(t, f.clone, "rev-parse", "HEAD")
				require.NoError(t, os.WriteFile(filepath.Join(f.clone, ".git", "info", "grafts"), []byte(fmt.Sprintf("%s %s\n", head, sourceHead)), 0600))
			}
			require.Equal(t, http.StatusConflict, adoptionPreviewResponse(f))
		})
	}
}

func adoptionEscapeRef(t *testing.T, f adoptionHTTPFixture, directory bool) {
	t.Helper()
	path := filepath.Join(f.clone, ".git", "refs", "heads", "feature", "adoption")
	target := filepath.Join(f.root, "external-ref")
	if directory {
		path = filepath.Dir(path)
		target = filepath.Join(f.root, "external-refs")
	}
	require.NoError(t, os.Rename(path, target))
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
}

func TestWorkspaceAdoptionRefusesEscapedRefAtEveryBoundary(t *testing.T) {
	for _, directory := range []bool{false, true} {
		for _, boundary := range []string{"preview", "apply", "receipt and reuse"} {
			t.Run(fmt.Sprintf("%t/%s", directory, boundary), func(t *testing.T) {
				f := newAdoptionHTTPFixture(t)
				if boundary == "preview" {
					adoptionEscapeRef(t, f, directory)
					require.Equal(t, http.StatusConflict, adoptionPreviewResponse(f))
					return
				}
				p := adoptionPreview(t, f)
				request := map[string]any{"preview": p, "operation_id": "11111111-1111-4111-8111-111111111111"}
				if boundary == "apply" {
					adoptionEscapeRef(t, f, directory)
					rec := adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/apply", request)
					require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
					return
				}
				rec := adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/apply", request)
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				adoptionEscapeRef(t, f, directory)
				rec = adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/apply", request)
				require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
				require.Error(t, f.manager.AdmitTaskRecovery(context.Background(), "task-adoption"))
			})
		}
	}
}

func TestWorkspaceAdoptionAcceptsPackedRefs(t *testing.T) {
	f := newAdoptionHTTPFixture(t)
	adoptionGit(t, f.clone, "pack-refs", "--all", "--prune")
	p := adoptionPreview(t, f)
	rec := adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/apply", map[string]any{"preview": p, "operation_id": "11111111-1111-4111-8111-111111111111"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, f.manager.AdmitTaskRecovery(context.Background(), "task-adoption"))
}

func TestWorkspaceAdoptionRefusesRedirectedObjectStorage(t *testing.T) {
	for _, kind := range []string{"pack directory", "alternates"} {
		t.Run(kind, func(t *testing.T) {
			f := newAdoptionHTTPFixture(t)
			if kind == "pack directory" {
				path := filepath.Join(f.clone, ".git", "objects", "pack")
				external := filepath.Join(f.root, "external-pack")
				require.NoError(t, os.Rename(path, external))
				if err := os.Symlink(external, path); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			} else {
				require.NoError(t, os.WriteFile(filepath.Join(f.clone, ".git", "objects", "info", "alternates"), []byte(filepath.Join(f.source, ".git", "objects")+"\n"), 0600))
			}
			require.Equal(t, http.StatusConflict, adoptionPreviewResponse(f))
		})
	}
}
