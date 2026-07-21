package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newMemberSubscriptionController(t *testing.T, upstream http.Handler) (*gin.Engine, *Store, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	srv := httptest.NewServer(upstream)
	store := newTestStore(t)
	seedWorkspace(t, store, "workspace-test")
	if err := store.UpsertConfigForWorkspace(t.Context(), "workspace-test", &GitLabConfig{
		Host: srv.URL, AuthMethod: AuthMethodPAT,
	}); err != nil {
		t.Fatalf("seed GitLab config: %v", err)
	}
	svc := NewService(srv.URL, NewNoopClient(srv.URL), AuthMethodNone, nil, newTestLogger(t))
	svc.SetStore(store)
	svc.SetWorkspaceSecretStore(&configTestSecrets{values: map[string]string{
		SecretKeyForWorkspace("workspace-test"): "tok",
	}})
	router := gin.New()
	NewController(svc, newTestLogger(t)).RegisterHTTPRoutes(router)
	return router, store, srv.Close
}

func memberSubscriptionRequest(router *gin.Engine, method, target, body string) *httptest.ResponseRecorder {
	var payload *bytes.Reader
	if body == "" {
		payload = bytes.NewReader(nil)
	} else {
		payload = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, target, payload)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	return res
}

func TestControllerProjectMembersRequiresWorkspace(t *testing.T) {
	router, _, stop := newMemberSubscriptionController(t, http.NotFoundHandler())
	defer stop()

	res := memberSubscriptionRequest(router, http.MethodGet, "/api/v1/gitlab/projects/members?project=group/project", "")
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", res.Code, res.Body.String())
	}
}

func TestControllerProjectMembersReturnsNumericIDs(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/projects/group/project/members/all") {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"id":42,"username":"alice","name":"Alice","avatar_url":"https://img/alice","state":"active"}]`))
	})
	router, _, stop := newMemberSubscriptionController(t, upstream)
	defer stop()

	res := memberSubscriptionRequest(router, http.MethodGet, "/api/v1/gitlab/projects/members?workspace_id=workspace-test&project=group/project&query=ali", "")
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", res.Code, res.Body.String())
	}
	var members []ProjectMember
	if err := json.Unmarshal(res.Body.Bytes(), &members); err != nil {
		t.Fatalf("decode members: %v", err)
	}
	if len(members) != 1 || members[0].ID != 42 {
		t.Fatalf("members = %#v, want numeric ID 42", members)
	}
}

func TestControllerSetMRReviewersAllowsEmptyReplacementAndRefreshes(t *testing.T) {
	putCount := 0
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			putCount++
			var payload struct {
				ReviewerIDs []int64 `json:"reviewer_ids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode reviewers: %v", err)
			}
			if payload.ReviewerIDs == nil || len(payload.ReviewerIDs) != 0 {
				t.Fatalf("reviewer_ids = %#v, want non-nil empty list", payload.ReviewerIDs)
			}
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"id":1,"iid":12,"state":"opened","references":{"full":"group/project!12"},"reviewers":[]}`))
		default:
			t.Errorf("method = %s", r.Method)
		}
	})
	router, _, stop := newMemberSubscriptionController(t, upstream)
	defer stop()

	res := memberSubscriptionRequest(router, http.MethodPut, "/api/v1/gitlab/mrs/reviewers?workspace_id=workspace-test", `{"project":"group/project","iid":12,"reviewer_ids":[]}`)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", res.Code, res.Body.String())
	}
	if putCount != 1 {
		t.Fatalf("reviewer PUT count = %d, want 1", putCount)
	}
	var mr MR
	if err := json.Unmarshal(res.Body.Bytes(), &mr); err != nil {
		t.Fatalf("decode MR: %v", err)
	}
	if mr.IID != 12 {
		t.Fatalf("MR IID = %d, want 12", mr.IID)
	}
}

func TestControllerSubscriptionsUseLiveUpstreamStateWithoutWatches(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/subscribe") {
			_, _ = w.Write([]byte(`{"subscribed":true}`))
			return
		}
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"subscribed":false}`))
			return
		}
		http.NotFound(w, r)
	})
	router, store, stop := newMemberSubscriptionController(t, upstream)
	defer stop()

	getRes := memberSubscriptionRequest(router, http.MethodGet, "/api/v1/gitlab/issues/subscription?workspace_id=workspace-test&project=group/project&iid=9", "")
	if getRes.Code != http.StatusOK || getRes.Body.String() != `{"subscribed":false}` {
		t.Fatalf("GET status/body = %d/%s", getRes.Code, getRes.Body.String())
	}
	putRes := memberSubscriptionRequest(router, http.MethodPut, "/api/v1/gitlab/mrs/subscription?workspace_id=workspace-test", `{"project":"group/project","iid":12,"subscribed":true}`)
	if putRes.Code != http.StatusOK || putRes.Body.String() != `{"subscribed":true}` {
		t.Fatalf("PUT status/body = %d/%s", putRes.Code, putRes.Body.String())
	}

	reviewWatches, err := store.ListReviewWatches(context.Background(), "workspace-test")
	if err != nil {
		t.Fatalf("list review watches: %v", err)
	}
	issueWatches, err := store.ListIssueWatches(context.Background(), "workspace-test")
	if err != nil {
		t.Fatalf("list issue watches: %v", err)
	}
	if len(reviewWatches) != 0 || len(issueWatches) != 0 {
		t.Fatalf("notification toggle changed Kandev watches: review=%d issue=%d", len(reviewWatches), len(issueWatches))
	}
}

func TestControllerProviderActionFailuresAreSanitized(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"reviewer private@example.com is not eligible; token=glpat-secret"}`))
	})
	router, _, stop := newMemberSubscriptionController(t, upstream)
	defer stop()

	res := memberSubscriptionRequest(router, http.MethodPut, "/api/v1/gitlab/mrs/reviewers?workspace_id=workspace-test", `{"project":"group/project","iid":12,"reviewer_ids":[999]}`)
	if res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body=%s", res.Code, res.Body.String())
	}
	if strings.Contains(res.Body.String(), "private@example.com") || strings.Contains(res.Body.String(), "glpat-secret") {
		t.Fatalf("response leaked provider details: %s", res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "not eligible") {
		t.Fatalf("response = %s, want action-specific sanitized error", res.Body.String())
	}
}

func TestControllerMRReviewEndpointsSanitizeProviderFailures(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"private@example.com token=glpat-secret"}`))
	})
	router, _, stop := newMemberSubscriptionController(t, upstream)
	defer stop()

	tests := []struct {
		name   string
		method string
		target string
		body   string
	}{
		{"feedback", http.MethodGet, "/api/v1/gitlab/mrs/feedback?workspace_id=workspace-test&project=group/project&iid=12", ""},
		{"discussion reply", http.MethodPost, "/api/v1/gitlab/mrs/discussions/notes?workspace_id=workspace-test", `{"project":"group/project","iid":12,"discussion_id":"d1","body":"reply"}`},
		{"discussion resolve", http.MethodPost, "/api/v1/gitlab/mrs/discussions/resolve?workspace_id=workspace-test", `{"project":"group/project","iid":12,"discussion_id":"d1"}`},
		{"merge", http.MethodPut, "/api/v1/gitlab/mrs/merge?workspace_id=workspace-test", `{"project":"group/project","iid":12,"method":"merge"}`},
		{"approve", http.MethodPost, "/api/v1/gitlab/mrs/approve?workspace_id=workspace-test", `{"project":"group/project","iid":12}`},
		{"unapprove", http.MethodPost, "/api/v1/gitlab/mrs/unapprove?workspace_id=workspace-test", `{"project":"group/project","iid":12}`},
		{"labels", http.MethodPut, "/api/v1/gitlab/mrs/labels?workspace_id=workspace-test", `{"project":"group/project","iid":12,"labels":["bug"]}`},
		{"assignees", http.MethodPut, "/api/v1/gitlab/mrs/assignees?workspace_id=workspace-test", `{"project":"group/project","iid":12,"assignee_ids":[1]}`},
		{"files", http.MethodGet, "/api/v1/gitlab/mrs/files?workspace_id=workspace-test&project=group/project&iid=12", ""},
		{"commits", http.MethodGet, "/api/v1/gitlab/mrs/commits?workspace_id=workspace-test&project=group/project&iid=12", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := memberSubscriptionRequest(router, tt.method, tt.target, tt.body)
			if res.Code != http.StatusBadGateway {
				t.Fatalf("status = %d, want 502; body=%s", res.Code, res.Body.String())
			}
			if strings.Contains(res.Body.String(), "private@example.com") || strings.Contains(res.Body.String(), "glpat-secret") {
				t.Fatalf("response leaked provider details: %s", res.Body.String())
			}
		})
	}
}

func TestControllerExpectedHostMismatchFailsBeforeProviderRequest(t *testing.T) {
	requestCount := 0
	upstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		_, _ = w.Write([]byte(`{}`))
	})
	router, _, stop := newMemberSubscriptionController(t, upstream)
	defer stop()

	res := memberSubscriptionRequest(router, http.MethodGet, "/api/v1/gitlab/mrs/feedback?workspace_id=workspace-test&expected_host=https%3A%2F%2Fother.example&project=group/project&iid=12", "")
	if res.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", res.Code, res.Body.String())
	}
	if requestCount != 0 {
		t.Fatalf("provider requests = %d, want 0", requestCount)
	}
}
