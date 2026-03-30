package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
)

// stubClient implements Client with no-op defaults; override fields as needed.
type stubClient struct {
	getPRFunc func(ctx context.Context, owner, repo string, number int) (*PR, error)
}

func (s *stubClient) IsAuthenticated(context.Context) (bool, error) { return true, nil }
func (s *stubClient) GetAuthenticatedUser(context.Context) (string, error) {
	return "test-user", nil
}
func (s *stubClient) GetPR(ctx context.Context, owner, repo string, number int) (*PR, error) {
	if s.getPRFunc != nil {
		return s.getPRFunc(ctx, owner, repo, number)
	}
	return nil, fmt.Errorf("not implemented")
}
func (s *stubClient) FindPRByBranch(context.Context, string, string, string) (*PR, error) {
	return nil, nil
}
func (s *stubClient) ListAuthoredPRs(context.Context, string, string) ([]*PR, error) {
	return nil, nil
}
func (s *stubClient) ListReviewRequestedPRs(context.Context, string, string, string) ([]*PR, error) {
	return nil, nil
}
func (s *stubClient) ListUserOrgs(context.Context) ([]GitHubOrg, error) { return nil, nil }
func (s *stubClient) SearchOrgRepos(context.Context, string, string, int) ([]GitHubRepo, error) {
	return nil, nil
}
func (s *stubClient) ListPRReviews(context.Context, string, string, int) ([]PRReview, error) {
	return nil, nil
}
func (s *stubClient) ListPRComments(context.Context, string, string, int, *time.Time) ([]PRComment, error) {
	return nil, nil
}
func (s *stubClient) ListCheckRuns(context.Context, string, string, string) ([]CheckRun, error) {
	return nil, nil
}
func (s *stubClient) GetPRFeedback(context.Context, string, string, int) (*PRFeedback, error) {
	return nil, nil
}
func (s *stubClient) ListPRFiles(context.Context, string, string, int) ([]PRFile, error) {
	return nil, nil
}
func (s *stubClient) ListPRCommits(context.Context, string, string, int) ([]PRCommitInfo, error) {
	return nil, nil
}
func (s *stubClient) SubmitReview(context.Context, string, string, int, string, string) error {
	return nil
}
func (s *stubClient) ListRepoBranches(context.Context, string, string) ([]RepoBranch, error) {
	return nil, nil
}
func (s *stubClient) GetPRStatus(context.Context, string, string, int) (*PRStatus, error) {
	return nil, nil
}

func newControllerTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	return log
}

func setupControllerTest(client Client) (*gin.Engine, *Controller) {
	gin.SetMode(gin.TestMode)
	log := newControllerTestLogger()
	svc := NewService(client, "pat", nil, nil, nil, log)
	ctrl := NewController(svc, log)
	router := gin.New()
	ctrl.RegisterHTTPRoutes(router)
	return router, ctrl
}

func TestHttpGetPRInfo_Success(t *testing.T) {
	sc := &stubClient{
		getPRFunc: func(_ context.Context, owner, repo string, number int) (*PR, error) {
			if owner != "acme" || repo != "widget" || number != 42 {
				t.Errorf("unexpected params: %s/%s#%d", owner, repo, number)
			}
			return &PR{Number: 42, Title: "feat: add widget"}, nil
		},
	}
	router, _ := setupControllerTest(sc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/prs/acme/widget/42/info", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var pr PR
	if err := json.NewDecoder(w.Body).Decode(&pr); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if pr.Number != 42 {
		t.Errorf("expected PR number 42, got %d", pr.Number)
	}
	if pr.Title != "feat: add widget" {
		t.Errorf("expected title 'feat: add widget', got %q", pr.Title)
	}
}

func TestHttpGetPRInfo_InvalidNumber(t *testing.T) {
	router, _ := setupControllerTest(&stubClient{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/prs/acme/widget/abc/info", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHttpGetPRInfo_ServiceError(t *testing.T) {
	sc := &stubClient{
		getPRFunc: func(context.Context, string, string, int) (*PR, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	router, _ := setupControllerTest(sc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/prs/acme/widget/99/info", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
