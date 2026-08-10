package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// controllerReadStub overrides the read methods the list/search endpoints use.
// The embedded *stubClient supplies no-op defaults for the rest of Client.
type controllerReadStub struct {
	*stubClient
	orgs        []GitHubOrg
	orgsErr     error
	orgRepos    []GitHubRepo
	orgReposErr error
	branches    []RepoBranch
	branchesErr error
	prPage      *PRSearchPage
	prPageErr   error
	issuePage   *IssueSearchPage
	issueErr    error
	feedback    *PRFeedback
	feedbackErr error
	prStatus    *PRStatus
	statusErr   error
}

func newControllerReadStub() *controllerReadStub {
	return &controllerReadStub{stubClient: &stubClient{}}
}

func (s *controllerReadStub) ListUserOrgs(context.Context) ([]GitHubOrg, error) {
	return s.orgs, s.orgsErr
}

func (s *controllerReadStub) SearchOrgRepos(context.Context, string, string, int) ([]GitHubRepo, error) {
	return s.orgRepos, s.orgReposErr
}

func (s *controllerReadStub) ListRepoBranches(context.Context, string, string) ([]RepoBranch, error) {
	return s.branches, s.branchesErr
}

func (s *controllerReadStub) SearchPRsPaged(
	context.Context, string, string, int, int,
) (*PRSearchPage, error) {
	if s.prPageErr != nil {
		return nil, s.prPageErr
	}
	if s.prPage != nil {
		return s.prPage, nil
	}
	return &PRSearchPage{PRs: []*PR{}}, nil
}

func (s *controllerReadStub) ListIssuesPaged(
	context.Context, string, string, int, int,
) (*IssueSearchPage, error) {
	if s.issueErr != nil {
		return nil, s.issueErr
	}
	if s.issuePage != nil {
		return s.issuePage, nil
	}
	return &IssueSearchPage{Issues: []*Issue{}}, nil
}

func (s *controllerReadStub) GetPRFeedback(
	context.Context, string, string, int,
) (*PRFeedback, error) {
	return s.feedback, s.feedbackErr
}

func (s *controllerReadStub) GetPRStatus(context.Context, string, string, int) (*PRStatus, error) {
	return s.prStatus, s.statusErr
}

// doControllerRequest issues a request through the wired router and returns the
// recorder so callers can assert both status and decoded body.
func doControllerRequest(router http.Handler, method, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

// decodeControllerBody decodes a JSON response body into out.
func decodeControllerBody(t *testing.T, response *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), out); err != nil {
		t.Fatalf("decode body %q: %v", response.Body.String(), err)
	}
}

// The authenticated user is prepended as a pseudo-org so the picker can offer
// personal repositories alongside real organizations.
func TestController_ListUserOrgs(t *testing.T) {
	client := newControllerReadStub()
	client.orgs = []GitHubOrg{{Login: "acme", AvatarURL: "https://avatars/acme"}}
	router, _ := setupControllerTest(client)

	response := doControllerRequest(router, http.MethodGet, "/api/v1/github/orgs")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body)
	}
	var body struct {
		Orgs []GitHubOrg `json:"orgs"`
	}
	decodeControllerBody(t, response, &body)
	if len(body.Orgs) != 2 {
		t.Fatalf("orgs = %#v, want the pseudo-org plus acme", body.Orgs)
	}
	if body.Orgs[0] != (GitHubOrg{Login: "test-user"}) {
		t.Errorf("orgs[0] = %#v, want the authenticated user first", body.Orgs[0])
	}
	if body.Orgs[1] != (GitHubOrg{Login: "acme", AvatarURL: "https://avatars/acme"}) {
		t.Errorf("orgs[1] = %#v", body.Orgs[1])
	}
}

func TestController_ListUserOrgs_UpstreamFailureIs500(t *testing.T) {
	client := newControllerReadStub()
	client.orgsErr = &GitHubAPIError{StatusCode: http.StatusUnauthorized, Endpoint: "/user/orgs"}
	router, _ := setupControllerTest(client)

	response := doControllerRequest(router, http.MethodGet, "/api/v1/github/orgs")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %s", response.Code, response.Body)
	}
	var body struct {
		Error string `json:"error"`
	}
	decodeControllerBody(t, response, &body)
	if body.Error == "" {
		t.Error("error body is empty, want the upstream message surfaced")
	}
}

func TestController_SearchRepos(t *testing.T) {
	client := newControllerReadStub()
	client.orgRepos = []GitHubRepo{
		{FullName: "acme/widget", Owner: "acme", Name: "widget", DefaultBranch: "main"},
	}
	router, _ := setupControllerTest(client)

	response := doControllerRequest(router, http.MethodGet, "/api/v1/github/repos/search?org=acme&q=wid")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body)
	}
	var body struct {
		Repos []GitHubRepo `json:"repos"`
	}
	decodeControllerBody(t, response, &body)
	if len(body.Repos) != 1 || body.Repos[0].FullName != "acme/widget" {
		t.Fatalf("repos = %#v", body.Repos)
	}
	if body.Repos[0].DefaultBranch != "main" {
		t.Errorf("default_branch = %q, want main", body.Repos[0].DefaultBranch)
	}
}

func TestController_SearchRepos_OrgIsRequired(t *testing.T) {
	router, _ := setupControllerTest(newControllerReadStub())
	response := doControllerRequest(router, http.MethodGet, "/api/v1/github/repos/search?q=wid")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", response.Code, response.Body)
	}
	var body struct {
		Error string `json:"error"`
	}
	decodeControllerBody(t, response, &body)
	if body.Error != "org query parameter required" {
		t.Errorf("error = %q", body.Error)
	}
}

func TestController_SearchRepos_UpstreamFailureIs500(t *testing.T) {
	client := newControllerReadStub()
	client.orgReposErr = &GitHubAPIError{StatusCode: http.StatusForbidden, Endpoint: "/search/repositories"}
	router, _ := setupControllerTest(client)

	response := doControllerRequest(router, http.MethodGet, "/api/v1/github/repos/search?org=acme")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %s", response.Code, response.Body)
	}
}

func TestController_ListRepoBranches(t *testing.T) {
	client := newControllerReadStub()
	client.branches = []RepoBranch{{Name: "main"}, {Name: "release/1.0"}}
	router, _ := setupControllerTest(client)

	response := doControllerRequest(router, http.MethodGet, "/api/v1/github/repos/acme/widget/branches")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body)
	}
	var body struct {
		Branches []RepoBranch `json:"branches"`
	}
	decodeControllerBody(t, response, &body)
	if len(body.Branches) != 2 || body.Branches[0].Name != "main" ||
		body.Branches[1].Name != "release/1.0" {
		t.Fatalf("branches = %#v", body.Branches)
	}
}

// Upstream status codes must survive to the client so the UI can re-prompt for
// auth on a 401 rather than showing an opaque 500.
func TestController_ListRepoBranches_PreservesUpstreamStatus(t *testing.T) {
	cases := []struct {
		upstream int
		want     int
	}{
		{http.StatusNotFound, http.StatusNotFound},
		{http.StatusUnauthorized, http.StatusUnauthorized},
		{http.StatusForbidden, http.StatusForbidden},
		{http.StatusBadGateway, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.upstream), func(t *testing.T) {
			client := newControllerReadStub()
			client.branchesErr = &GitHubAPIError{
				StatusCode: tc.upstream, Endpoint: "/repos/acme/widget/branches", Body: "upstream said no",
			}
			router, _ := setupControllerTest(client)
			response := doControllerRequest(
				router, http.MethodGet, "/api/v1/github/repos/acme/widget/branches",
			)
			if response.Code != tc.want {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, tc.want, response.Body)
			}
		})
	}
}

// GitHub not being configured is its own contract: a 503 carrying a stable
// code the frontend switches on.
func TestController_ListRepoBranches_NotConfiguredIs503WithCode(t *testing.T) {
	router, _ := setupControllerTest(nil)
	response := doControllerRequest(router, http.MethodGet, "/api/v1/github/repos/acme/widget/branches")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body = %s", response.Code, response.Body)
	}
	var body struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	decodeControllerBody(t, response, &body)
	if body.Code != "github_not_configured" {
		t.Errorf("code = %q, want github_not_configured", body.Code)
	}
	if body.Error == "" {
		t.Error("error message is empty, want the remediation hint")
	}
}

func TestController_SearchUserPRs(t *testing.T) {
	client := newControllerReadStub()
	client.prPage = &PRSearchPage{
		PRs:        []*PR{{Number: 7, Title: "Fix auth", State: "open"}},
		TotalCount: 1,
	}
	router, _ := setupControllerTest(client)

	response := doControllerRequest(
		router, http.MethodGet, "/api/v1/github/user/prs?filter=is:open&page=2&per_page=25",
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body)
	}
	var page PRSearchPage
	decodeControllerBody(t, response, &page)
	if len(page.PRs) != 1 || page.PRs[0].Number != 7 || page.PRs[0].Title != "Fix auth" {
		t.Fatalf("prs = %#v", page.PRs)
	}
	if page.Page != 2 || page.PerPage != 25 {
		t.Errorf("page/per_page = (%d, %d), want (2, 25)", page.Page, page.PerPage)
	}
}

// A nil PR slice must serialize as [] rather than null so the frontend can map
// over the result without a guard.
func TestController_SearchUserPRs_NilResultSerializesAsEmptyArray(t *testing.T) {
	client := newControllerReadStub()
	client.prPage = &PRSearchPage{PRs: nil, TotalCount: 0}
	router, _ := setupControllerTest(client)

	response := doControllerRequest(router, http.MethodGet, "/api/v1/github/user/prs")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body)
	}
	var raw map[string]json.RawMessage
	decodeControllerBody(t, response, &raw)
	if string(raw["prs"]) != "[]" {
		t.Errorf("prs = %s, want []", raw["prs"])
	}
}

func TestController_SearchUserIssues(t *testing.T) {
	client := newControllerReadStub()
	client.issuePage = &IssueSearchPage{
		Issues:     []*Issue{{Number: 3, Title: "Broken login", State: "open"}},
		TotalCount: 1,
	}
	router, _ := setupControllerTest(client)

	response := doControllerRequest(router, http.MethodGet, "/api/v1/github/user/issues?query=is:issue")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body)
	}
	var page IssueSearchPage
	decodeControllerBody(t, response, &page)
	if len(page.Issues) != 1 || page.Issues[0].Number != 3 || page.Issues[0].Title != "Broken login" {
		t.Fatalf("issues = %#v", page.Issues)
	}
	// Defaults apply when the caller sends no pagination params.
	if page.Page != 1 || page.PerPage != 50 {
		t.Errorf("page/per_page = (%d, %d), want the (1, 50) defaults", page.Page, page.PerPage)
	}
}

func TestController_SearchUserIssues_NilResultSerializesAsEmptyArray(t *testing.T) {
	client := newControllerReadStub()
	client.issuePage = &IssueSearchPage{Issues: nil}
	router, _ := setupControllerTest(client)

	response := doControllerRequest(router, http.MethodGet, "/api/v1/github/user/issues")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body)
	}
	var raw map[string]json.RawMessage
	decodeControllerBody(t, response, &raw)
	if string(raw["issues"]) != "[]" {
		t.Errorf("issues = %s, want []", raw["issues"])
	}
}

// Both search endpoints route their failures through handleSearchError, which
// forwards the recognised upstream statuses and collapses everything else.
func TestController_SearchEndpoints_MapUpstreamStatuses(t *testing.T) {
	cases := []struct {
		upstream int
		want     int
	}{
		{http.StatusNotFound, http.StatusNotFound},
		{http.StatusUnauthorized, http.StatusUnauthorized},
		{http.StatusForbidden, http.StatusForbidden},
		{http.StatusTeapot, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.upstream), func(t *testing.T) {
			apiErr := &GitHubAPIError{StatusCode: tc.upstream, Endpoint: "/search/issues"}
			client := newControllerReadStub()
			client.prPageErr = apiErr
			client.issueErr = apiErr
			router, _ := setupControllerTest(client)

			for _, path := range []string{"/api/v1/github/user/prs", "/api/v1/github/user/issues"} {
				response := doControllerRequest(router, http.MethodGet, path)
				if response.Code != tc.want {
					t.Errorf("%s status = %d, want %d; body = %s",
						path, response.Code, tc.want, response.Body)
				}
			}
		})
	}
}

func TestController_SearchEndpoints_NotConfiguredIs503WithCode(t *testing.T) {
	router, _ := setupControllerTest(nil)
	for _, path := range []string{"/api/v1/github/user/prs", "/api/v1/github/user/issues"} {
		response := doControllerRequest(router, http.MethodGet, path)
		if response.Code != http.StatusServiceUnavailable {
			t.Errorf("%s status = %d, want 503; body = %s", path, response.Code, response.Body)
			continue
		}
		var body struct {
			Code string `json:"code"`
		}
		decodeControllerBody(t, response, &body)
		if body.Code != "github_not_configured" {
			t.Errorf("%s code = %q, want github_not_configured", path, body.Code)
		}
	}
}

func TestController_GetPRFeedback(t *testing.T) {
	client := newControllerReadStub()
	client.feedback = &PRFeedback{
		PR:        &PR{Number: 42, Title: "Add widgets", State: "open"},
		Reviews:   []PRReview{{ID: 1, Author: "alice", State: "APPROVED"}},
		Comments:  []PRComment{},
		Checks:    []CheckRun{{Name: "unit", Status: "completed", Conclusion: "failure"}},
		HasIssues: true,
	}
	router, _ := setupControllerTest(client)

	response := doControllerRequest(router, http.MethodGet, "/api/v1/github/prs/acme/widget/42")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body)
	}
	var body PRFeedback
	decodeControllerBody(t, response, &body)
	if body.PR == nil || body.PR.Number != 42 || body.PR.Title != "Add widgets" {
		t.Fatalf("PR = %#v", body.PR)
	}
	if len(body.Reviews) != 1 || body.Reviews[0].Author != "alice" {
		t.Errorf("reviews = %#v", body.Reviews)
	}
	if len(body.Checks) != 1 || body.Checks[0].Conclusion != "failure" {
		t.Errorf("checks = %#v", body.Checks)
	}
	if !body.HasIssues {
		t.Error("has_issues = false, want true")
	}
}

func TestController_GetPRFeedback_RejectsANonNumericPRNumber(t *testing.T) {
	router, _ := setupControllerTest(newControllerReadStub())
	response := doControllerRequest(router, http.MethodGet, "/api/v1/github/prs/acme/widget/abc")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", response.Code, response.Body)
	}
	var body struct {
		Error string `json:"error"`
	}
	decodeControllerBody(t, response, &body)
	if body.Error != "invalid PR number" {
		t.Errorf("error = %q", body.Error)
	}
}

func TestController_GetPRFeedback_UpstreamFailureIs500(t *testing.T) {
	client := newControllerReadStub()
	client.feedbackErr = &GitHubAPIError{StatusCode: http.StatusNotFound, Endpoint: "/repos/acme/widget/pulls/42"}
	router, _ := setupControllerTest(client)

	response := doControllerRequest(router, http.MethodGet, "/api/v1/github/prs/acme/widget/42")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %s", response.Code, response.Body)
	}
}

func TestController_GetPRStatus(t *testing.T) {
	client := newControllerReadStub()
	client.prStatus = &PRStatus{
		PR:              &PR{Number: 42, State: "open"},
		ReviewState:     computedReviewStateApproved,
		ChecksState:     "failure",
		MergeableState:  "blocked",
		ReviewCount:     2,
		ChecksTotal:     3,
		ChecksPassing:   2,
		ChecksPopulated: true,
	}
	router, _ := setupControllerTest(client)

	response := doControllerRequest(router, http.MethodGet, "/api/v1/github/prs/acme/widget/42/status")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body)
	}
	var body PRStatus
	decodeControllerBody(t, response, &body)
	if body.PR == nil || body.PR.Number != 42 {
		t.Fatalf("PR = %#v", body.PR)
	}
	if body.ReviewState != computedReviewStateApproved || body.ChecksState != "failure" {
		t.Errorf("states = (%q, %q)", body.ReviewState, body.ChecksState)
	}
	if body.MergeableState != "blocked" || body.ReviewCount != 2 {
		t.Errorf("mergeable/review count = (%q, %d)", body.MergeableState, body.ReviewCount)
	}
	if body.ChecksTotal != 3 || body.ChecksPassing != 2 || !body.ChecksPopulated {
		t.Errorf("checks = %d/%d populated=%v", body.ChecksPassing, body.ChecksTotal, body.ChecksPopulated)
	}
}

func TestController_GetPRStatus_RejectsANonNumericPRNumber(t *testing.T) {
	router, _ := setupControllerTest(newControllerReadStub())
	response := doControllerRequest(router, http.MethodGet, "/api/v1/github/prs/acme/widget/xyz/status")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", response.Code, response.Body)
	}
}

// httpGetPRStatus goes through handleSearchError, so unlike the feedback
// endpoint it forwards the recognised upstream statuses.
func TestController_GetPRStatus_PreservesUpstreamStatus(t *testing.T) {
	client := newControllerReadStub()
	client.statusErr = &GitHubAPIError{
		StatusCode: http.StatusNotFound, Endpoint: "/repos/acme/widget/pulls/42",
	}
	router, _ := setupControllerTest(client)

	response := doControllerRequest(router, http.MethodGet, "/api/v1/github/prs/acme/widget/42/status")
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", response.Code, response.Body)
	}
}

func TestController_GetStats_RejectsMalformedDates(t *testing.T) {
	cases := []struct {
		name   string
		target string
		want   string
	}{
		{
			"bad start_date", "/api/v1/github/stats?start_date=01-02-2026",
			"invalid start_date format, expected YYYY-MM-DD",
		},
		{
			"bad end_date", "/api/v1/github/stats?start_date=2026-01-01&end_date=nope",
			"invalid end_date format, expected YYYY-MM-DD",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router, _ := setupControllerTest(newControllerReadStub())
			response := doControllerRequest(router, http.MethodGet, tc.target)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body = %s", response.Code, response.Body)
			}
			var body struct {
				Error string `json:"error"`
			}
			decodeControllerBody(t, response, &body)
			if body.Error != tc.want {
				t.Errorf("error = %q, want %q", body.Error, tc.want)
			}
		})
	}
}

func TestController_GetWorkspaceSettings_RequiresAWorkspace(t *testing.T) {
	router, _ := setupControllerTest(newControllerReadStub())
	request := httptest.NewRequest(http.MethodGet, "/api/v1/github/workspace-settings", nil)
	request.Header.Set("X-Test-Omit-Workspace", "1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", response.Code, response.Body)
	}
	var body struct {
		Error string `json:"error"`
	}
	decodeControllerBody(t, response, &body)
	if body.Error != "workspace_id query parameter required" {
		t.Errorf("error = %q", body.Error)
	}
}

func TestController_ClearToken_RequiresAWorkspace(t *testing.T) {
	router, _ := setupControllerTest(newControllerReadStub())
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/github/token", nil)
	request.Header.Set("X-Test-Omit-Workspace", "1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code == http.StatusOK {
		t.Fatalf("status = %d, want a failure when workspace_id is missing; body = %s",
			response.Code, response.Body)
	}
}

func TestController_GetStatus_RequiresAWorkspace(t *testing.T) {
	router, _ := setupControllerTest(newControllerReadStub())
	request := httptest.NewRequest(http.MethodGet, "/api/v1/github/status", nil)
	request.Header.Set("X-Test-Omit-Workspace", "1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code == http.StatusOK {
		t.Fatalf("status = %d, want a failure when workspace_id is missing; body = %s",
			response.Code, response.Body)
	}
}

// Guard the pagination defaults independently of any one endpoint: page floors
// at 1 and per_page falls back to 50, so a malformed query can never ask GitHub
// for page 0 or an empty page.
func TestParsePaginationQuery(t *testing.T) {
	cases := []struct {
		name        string
		query       string
		wantPage    int
		wantPerPage int
	}{
		{"defaults", "", 1, 50},
		{"explicit values", "?page=3&per_page=25", 3, 25},
		{"zero page floors to 1", "?page=0&per_page=25", 1, 25},
		{"negative page floors to 1", "?page=-4&per_page=25", 1, 25},
		{"zero per_page falls back", "?page=2&per_page=0", 2, 50},
		{"negative per_page falls back", "?page=2&per_page=-1", 2, 50},
		{"non-numeric values fall back", "?page=abc&per_page=xyz", 1, 50},
		// The 100 cap lives in clampSearchPage, not here.
		{"large per_page passes through", "?page=1&per_page=5000", 1, 5000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newPaginationGinContext(t, tc.query)
			page, perPage := parsePaginationQuery(ctx)
			if page != tc.wantPage || perPage != tc.wantPerPage {
				t.Errorf("parsePaginationQuery(%q) = (%d, %d), want (%d, %d)",
					tc.query, page, perPage, tc.wantPage, tc.wantPerPage)
			}
		})
	}
}

func TestController_ListUserOrgs_ContextCancellationPropagates(t *testing.T) {
	client := newControllerReadStub()
	client.orgsErr = context.Canceled
	router, _ := setupControllerTest(client)

	response := doControllerRequest(router, http.MethodGet, "/api/v1/github/orgs")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %s", response.Code, response.Body)
	}
}

func TestController_SearchRepos_HonoursTheRequestDeadline(t *testing.T) {
	client := newControllerReadStub()
	client.orgReposErr = context.DeadlineExceeded
	router, _ := setupControllerTest(client)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/github/repos/search?org=acme", nil)
	ctx, cancel := context.WithTimeout(request.Context(), time.Millisecond)
	defer cancel()
	request = request.WithContext(ctx)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %s", response.Code, response.Body)
	}
}

// newPaginationGinContext builds a bare gin context carrying only the query
// string, so parsePaginationQuery can be exercised without a route.
func newPaginationGinContext(t *testing.T, query string) *gin.Context {
	t.Helper()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/search"+query, nil)
	return ctx
}
