package gitlab

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newMockControllerFixture(t *testing.T) (*gin.Engine, *MockClient, *MockClient) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	legacy := NewMockClient(DefaultHost)
	workspaceA := NewMockClient("https://gitlab-a.example.com")
	workspaceB := NewMockClient("https://gitlab-b.example.com")
	service := NewService(DefaultHost, legacy, "mock", nil, newTestLogger(t))
	service.SetStore(newTestStore(t))
	service.workspaceClients["workspace-a"] = workspaceA
	service.workspaceClients["workspace-b"] = workspaceB
	router := gin.New()
	NewMockController(legacy, service, newTestLogger(t)).RegisterRoutes(router)
	return router, workspaceA, workspaceB
}

func mockControlRequest(t *testing.T, router *gin.Engine, method, target string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
	}
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	return res
}

func TestMockControllerSeedsOnlyRequestedWorkspace(t *testing.T) {
	router, workspaceA, workspaceB := newMockControllerFixture(t)
	res := mockControlRequest(t, router, http.MethodPost,
		"/api/v1/gitlab/mock/mrs?workspace_id=workspace-a",
		mockMRRequest{Project: "group/project", MRs: []MR{{IID: 7, Title: "Workspace A"}}})
	if res.Code != http.StatusOK {
		t.Fatalf("seed status=%d body=%s", res.Code, res.Body.String())
	}
	if _, err := workspaceA.GetMR(t.Context(), "group/project", 7); err != nil {
		t.Fatalf("workspace A MR missing: %v", err)
	}
	if _, err := workspaceB.GetMR(t.Context(), "group/project", 7); err == nil {
		t.Fatal("workspace B observed workspace A seed")
	}
}

func TestMockControllerSeedsReviewDataAndResetsWorkspace(t *testing.T) {
	router, workspaceA, workspaceB := newMockControllerFixture(t)
	workspaceB.SeedMR("other/project", &MR{IID: 9, Title: "Keep"})

	requests := []struct {
		path string
		body any
	}{
		{"/members", mockMembersRequest{Project: "group/project", Members: []ProjectMember{{ID: 42, Username: "alice"}}}},
		{"/files", mockFilesRequest{Project: "group/project", IID: 7, Files: []MRFile{{Filename: "main.go"}}}},
		{"/commits", mockCommitsRequest{Project: "group/project", IID: 7, Commits: []MRCommitInfo{{SHA: "abc"}}}},
		{"/repo-files", mockRepoFilesRequest{
			Project: "group/project", Ref: "main",
			Files: []mockRepoFileItem{{Path: ".kandev/workflows/review.yaml", Content: "name: Review\n"}},
		}},
	}
	workspaceA.SeedMR("group/project", &MR{IID: 7, Title: "Reset me"})
	for _, item := range requests {
		res := mockControlRequest(t, router, http.MethodPost,
			"/api/v1/gitlab/mock"+item.path+"?workspace_id=workspace-a", item.body)
		if res.Code != http.StatusOK {
			t.Fatalf("seed %s status=%d body=%s", item.path, res.Code, res.Body.String())
		}
	}

	res := mockControlRequest(t, router, http.MethodDelete,
		"/api/v1/gitlab/mock/reset?workspace_id=workspace-a", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("reset status=%d body=%s", res.Code, res.Body.String())
	}
	if got := workspaceA.Stats(); got != "mrs=0 discussions=0 issues=0" {
		t.Fatalf("workspace A after reset = %q", got)
	}
	if entries, err := workspaceA.ListRepoTree(t.Context(), "group/project", ".kandev/workflows", "main"); err != nil || len(entries) != 0 {
		t.Fatalf("workspace A repo files after reset: entries=%+v err=%v", entries, err)
	}
	if _, err := workspaceB.GetMR(t.Context(), "other/project", 9); err != nil {
		t.Fatalf("workspace B was reset: %v", err)
	}
}

func TestMockControllerRejectsUnknownWorkspace(t *testing.T) {
	router, _, _ := newMockControllerFixture(t)
	res := mockControlRequest(t, router, http.MethodPost,
		"/api/v1/gitlab/mock/mrs?workspace_id=missing",
		mockMRRequest{Project: "group/project", MRs: []MR{{IID: 7}}})
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestMockControllerServesWorkspaceScopedGitLabCreationAPI(t *testing.T) {
	router, workspaceA, workspaceB := newMockControllerFixture(t)
	payload := mockCreateMRRequest{
		SourceBranch: "feature/gitlab", TargetBranch: "main", Title: "Draft: Created from task",
		Description: "Creation body",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost,
		"/api/v4/projects/group%2Fproject/merge_requests", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("PRIVATE-TOKEN", mockWorkspaceTokenPrefix+"workspace-a")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", res.Code, res.Body.String())
	}
	mr, err := workspaceA.GetMR(t.Context(), "group/project", 100)
	if err != nil {
		t.Fatalf("workspace A created MR missing: %v", err)
	}
	if mr.Title != payload.Title || !mr.Draft || mr.HeadBranch != payload.SourceBranch {
		t.Fatalf("created MR = %#v", mr)
	}
	if _, err := workspaceB.GetMR(t.Context(), "group/project", 100); err == nil {
		t.Fatal("workspace B observed workspace A REST-created MR")
	}

	get := httptest.NewRequest(http.MethodGet,
		"/api/v4/projects/group%2Fproject/merge_requests?source_branch=feature%2Fgitlab&target_branch=main",
		nil)
	get.Header.Set("PRIVATE-TOKEN", mockWorkspaceTokenPrefix+"workspace-a")
	listed := httptest.NewRecorder()
	router.ServeHTTP(listed, get)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), mr.WebURL) {
		t.Fatalf("list status=%d body=%s", listed.Code, listed.Body.String())
	}
}

func TestMockControllerSeedsRepoFilesScopedToWorkspace(t *testing.T) {
	router, workspaceA, workspaceB := newMockControllerFixture(t)
	res := mockControlRequest(t, router, http.MethodPost,
		"/api/v1/gitlab/mock/repo-files?workspace_id=workspace-a",
		mockRepoFilesRequest{
			Project: "group/project",
			Ref:     "main",
			Files: []mockRepoFileItem{
				{Path: ".kandev/workflows/review.yaml", Content: "name: Review\n"},
			},
		})
	if res.Code != http.StatusOK {
		t.Fatalf("seed status=%d body=%s", res.Code, res.Body.String())
	}

	entries, err := workspaceA.ListRepoTree(t.Context(), "group/project", ".kandev/workflows", "main")
	if err != nil {
		t.Fatalf("workspace A tree missing: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "review.yaml" {
		t.Fatalf("workspace A entries = %+v", entries)
	}

	bEntries, err := workspaceB.ListRepoTree(t.Context(), "group/project", ".kandev/workflows", "main")
	if err != nil {
		t.Fatalf("workspace B tree err = %v", err)
	}
	if len(bEntries) != 0 {
		t.Fatalf("workspace B observed workspace A's seeded repo file: %+v", bEntries)
	}
}

func TestMockControllerRejectsIncompleteRepoFilesRequest(t *testing.T) {
	router, _, _ := newMockControllerFixture(t)
	res := mockControlRequest(t, router, http.MethodPost,
		"/api/v1/gitlab/mock/repo-files?workspace_id=workspace-a",
		mockRepoFilesRequest{Project: "group/project"})
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestMockControllerSeedsRemainingResourceTypes(t *testing.T) {
	router, workspaceA, _ := newMockControllerFixture(t)
	workspaceA.SeedMR("group/project", &MR{IID: 7, Title: "Review"})
	requests := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodPut, "/user", map[string]string{"username": "reviewer"}},
		{http.MethodPost, "/issues", mockIssueRequest{Project: "group/project", Issues: []Issue{{IID: 3, Title: "Bug", State: "opened"}}}},
		{http.MethodPost, "/pipelines", mockPipelineRequest{Project: "group/project", Pipelines: []Pipeline{{ID: 10, Status: "failed"}}}},
		{http.MethodPost, "/pipeline-jobs", mockPipelineJobsRequest{PipelineID: 10, Jobs: []PipelineJob{{ID: 11, Name: "test", Status: "failed"}}}},
		{http.MethodPost, "/discussions", mockDiscussionRequest{Project: "group/project", IID: 7, Discussions: []MRDiscussion{{ID: "thread-1", Resolvable: true}}}},
		{http.MethodPost, "/approvals", mockApprovalRequest{Project: "group/project", IID: 7, Approvals: []MRApproval{{Username: "alice"}}, Required: 2}},
		{http.MethodPost, "/branches", mockBranchesRequest{Project: "group/project", Branches: []RepoBranch{{Name: "main"}}}},
	}
	for _, item := range requests {
		response := mockControlRequest(t, router, item.method,
			"/api/v1/gitlab/mock"+item.path+"?workspace_id=workspace-a", item.body)
		if response.Code != http.StatusOK {
			t.Fatalf("seed %s status=%d body=%s", item.path, response.Code, response.Body.String())
		}
	}

	if username, err := workspaceA.GetAuthenticatedUser(t.Context()); err != nil || username != "reviewer" {
		t.Fatalf("user = (%q, %v), want reviewer", username, err)
	}
	if state, err := workspaceA.GetIssueState(t.Context(), "group/project", 3); err != nil || state != "opened" {
		t.Fatalf("issue state = (%q, %v), want opened", state, err)
	}
	if jobs, err := workspaceA.ListPipelineJobs(t.Context(), "group/project", 10); err != nil || len(jobs) != 1 || jobs[0].ID != 11 {
		t.Fatalf("jobs = (%#v, %v), want job 11", jobs, err)
	}
	if discussions, err := workspaceA.ListMRDiscussions(t.Context(), "group/project", 7, nil); err != nil || len(discussions) != 1 || discussions[0].ID != "thread-1" {
		t.Fatalf("discussions = (%#v, %v), want thread-1", discussions, err)
	}
	if approvals, err := workspaceA.ListMRApprovals(t.Context(), "group/project", 7); err != nil || len(approvals) != 1 || approvals[0].Username != "alice" {
		t.Fatalf("approvals = (%#v, %v), want alice", approvals, err)
	}
	if branches, err := workspaceA.ListProjectBranches(t.Context(), "group/project"); err != nil || len(branches) != 1 || branches[0].Name != "main" {
		t.Fatalf("branches = (%#v, %v), want main", branches, err)
	}
}

func TestMockControllerSeedRoutesRejectInvalidPayloads(t *testing.T) {
	router, _, _ := newMockControllerFixture(t)
	for _, path := range []string{
		"/mrs", "/issues", "/pipelines", "/pipeline-jobs", "/discussions",
		"/approvals", "/branches", "/members", "/files", "/commits",
	} {
		response := mockControlRequest(t, router, http.MethodPost, "/api/v1/gitlab/mock"+path, map[string]any{})
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s status=%d body=%s, want 400", path, response.Code, response.Body.String())
		}
	}
}
