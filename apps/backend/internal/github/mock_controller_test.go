package github

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func setupMockControllerTestForAddIssues() (*gin.Engine, *MockClient) {
	gin.SetMode(gin.TestMode)
	mock := NewMockClient()
	ctrl := NewMockController(mock, nil, nil, nil, newControllerTestLogger())
	router := gin.New()
	ctrl.RegisterRoutes(router)
	return router, mock
}

func TestMockControllerAddIssues(t *testing.T) {
	router, mock := setupMockControllerTestForAddIssues()
	body := bytes.NewBufferString(`{"issues":[{"number":1456,"title":"Fix picker","repo_owner":"owner","repo_name":"repo"}]}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/mock/issues", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got struct {
		Added int `json:"added"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Added != 1 {
		t.Fatalf("expected added=1, got %d", got.Added)
	}
	issue, err := mock.GetIssue(context.Background(), "owner", "repo", 1456)
	if err != nil {
		t.Fatalf("get seeded issue: %v", err)
	}
	if issue.Title != "Fix picker" {
		t.Fatalf("expected seeded issue title, got %q", issue.Title)
	}
}

func TestMockControllerAddIssuesInvalidPayload(t *testing.T) {
	router, _ := setupMockControllerTestForAddIssues()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/mock/issues", bytes.NewBufferString(`{`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMockControllerAddRepoFiles(t *testing.T) {
	router, mock := setupMockControllerTestForAddIssues()
	body := bytes.NewBufferString(`{
		"owner":"o",
		"repo":"r",
		"ref":"main",
		"files":[{"path":"workflows/deploy.yaml","content":"name: deploy"}]
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/mock/repo-files", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got struct {
		Added int `json:"added"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Added != 1 {
		t.Fatalf("expected added=1, got %d", got.Added)
	}
	content, err := mock.GetRepoFileContent(context.Background(), "o", "r", "workflows/deploy.yaml", "main")
	if err != nil {
		t.Fatalf("get seeded repo file: %v", err)
	}
	if string(content) != "name: deploy" {
		t.Fatalf("expected seeded content, got %q", content)
	}
}

func TestMockControllerAddRepoFilesInvalidPayload(t *testing.T) {
	router, _ := setupMockControllerTestForAddIssues()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/mock/repo-files", bytes.NewBufferString(`{`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMockControllerAddRepoFilesRequiresOwnerAndRepo(t *testing.T) {
	router, _ := setupMockControllerTestForAddIssues()
	body := bytes.NewBufferString(`{"files":[{"path":"a.yaml","content":"x"}]}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/mock/repo-files", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestEnsureMockPRForRequestCopiesMergeableState(t *testing.T) {
	mock := NewMockClient()
	controller := &MockController{mock: mock}
	req := &associateTaskPRRequest{
		Owner:          "testorg",
		Repo:           "testrepo",
		PRNumber:       102,
		PRURL:          "https://github.com/testorg/testrepo/pull/102",
		PRTitle:        "Ready to ship",
		HeadBranch:     "feat/ready",
		BaseBranch:     "main",
		AuthorLogin:    "test-user",
		State:          "open",
		MergeableState: "clean",
	}

	controller.ensureMockPRForRequest(context.Background(), req, time.Now().UTC())

	pr, err := mock.GetPR(context.Background(), req.Owner, req.Repo, req.PRNumber)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if pr == nil {
		t.Fatal("expected synthetic PR")
	}
	if pr.MergeableState != "clean" {
		t.Fatalf("MergeableState = %q, want clean", pr.MergeableState)
	}
}
