package dashboard_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func resolveIdentifierReq(wsID, identifier string) *http.Request {
	return httptest.NewRequest(http.MethodGet,
		"/api/v1/office/workspaces/"+wsID+"/tasks/by-identifier/"+identifier, nil)
}

func TestResolveTaskIdentifier(t *testing.T) {
	f := newTasksSecurityFixture(t)
	seedTasksWorkspace(t, f.repo, "ws-1")
	seedTasksWorkspace(t, f.repo, "ws-2")
	for _, row := range [][]string{
		{"task-uuid-1", "ws-1", "KAN-11"},
		{"task-uuid-2", "ws-2", "KAN-11"},
	} {
		if _, err := f.repo.ExecRaw(context.Background(),
			`INSERT INTO tasks (id, workspace_id, identifier, title) VALUES (?, ?, ?, 't')`,
			row[0], row[1], row[2],
		); err != nil {
			t.Fatalf("seed task: %v", err)
		}
	}

	cases := []struct {
		name       string
		ws, ident  string
		wantStatus int
		wantID     string
	}{
		{"found", "ws-1", "KAN-11", http.StatusOK, "task-uuid-1"},
		{"same identifier, other workspace", "ws-2", "KAN-11", http.StatusOK, "task-uuid-2"},
		{"not found", "ws-1", "KAN-99", http.StatusNotFound, ""},
		{"wrong workspace", "ws-3", "KAN-11", http.StatusNotFound, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			f.router.ServeHTTP(rec, resolveIdentifierReq(tc.ws, tc.ident))
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantID == "" {
				return
			}
			var body struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.ID != tc.wantID {
				t.Fatalf("id = %q, want %q", body.ID, tc.wantID)
			}
		})
	}
}
