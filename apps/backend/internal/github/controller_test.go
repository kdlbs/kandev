package github

import (
	"bytes"
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
	getPRFunc             func(ctx context.Context, owner, repo string, number int) (*PR, error)
	mergePRFn             func(ctx context.Context, owner, repo string, number int, mergeMethod string) error
	getRepoMergeMethodsFn func() (RepoMergeMethods, error)
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
func (s *stubClient) SearchPRs(context.Context, string, string) ([]*PR, error) {
	return nil, nil
}
func (s *stubClient) SearchPRsPaged(context.Context, string, string, int, int) (*PRSearchPage, error) {
	return &PRSearchPage{PRs: []*PR{}}, nil
}
func (s *stubClient) ListReviewRequestedPRs(context.Context, string, string, string) ([]*PR, error) {
	return nil, nil
}
func (s *stubClient) ListUserOrgs(context.Context) ([]GitHubOrg, error) { return nil, nil }
func (s *stubClient) SearchOrgRepos(context.Context, string, string, int) ([]GitHubRepo, error) {
	return nil, nil
}
func (s *stubClient) ListUserRepos(context.Context, string, int) ([]GitHubRepo, error) {
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
func (s *stubClient) MergePR(ctx context.Context, owner, repo string, number int, mergeMethod string) error {
	if s.mergePRFn != nil {
		return s.mergePRFn(ctx, owner, repo, number, mergeMethod)
	}
	return nil
}
func (s *stubClient) ListRepoBranches(context.Context, string, string) ([]RepoBranch, error) {
	return nil, nil
}
func (s *stubClient) GetRepoMergeMethods(context.Context, string, string) (RepoMergeMethods, error) {
	if s.getRepoMergeMethodsFn != nil {
		return s.getRepoMergeMethodsFn()
	}
	return RepoMergeMethods{Merge: true, Squash: true, Rebase: true}, nil
}
func (s *stubClient) ListIssues(context.Context, string, string) ([]*Issue, error) {
	return nil, nil
}
func (s *stubClient) ListIssuesPaged(context.Context, string, string, int, int) (*IssueSearchPage, error) {
	return &IssueSearchPage{Issues: []*Issue{}}, nil
}
func (s *stubClient) GetIssueState(context.Context, string, string, int) (string, error) {
	return defaultPRState, nil
}
func (s *stubClient) GetPRStatus(context.Context, string, string, int) (*PRStatus, error) {
	return nil, nil
}
func (s *stubClient) CreateGist(context.Context, CreateGistInput) (*GistResponse, error) {
	return nil, nil
}
func (s *stubClient) DeleteGist(context.Context, string) error { return nil }

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

func TestHttpMergePR_Success(t *testing.T) {
	var called struct {
		owner       string
		repo        string
		number      int
		mergeMethod string
	}
	sc := &stubClient{
		mergePRFn: func(_ context.Context, owner, repo string, number int, mergeMethod string) error {
			called.owner = owner
			called.repo = repo
			called.number = number
			called.mergeMethod = mergeMethod
			return nil
		},
	}
	router, _ := setupControllerTest(sc)

	body := bytes.NewBufferString(`{"merge_method":"squash"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/github/prs/acme/widget/42/merge", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if called.owner != "acme" || called.repo != "widget" || called.number != 42 || called.mergeMethod != "squash" {
		t.Errorf("unexpected MergePR args: %+v", called)
	}
}

func TestHttpMergePR_InvalidMethod(t *testing.T) {
	router, _ := setupControllerTest(&stubClient{})

	body := bytes.NewBufferString(`{"merge_method":"bogus"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/github/prs/acme/widget/42/merge", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHttpMergePR_EmptyBody_ResolvesToAllowedMethod(t *testing.T) {
	// Empty merge_method must NOT propagate to GitHub — that would let
	// GitHub default to "merge" and 405 on squash-only repos. The service
	// should resolve to the first allowed method instead.
	var gotMethod string
	sc := &stubClient{
		mergePRFn: func(_ context.Context, _, _ string, _ int, mergeMethod string) error {
			gotMethod = mergeMethod
			return nil
		},
		getRepoMergeMethodsFn: func() (RepoMergeMethods, error) {
			// Squash-only repo (the case that surfaced this bug).
			return RepoMergeMethods{Squash: true}, nil
		},
	}
	router, _ := setupControllerTest(sc)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/github/prs/acme/widget/42/merge", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotMethod != "squash" {
		t.Errorf("expected merge_method=squash, got %q", gotMethod)
	}
}

func TestHttpMergePR_ExplicitMethod_Passthrough(t *testing.T) {
	// When the user picks a method from the dropdown, the service must
	// NOT second-guess it via the repo lookup.
	var gotMethod string
	var lookupCalls int
	sc := &stubClient{
		mergePRFn: func(_ context.Context, _, _ string, _ int, mergeMethod string) error {
			gotMethod = mergeMethod
			return nil
		},
		getRepoMergeMethodsFn: func() (RepoMergeMethods, error) {
			lookupCalls++
			return RepoMergeMethods{Merge: true}, nil
		},
	}
	router, _ := setupControllerTest(sc)

	body := bytes.NewBufferString(`{"merge_method":"rebase"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/github/prs/acme/widget/42/merge", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotMethod != "rebase" {
		t.Errorf("expected merge_method=rebase, got %q", gotMethod)
	}
	if lookupCalls != 0 {
		t.Errorf("expected no merge-methods lookup for explicit pin, got %d", lookupCalls)
	}
}

func TestHttpMergePR_EmptyBody_LookupFails_FallsBackToGitHubDefault(t *testing.T) {
	// If the repo lookup itself errors, we still attempt the merge with an
	// empty method and let GitHub surface whatever error it surfaces — better
	// than refusing to merge because of an unrelated lookup failure.
	var gotMethod string
	sc := &stubClient{
		mergePRFn: func(_ context.Context, _, _ string, _ int, mergeMethod string) error {
			gotMethod = mergeMethod
			return nil
		},
		getRepoMergeMethodsFn: func() (RepoMergeMethods, error) {
			return RepoMergeMethods{}, fmt.Errorf("rate limited")
		},
	}
	router, _ := setupControllerTest(sc)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/github/prs/acme/widget/42/merge", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotMethod != "" {
		t.Errorf("expected empty merge_method (fall back to GitHub default), got %q", gotMethod)
	}
}

func TestHttpGetRepoMergeMethods_OK(t *testing.T) {
	sc := &stubClient{
		getRepoMergeMethodsFn: func() (RepoMergeMethods, error) {
			return RepoMergeMethods{Squash: true, Rebase: true}, nil
		},
	}
	router, _ := setupControllerTest(sc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/repos/acme/widget/merge-methods", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got RepoMergeMethods
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := RepoMergeMethods{Squash: true, Rebase: true}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestHttpGetRepoMergeMethods_NoClient_Returns503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newControllerTestLogger()
	// nil client triggers ErrNoClient via Service.GetRepoMergeMethods.
	svc := NewService(nil, "none", nil, nil, nil, log)
	ctrl := NewController(svc, log)
	router := gin.New()
	ctrl.RegisterHTTPRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/repos/acme/widget/merge-methods", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHttpGetRepoMergeMethods_NotFound_Returns404(t *testing.T) {
	sc := &stubClient{
		getRepoMergeMethodsFn: func() (RepoMergeMethods, error) {
			return RepoMergeMethods{}, &GitHubAPIError{StatusCode: http.StatusNotFound, Endpoint: "/repos/acme/widget"}
		},
	}
	router, _ := setupControllerTest(sc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/repos/acme/widget/merge-methods", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHttpGetRepoMergeMethods_OtherError_Returns500(t *testing.T) {
	sc := &stubClient{
		getRepoMergeMethodsFn: func() (RepoMergeMethods, error) {
			return RepoMergeMethods{}, fmt.Errorf("unexpected upstream failure")
		},
	}
	router, _ := setupControllerTest(sc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/repos/acme/widget/merge-methods", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHttpMergePR_MalformedJSON(t *testing.T) {
	router, _ := setupControllerTest(&stubClient{})

	// Truncated JSON: parser fails with a non-EOF error, which must now
	// surface as 400 rather than silently falling through to the default
	// merge method.
	body := bytes.NewBufferString(`{"merge_method":"squash"`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/github/prs/acme/widget/42/merge", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHttpMergePR_NoClient_Returns503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newControllerTestLogger()
	// nil client triggers ErrNoClient via Service.MergePR.
	svc := NewService(nil, "none", nil, nil, nil, log)
	ctrl := NewController(svc, log)
	router := gin.New()
	ctrl.RegisterHTTPRoutes(router)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/github/prs/acme/widget/42/merge", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHttpSubmitReview_SelfApproveReturns422 exercises the full controller →
// service guard → HTTP mapping for the ErrSelfApprove path. Guards against a
// future refactor accidentally reclassifying the typed error as 500, which
// would mask "you can't approve your own PR" as an opaque upstream failure.
func TestHttpSubmitReview_SelfApproveReturns422(t *testing.T) {
	sc := &stubClient{
		// stubClient.GetAuthenticatedUser returns "test-user"; matching
		// AuthorLogin triggers the self-approve guard inside SubmitReview.
		getPRFunc: func(_ context.Context, _, _ string, _ int) (*PR, error) {
			return &PR{Number: 42, AuthorLogin: "test-user", State: "open"}, nil
		},
	}
	router, _ := setupControllerTest(sc)

	body := bytes.NewBufferString(`{"event":"APPROVE"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/prs/acme/widget/42/reviews", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp.Error != ErrSelfApprove.Error() {
		t.Errorf("expected error %q, got %q", ErrSelfApprove.Error(), resp.Error)
	}
}

func TestHttpMergePR_Conflict(t *testing.T) {
	sc := &stubClient{
		mergePRFn: func(context.Context, string, string, int, string) error {
			return &GitHubAPIError{StatusCode: http.StatusMethodNotAllowed, Endpoint: "/merge", Body: "not mergeable"}
		},
	}
	router, _ := setupControllerTest(sc)

	body := bytes.NewBufferString(`{"merge_method":"merge"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/github/prs/acme/widget/42/merge", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}
