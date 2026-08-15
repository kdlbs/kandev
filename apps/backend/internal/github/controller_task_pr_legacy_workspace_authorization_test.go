package github

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestHTTPTaskPRMutationsLegacyWorkspaceMismatch(t *testing.T) {
	tests := []struct {
		name   string
		method string
		body   string
		check  func(t *testing.T, store *Store, associationID string)
	}{
		{
			name:   "detach",
			method: http.MethodDelete,
			check: func(t *testing.T, store *Store, associationID string) {
				t.Helper()
				row, err := store.GetTaskPRByID(context.Background(), associationID)
				if err != nil {
					t.Fatalf("reload detached association: %v", err)
				}
				if row == nil || row.DetachedAt != nil {
					t.Fatalf("row = %+v, want active association", row)
				}
			},
		},
		{
			name:   "disposition",
			method: http.MethodPatch,
			body:   `{"disposition":"duplicate"}`,
			check: func(t *testing.T, store *Store, associationID string) {
				t.Helper()
				row, err := store.GetTaskPRByID(context.Background(), associationID)
				if err != nil {
					t.Fatalf("reload disposition association: %v", err)
				}
				if row == nil || row.Disposition != nil || row.DispositionRecordedAt != nil {
					t.Fatalf("row = %+v, want no disposition write", row)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, store := setupLegacyTaskPRControllerTest(t)
			association := &TaskPR{
				TaskID: "task-legacy", Owner: "acme", Repo: "demo", PRNumber: 12,
				PRURL: "https://github.com/acme/demo/pull/12", PRTitle: "closed pr",
				State: "closed", CreatedAt: time.Now().UTC(),
			}
			if err := store.CreateTaskPR(context.Background(), association); err != nil {
				t.Fatalf("create legacy association: %v", err)
			}
			path := "/api/v1/github/task-prs/" + association.ID
			if tt.method == http.MethodPatch {
				path += "/disposition"
			}
			path += "?workspace_id=ws-other"
			request := httptest.NewRequest(tt.method, path, bytes.NewBufferString(tt.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d, body = %s, want 404", response.Code, response.Body.String())
			}
			if got := strings.TrimSpace(response.Body.String()); got != `{"error":"task PR not found"}` {
				t.Fatalf("body = %s, want task PR not-found response", got)
			}
			tt.check(t, store, association.ID)
		})
	}
}

func setupLegacyTaskPRControllerTest(t *testing.T) (*gin.Engine, *Store) {
	t.Helper()
	svc, store := setupWatchServiceTest(t)
	svc.SetTaskIssueStore(&fakeTaskIssueStore{
		task: &taskmodels.Task{ID: "task-legacy", WorkspaceID: "ws-owner"},
	})
	controller := NewController(svc, newControllerTestLogger())
	router := gin.New()
	useControllerTestWorkspace(router)
	controller.RegisterHTTPRoutes(router)
	return router, store
}
