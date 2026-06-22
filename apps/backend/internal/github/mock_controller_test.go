package github

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupMockControllerTest() (*gin.Engine, *MockClient) {
	gin.SetMode(gin.TestMode)
	mock := NewMockClient()
	ctrl := NewMockController(mock, nil, nil, nil, newControllerTestLogger())
	router := gin.New()
	ctrl.RegisterRoutes(router)
	return router, mock
}

func TestMockControllerAddIssues(t *testing.T) {
	router, mock := setupMockControllerTest()
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
	router, _ := setupMockControllerTest()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/mock/issues", bytes.NewBufferString(`{`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
