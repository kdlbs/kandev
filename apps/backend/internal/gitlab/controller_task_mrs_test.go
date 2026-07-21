package gitlab

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func newTaskMRControllerFixture(t *testing.T) (*gin.Engine, *Store) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	const host = "https://gitlab.acme.test"
	svc, store, client := newTaskMRLinkService(t, host)
	seedTaskMRLinkFixture(t, store, "ws-1", "task-1", "repo-1")
	setTaskMRRepositoryIdentity(t, store, "repo-1", host, "group/project")
	client.SeedMR("group/project", &MR{
		IID: 7, Title: "Review this", WebURL: host + "/group/project/-/merge_requests/7",
		State: "opened", HeadBranch: "feature", BaseBranch: "main", CreatedAt: time.Now().UTC(),
	})
	router := gin.New()
	controller := NewController(svc, newTestLogger(t))
	controller.RegisterTaskMRHTTPRoutes(router.Group("/api/v1/gitlab"))
	return router, store
}

func TestTaskMRControllerCreatesAndUnlinksAssociation(t *testing.T) {
	router, store := newTaskMRControllerFixture(t)
	body := `{"task_id":"task-1","repository_id":"repo-1","mr_url":"https://gitlab.acme.test/group/project/-/merge_requests/7"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gitlab/task-mrs?workspace_id=ws-1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("POST status = %d, body = %s", response.Code, response.Body.String())
	}
	var association TaskMR
	if err := json.Unmarshal(response.Body.Bytes(), &association); err != nil || association.ID == "" {
		t.Fatalf("decode association: %+v, err = %v", association, err)
	}

	deleteRequest := httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/gitlab/task-mrs/"+association.ID+"?workspace_id=ws-1",
		nil,
	)
	deleteResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d, body = %s", deleteResponse.Code, deleteResponse.Body.String())
	}
	rows, err := store.ListTaskMRsByTask(t.Context(), "task-1")
	if err != nil || len(rows) != 0 {
		t.Fatalf("rows after unlink = %d, err = %v", len(rows), err)
	}
}

func TestTaskMRControllerMapsScopeAndURLFailures(t *testing.T) {
	router, _ := newTaskMRControllerFixture(t)
	tests := []struct {
		name   string
		target string
		body   string
		want   int
	}{
		{name: "workspace required", target: "/api/v1/gitlab/task-mrs", body: `{}`, want: http.StatusBadRequest},
		{
			name: "wrong host", target: "/api/v1/gitlab/task-mrs?workspace_id=ws-1",
			body: `{"task_id":"task-1","repository_id":"repo-1","mr_url":"https://gitlab.com/group/project/-/merge_requests/7"}`,
			want: http.StatusBadRequest,
		},
		{
			name: "unknown task", target: "/api/v1/gitlab/task-mrs?workspace_id=ws-1",
			body: `{"task_id":"missing","repository_id":"repo-1","mr_url":"https://gitlab.acme.test/group/project/-/merge_requests/7"}`,
			want: http.StatusNotFound,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, test.target, strings.NewReader(test.body))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.want, response.Body.String())
			}
		})
	}
}
