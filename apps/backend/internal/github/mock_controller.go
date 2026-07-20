package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

const defaultPRState = "open"

// MockController handles HTTP endpoints for controlling the MockClient in E2E tests.
type MockController struct {
	mock     *MockClient
	auth     *MockAuthState
	store    *Store
	eventBus bus.EventBus
	logger   *logger.Logger
	service  *Service
}

// NewMockController creates a new MockController.
func NewMockController(mock *MockClient, store *Store, eventBus bus.EventBus, svc *Service, log *logger.Logger) *MockController {
	auth := NewMockAuthState(mock)
	if svc != nil {
		auth = svc.enableMockAuth(mock)
	}
	return &MockController{mock: mock, auth: auth, store: store, eventBus: eventBus, service: svc, logger: log}
}

// RegisterRoutes registers all mock control HTTP routes.
func (c *MockController) RegisterRoutes(router *gin.Engine) {
	api := router.Group("/api/v1/github/mock")
	api.PUT("/user", c.setUser)
	api.POST("/prs", c.addPRs)
	api.POST("/issues", c.addIssues)
	api.POST("/orgs", c.addOrgs)
	api.POST("/repos", c.addRepos)
	api.POST("/reviews", c.addReviews)
	api.POST("/comments", c.addComments)
	api.POST("/checks", c.addCheckRuns)
	api.POST("/files", c.addPRFiles)
	api.POST("/commits", c.addPRCommits)
	api.POST("/branches", c.addBranches)
	api.POST("/repo-files", c.addRepoFiles)
	api.POST("/task-prs", c.associateTaskPR)
	api.POST("/pr-feedback", c.seedPRFeedback)
	api.PUT("/auth-health", c.setAuthHealth)
	api.PUT("/workspace-connections/:workspaceId", c.setWorkspaceConnection)
	api.DELETE("/workspace-connections/:workspaceId", c.deleteWorkspaceConnection)
	api.PUT("/workspace-connections/:workspaceId/status", c.setWorkspaceConnectionStatus)
	api.PUT("/personal-connections/:workspaceId", c.setPersonalConnection)
	api.DELETE("/personal-connections/:workspaceId", c.deletePersonalConnection)
	api.PUT("/cli-accounts", c.setCLIAccounts)
	api.PUT("/app-available", c.setAppAvailable)
	api.PUT("/deployment-app-registration", c.setDeploymentAppRegistration)
	api.PUT("/repos-unavailable", c.setReposUnavailable)
	api.DELETE("/reset", c.reset)
}

type mockWorkspaceConnectionRequest struct {
	Source                   ConnectionSource             `json:"source"`
	Status                   ConnectionStatus             `json:"status"`
	Login                    string                       `json:"login"`
	InstallationID           *int64                       `json:"installation_id"`
	InstallationAccountLogin string                       `json:"installation_account_login"`
	InstallationAccountType  string                       `json:"installation_account_type"`
	Capabilities             map[GitHubAppCapability]bool `json:"capabilities"`
}

func (c *MockController) setWorkspaceConnection(ctx *gin.Context) {
	if c.store == nil || c.service == nil || c.auth == nil {
		ctx.JSON(http.StatusConflict, gin.H{errKey: "mock GitHub auth is unavailable"})
		return
	}
	workspaceID := strings.TrimSpace(ctx.Param("workspaceId"))
	var req mockWorkspaceConnectionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondInvalidPayload(ctx)
		return
	}
	if req.Status == "" {
		req.Status = ConnectionStatusActive
	}
	if err := validateMockWorkspaceConnectionRequest(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{errKey: err.Error()})
		return
	}
	existing, err := c.store.GetWorkspaceConnection(ctx.Request.Context(), workspaceID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
		return
	}
	connection := &WorkspaceConnection{
		WorkspaceID: workspaceID, Source: req.Source, GitHubHost: defaultGitHubHost,
		Login: req.Login, InstallationID: req.InstallationID,
		InstallationAccountLogin: req.InstallationAccountLogin,
		InstallationAccountType:  req.InstallationAccountType,
		Status:                   req.Status, CredentialGeneration: nextCredentialGeneration(existing),
	}
	if err := c.store.UpsertWorkspaceConnection(ctx.Request.Context(), connection); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
		return
	}
	if req.Capabilities == nil {
		c.auth.DeleteWorkspace(workspaceID)
	} else {
		c.auth.SetWorkspaceCapabilities(workspaceID, req.Capabilities)
	}
	c.service.invalidateWorkspaceCredential(workspaceID)
	ctx.JSON(http.StatusOK, gin.H{"connection": connection, "capabilities": c.auth.workspaceCapabilities(workspaceID)})
}

func validateMockWorkspaceConnectionRequest(req *mockWorkspaceConnectionRequest) error {
	if req == nil || !validMockConnectionSource(req.Source) || !validMockConnectionStatus(req.Status) {
		return errors.New(errMsgInvalidPayload)
	}
	switch req.Source {
	case ConnectionSourceGitHubAppInstallation:
		if req.InstallationID == nil || *req.InstallationID <= 0 {
			return errors.New("installation_id is required for GitHub App connections")
		}
		if strings.TrimSpace(req.InstallationAccountLogin) == "" {
			return errors.New("installation_account_login is required for GitHub App connections")
		}
		if req.InstallationAccountType == "" {
			req.InstallationAccountType = "Organization"
		}
	case ConnectionSourcePAT, ConnectionSourceGHCLI:
		if strings.TrimSpace(req.Login) == "" {
			return errors.New("login is required for PAT and CLI connections")
		}
	}
	return nil
}

func validMockConnectionSource(source ConnectionSource) bool {
	switch source {
	case ConnectionSourcePAT, ConnectionSourceGHCLI, ConnectionSourceGitHubAppInstallation,
		ConnectionSourceLegacyShared:
		return true
	default:
		return false
	}
}

func validMockConnectionStatus(status ConnectionStatus) bool {
	switch status {
	case "", ConnectionStatusActive, ConnectionStatusInvalid, ConnectionStatusSuspended, ConnectionStatusRevoked:
		return true
	default:
		return false
	}
}

func (c *MockController) deleteWorkspaceConnection(ctx *gin.Context) {
	workspaceID := strings.TrimSpace(ctx.Param("workspaceId"))
	if c.service == nil {
		ctx.JSON(http.StatusConflict, gin.H{errKey: "mock GitHub auth is unavailable"})
		return
	}
	if err := c.service.DeleteWorkspaceConnection(ctx.Request.Context(), workspaceID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
		return
	}
	c.auth.DeleteWorkspace(workspaceID)
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (c *MockController) setWorkspaceConnectionStatus(ctx *gin.Context) {
	var req struct {
		Status ConnectionStatus `json:"status"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Status == "" || !validMockConnectionStatus(req.Status) {
		respondInvalidPayload(ctx)
		return
	}
	workspaceID := strings.TrimSpace(ctx.Param("workspaceId"))
	connection, err := c.store.GetWorkspaceConnection(ctx.Request.Context(), workspaceID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
		return
	}
	if connection == nil {
		ctx.JSON(http.StatusNotFound, gin.H{errKey: "workspace connection not found"})
		return
	}
	connection.Status = req.Status
	connection.CredentialGeneration++
	if err := c.store.UpsertWorkspaceConnection(ctx.Request.Context(), connection); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
		return
	}
	c.service.invalidateWorkspaceCredential(workspaceID)
	ctx.JSON(http.StatusOK, connection)
}

func (c *MockController) setPersonalConnection(ctx *gin.Context) {
	var req struct {
		Login           string           `json:"login"`
		Status          ConnectionStatus `json:"status"`
		GitHubUserID    int64            `json:"github_user_id"`
		AccessExpiresAt time.Time        `json:"access_expires_at"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Login) == "" ||
		req.GitHubUserID <= 0 || req.AccessExpiresAt.IsZero() ||
		(req.Status != ConnectionStatusActive && req.Status != ConnectionStatusInvalid && req.Status != ConnectionStatusRevoked) {
		respondInvalidPayload(ctx)
		return
	}
	workspaceID := strings.TrimSpace(ctx.Param("workspaceId"))
	existing, err := c.store.GetUserConnection(ctx.Request.Context(), workspaceID, DefaultUserID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
		return
	}
	generation := int64(1)
	if existing != nil {
		generation = existing.CredentialGeneration + 1
	}
	connection := &UserConnection{
		WorkspaceID: workspaceID, UserID: DefaultUserID, Login: req.Login,
		GitHubUserID: req.GitHubUserID, Status: req.Status, AccessExpiresAt: req.AccessExpiresAt,
		CredentialGeneration: generation,
	}
	if err := c.store.UpsertUserConnection(ctx.Request.Context(), connection); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
		return
	}
	c.service.invalidateWorkspaceCredential(workspaceID)
	ctx.JSON(http.StatusOK, connection)
}

func (c *MockController) deletePersonalConnection(ctx *gin.Context) {
	workspaceID := strings.TrimSpace(ctx.Param("workspaceId"))
	if err := c.store.DeleteUserConnection(ctx.Request.Context(), workspaceID, DefaultUserID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
		return
	}
	c.service.invalidateWorkspaceCredential(workspaceID)
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (c *MockController) setCLIAccounts(ctx *gin.Context) {
	var req struct {
		Accounts []GHAccount `json:"accounts"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondInvalidPayload(ctx)
		return
	}
	for _, account := range req.Accounts {
		if strings.TrimSpace(account.Host) == "" || strings.TrimSpace(account.Login) == "" {
			respondInvalidPayload(ctx)
			return
		}
	}
	c.auth.SetCLIAccounts(req.Accounts)
	ctx.JSON(http.StatusOK, gin.H{"accounts": req.Accounts})
}

func (c *MockController) setAppAvailable(ctx *gin.Context) {
	var req struct {
		Available *bool `json:"available"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Available == nil {
		respondInvalidPayload(ctx)
		return
	}
	c.service.setMockAppAvailable(*req.Available)
	ctx.JSON(http.StatusOK, gin.H{"available": *req.Available})
}

func (c *MockController) setDeploymentAppRegistration(ctx *gin.Context) {
	var request struct {
		Source            DeploymentAppSource        `json:"source"`
		State             string                     `json:"state"`
		Ready             bool                       `json:"ready"`
		AppID             int64                      `json:"app_id"`
		Slug              string                     `json:"slug"`
		OwnerLogin        string                     `json:"owner_login"`
		OwnerType         DeploymentAppOwnerType     `json:"owner_type"`
		PublicBaseURL     string                     `json:"public_base_url"`
		WebhookStatus     DeploymentAppWebhookStatus `json:"webhook_status"`
		UnavailableCode   string                     `json:"unavailable_code"`
		UnavailableReason string                     `json:"unavailable_reason"`
	}
	if err := ctx.ShouldBindJSON(&request); err != nil ||
		!validMockDeploymentAppState(request.Source, request.State, request.WebhookStatus) {
		respondInvalidPayload(ctx)
		return
	}
	status := DeploymentAppRegistrationStatus{
		Source: request.Source, State: request.State, Ready: request.Ready,
		ReadOnly:        request.Source == DeploymentAppSourceEnvironment,
		UnavailableCode: request.UnavailableCode, UnavailableReason: request.UnavailableReason,
	}
	if request.AppID > 0 {
		status.Registration = &DeploymentAppRegistration{
			GitHubHost: defaultGitHubAppHost, AppID: request.AppID, Slug: request.Slug,
			OwnerLogin: request.OwnerLogin, OwnerType: request.OwnerType,
			PublicBaseURL: request.PublicBaseURL, WebhookStatus: request.WebhookStatus,
		}
	}
	if status.UnavailableReason == "" && status.UnavailableCode != "" {
		status.UnavailableReason = deploymentAppUnavailableReason(status.UnavailableCode)
	}
	c.service.setMockDeploymentAppStatus(&status)
	c.service.setMockAppAvailable(status.Ready)
	ctx.JSON(http.StatusOK, status)
}

func validMockDeploymentAppState(
	source DeploymentAppSource,
	state string,
	webhookStatus DeploymentAppWebhookStatus,
) bool {
	if source != DeploymentAppSourceNone && source != DeploymentAppSourceEnvironment &&
		source != DeploymentAppSourceManaged {
		return false
	}
	if state != "unconfigured" && state != "registering" && state != "ready" && state != "invalid" {
		return false
	}
	return webhookStatus == "" || webhookStatus == DeploymentAppWebhookUnverified ||
		webhookStatus == DeploymentAppWebhookVerified || webhookStatus == DeploymentAppWebhookFailing
}

// setReposUnavailable toggles the mock client's "list accessible repos
// unavailable" branch so e2e tests can drive the Remote-tab chip popover
// into its "Connect GitHub" banner state. Also clears the service's
// accessible-repos / user-orgs TTL caches so a previously-cached success
// doesn't shadow the toggle for up to 60s.
func (c *MockController) setReposUnavailable(ctx *gin.Context) {
	var req struct {
		Unavailable bool `json:"unavailable"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondInvalidPayload(ctx)
		return
	}
	c.mock.SetReposUnavailable(req.Unavailable)
	if c.service != nil {
		c.service.ClearAccessibleReposCaches()
	}
	ctx.JSON(http.StatusOK, gin.H{"unavailable": req.Unavailable})
}

// errKey is the JSON key used for error responses in this controller. Pulled
// into a constant so goconst (min-occurrences=3 on new code) stops flagging
// every new gin.H{"error":...} the file grows. Mirrors `commitStatusError`
// in scope-of-meaning but is distinct because that constant means a GitHub
// commit-status state, not a generic JSON error key.
const errKey = "error"

// respondInvalidPayload is a tiny helper so callers in this file (and any
// future mock handlers added here) don't each spell out the gin.H{<errKey>:
// "invalid payload"} pair. Reuses the package-level errMsgInvalidPayload
// constant from handlers.go to keep wording (and goconst) in sync.
func respondInvalidPayload(ctx *gin.Context) {
	ctx.JSON(http.StatusBadRequest, gin.H{errKey: errMsgInvalidPayload})
}

func (c *MockController) setUser(ctx *gin.Context) {
	var req struct {
		Username string `json:"username"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	c.mock.SetUser(req.Username)
	ctx.JSON(http.StatusOK, gin.H{"username": req.Username})
}

func (c *MockController) addPRs(ctx *gin.Context) {
	var req struct {
		PRs []PR `json:"prs"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	for i := range req.PRs {
		c.mock.AddPR(&req.PRs[i])
	}
	ctx.JSON(http.StatusOK, gin.H{"added": len(req.PRs)})
}

func (c *MockController) addIssues(ctx *gin.Context) {
	var req struct {
		Issues []Issue `json:"issues"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	for i := range req.Issues {
		c.mock.AddIssue(&req.Issues[i])
	}
	ctx.JSON(http.StatusOK, gin.H{"added": len(req.Issues)})
}

func (c *MockController) addOrgs(ctx *gin.Context) {
	var req struct {
		Orgs []GitHubOrg `json:"orgs"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	c.mock.AddOrgs(req.Orgs)
	ctx.JSON(http.StatusOK, gin.H{"added": len(req.Orgs)})
}

func (c *MockController) addRepos(ctx *gin.Context) {
	var req struct {
		Org   string       `json:"org"`
		Repos []GitHubRepo `json:"repos"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	c.mock.AddRepos(req.Org, req.Repos)
	ctx.JSON(http.StatusOK, gin.H{"added": len(req.Repos)})
}

func (c *MockController) addReviews(ctx *gin.Context) {
	var req struct {
		Owner   string     `json:"owner"`
		Repo    string     `json:"repo"`
		Number  int        `json:"number"`
		Reviews []PRReview `json:"reviews"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	c.mock.AddReviews(req.Owner, req.Repo, req.Number, req.Reviews)
	ctx.JSON(http.StatusOK, gin.H{"added": len(req.Reviews)})
}

func (c *MockController) addComments(ctx *gin.Context) {
	var req struct {
		Owner    string      `json:"owner"`
		Repo     string      `json:"repo"`
		Number   int         `json:"number"`
		Comments []PRComment `json:"comments"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	c.mock.AddComments(req.Owner, req.Repo, req.Number, req.Comments)
	ctx.JSON(http.StatusOK, gin.H{"added": len(req.Comments)})
}

func (c *MockController) addCheckRuns(ctx *gin.Context) {
	var req struct {
		Owner  string     `json:"owner"`
		Repo   string     `json:"repo"`
		Ref    string     `json:"ref"`
		Checks []CheckRun `json:"checks"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	c.mock.AddCheckRuns(req.Owner, req.Repo, req.Ref, req.Checks)
	ctx.JSON(http.StatusOK, gin.H{"added": len(req.Checks)})
}

func (c *MockController) addPRFiles(ctx *gin.Context) {
	var req struct {
		Owner  string   `json:"owner"`
		Repo   string   `json:"repo"`
		Number int      `json:"number"`
		Files  []PRFile `json:"files"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	c.mock.AddPRFiles(req.Owner, req.Repo, req.Number, req.Files)
	ctx.JSON(http.StatusOK, gin.H{"added": len(req.Files)})
}

func (c *MockController) addPRCommits(ctx *gin.Context) {
	var req struct {
		Owner   string         `json:"owner"`
		Repo    string         `json:"repo"`
		Number  int            `json:"number"`
		Commits []PRCommitInfo `json:"commits"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	c.mock.AddPRCommits(req.Owner, req.Repo, req.Number, req.Commits)
	ctx.JSON(http.StatusOK, gin.H{"added": len(req.Commits)})
}

func (c *MockController) addBranches(ctx *gin.Context) {
	var req struct {
		Owner    string       `json:"owner"`
		Repo     string       `json:"repo"`
		Branches []RepoBranch `json:"branches"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if req.Owner == "" || req.Repo == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "owner and repo are required"})
		return
	}
	c.mock.AddBranches(req.Owner, req.Repo, req.Branches)
	ctx.JSON(http.StatusOK, gin.H{"added": len(req.Branches)})
}

// addRepoFiles seeds file content for MockClient.ListRepoDirectory /
// GetRepoFileContent. ref "" seeds a wildcard entry that matches any
// requested ref (see MockClient.SeedRepoFile).
func (c *MockController) addRepoFiles(ctx *gin.Context) {
	var req struct {
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
		Ref   string `json:"ref"`
		Files []struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		} `json:"files"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondInvalidPayload(ctx)
		return
	}
	if req.Owner == "" || req.Repo == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{errKey: "owner and repo are required"})
		return
	}
	for _, f := range req.Files {
		c.mock.SeedRepoFile(req.Owner, req.Repo, req.Ref, f.Path, []byte(f.Content))
	}
	ctx.JSON(http.StatusOK, gin.H{"added": len(req.Files)})
}

// associateTaskPRRequest is the JSON body for the mock-controller's
// associateTaskPR endpoint. Pointer fields are optional — leave them nil
// to skip the corresponding TaskPR column update.
type associateTaskPRRequest struct {
	TaskID                  string `json:"task_id"`
	Owner                   string `json:"owner"`
	Repo                    string `json:"repo"`
	PRNumber                int    `json:"pr_number"`
	PRURL                   string `json:"pr_url"`
	PRTitle                 string `json:"pr_title"`
	HeadBranch              string `json:"head_branch"`
	BaseBranch              string `json:"base_branch"`
	AuthorLogin             string `json:"author_login"`
	State                   string `json:"state"`
	ReviewState             string `json:"review_state"`
	ChecksState             string `json:"checks_state"`
	MergeableState          string `json:"mergeable_state"`
	Additions               int    `json:"additions"`
	Deletions               int    `json:"deletions"`
	ReviewCount             *int   `json:"review_count,omitempty"`
	PendingReviewCount      *int   `json:"pending_review_count,omitempty"`
	RequiredReviews         *int   `json:"required_reviews,omitempty"`
	ChecksTotal             *int   `json:"checks_total,omitempty"`
	ChecksPassing           *int   `json:"checks_passing,omitempty"`
	UnresolvedReviewThreads *int   `json:"unresolved_review_threads,omitempty"`
}

// associateTaskPR directly creates (or replaces) a github_task_prs record for
// E2E testing. Repeating the call with the same (task_id, owner, repo,
// pr_number) updates the row so tests can drive the live-refresh
// scenario by re-POSTing with new aggregate counts.
func (c *MockController) associateTaskPR(ctx *gin.Context) {
	var req associateTaskPRRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if req.State == "" {
		req.State = defaultPRState
	}
	now := time.Now().UTC()
	tp := buildTaskPRFromRequest(&req, now)
	if err := c.store.ReplaceTaskPR(ctx.Request.Context(), tp); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.ensureMockPRForRequest(ctx.Request.Context(), &req, now)
	// Publish the event so the frontend Zustand store picks up the new PR
	// without requiring a page reload — mirrors real AssociatePRWithTask.
	if c.eventBus != nil {
		event := bus.NewEvent(events.GitHubTaskPRUpdated, "github", tp)
		if err := c.eventBus.Publish(ctx.Request.Context(), events.GitHubTaskPRUpdated, event); err != nil {
			c.logger.Debug("mock: failed to publish task PR updated event", zap.Error(err))
		}
	}
	ctx.JSON(http.StatusCreated, tp)
}

// buildTaskPRFromRequest copies the required fields from the JSON body into
// a TaskPR and applies the optional pointer fields when present.
func buildTaskPRFromRequest(req *associateTaskPRRequest, now time.Time) *TaskPR {
	tp := &TaskPR{
		TaskID:         req.TaskID,
		Owner:          req.Owner,
		Repo:           req.Repo,
		PRNumber:       req.PRNumber,
		PRURL:          req.PRURL,
		PRTitle:        req.PRTitle,
		HeadBranch:     req.HeadBranch,
		BaseBranch:     req.BaseBranch,
		AuthorLogin:    req.AuthorLogin,
		State:          req.State,
		ReviewState:    req.ReviewState,
		ChecksState:    req.ChecksState,
		MergeableState: req.MergeableState,
		Additions:      req.Additions,
		Deletions:      req.Deletions,
		CreatedAt:      now,
	}
	if req.ReviewCount != nil {
		tp.ReviewCount = *req.ReviewCount
	}
	if req.PendingReviewCount != nil {
		tp.PendingReviewCount = *req.PendingReviewCount
	}
	if req.RequiredReviews != nil {
		v := *req.RequiredReviews
		tp.RequiredReviews = &v
	}
	if req.ChecksTotal != nil {
		tp.ChecksTotal = *req.ChecksTotal
	}
	if req.ChecksPassing != nil {
		tp.ChecksPassing = *req.ChecksPassing
	}
	if req.UnresolvedReviewThreads != nil {
		tp.UnresolvedReviewThreads = *req.UnresolvedReviewThreads
	}
	return tp
}

// ensureMockPRForRequest seeds a synthetic PR in the mock client so the lazy
// PRFeedback path can resolve check runs by HeadSHA. Skipped when the PR was
// already seeded explicitly via addPRs (tests that pin a head_sha for
// matching check_runs must not have it overwritten).
func (c *MockController) ensureMockPRForRequest(ctx context.Context, req *associateTaskPRRequest, now time.Time) {
	if existing, _ := c.mock.GetPR(ctx, req.Owner, req.Repo, req.PRNumber); existing != nil {
		return
	}
	c.mock.AddPR(&PR{
		Number:         req.PRNumber,
		Title:          req.PRTitle,
		URL:            req.PRURL,
		HTMLURL:        req.PRURL,
		State:          req.State,
		HeadBranch:     req.HeadBranch,
		HeadSHA:        mockHeadSHA(req.Owner, req.Repo, req.PRNumber),
		BaseBranch:     req.BaseBranch,
		AuthorLogin:    req.AuthorLogin,
		MergeableState: req.MergeableState,
		RepoOwner:      req.Owner,
		RepoName:       req.Repo,
		Additions:      req.Additions,
		Deletions:      req.Deletions,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
}

// seedPRFeedback registers checks (and optionally reviews / comments) for a
// specific PR. The PR must already exist in the mock client (associateTaskPR
// synthesizes one). The check ref is the PR's HeadSHA so getPRFeedback
// resolves them via ListCheckRuns.
func (c *MockController) seedPRFeedback(ctx *gin.Context) {
	var req struct {
		Owner    string      `json:"owner"`
		Repo     string      `json:"repo"`
		PRNumber int         `json:"pr_number"`
		Checks   []CheckRun  `json:"checks"`
		Reviews  []PRReview  `json:"reviews"`
		Comments []PRComment `json:"comments"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if req.Owner == "" || req.Repo == "" || req.PRNumber == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "owner, repo, pr_number are required"})
		return
	}
	headSHA := mockHeadSHA(req.Owner, req.Repo, req.PRNumber)
	// If associateTaskPR ran first, an underlying PR row exists with this
	// HeadSHA. Otherwise synthesize a minimal one so getPRFeedback's GetPR
	// call doesn't fail.
	if pr, err := c.mock.GetPR(ctx.Request.Context(), req.Owner, req.Repo, req.PRNumber); err != nil || pr == nil {
		c.mock.AddPR(&PR{
			Number:    req.PRNumber,
			RepoOwner: req.Owner,
			RepoName:  req.Repo,
			HeadSHA:   headSHA,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		})
	}
	// Replace prior seeded checks/reviews/comments so a follow-up call gives
	// deterministic state; helpful for tests that drive a "then a check
	// finishes" transition.
	c.mock.ReplaceCheckRuns(req.Owner, req.Repo, headSHA, req.Checks)
	c.mock.ReplaceReviews(req.Owner, req.Repo, req.PRNumber, req.Reviews)
	c.mock.ReplaceComments(req.Owner, req.Repo, req.PRNumber, req.Comments)
	ctx.JSON(http.StatusOK, gin.H{
		"checks":   len(req.Checks),
		"reviews":  len(req.Reviews),
		"comments": len(req.Comments),
	})
}

// setAuthHealth toggles the mock client's authenticated state so e2e tests
// can drive the "Reconnect GitHub" branch in the popover.
func (c *MockController) setAuthHealth(ctx *gin.Context) {
	var req struct {
		Authenticated bool   `json:"authenticated"`
		Error         string `json:"error"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	c.mock.SetAuthHealth(req.Authenticated, req.Error)
	ctx.JSON(http.StatusOK, gin.H{"authenticated": req.Authenticated})
}

// mockHeadSHA produces a deterministic synthetic head SHA for a (owner, repo,
// pr_number) tuple so test seed calls can compute the same key.
func mockHeadSHA(owner, repo string, prNumber int) string {
	return fmt.Sprintf("mocksha-%s-%s-%d", owner, repo, prNumber)
}

func (c *MockController) reset(ctx *gin.Context) {
	c.mock.Reset()
	if c.service != nil {
		c.service.ClearAccessibleReposCaches()
		c.service.ResetMockAuth("")
	}
	if c.store != nil {
		if err := c.store.DeleteAllMockAuthData(ctx.Request.Context()); err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
			return
		}
	}
	if c.service != nil {
		if err := c.service.ResetDeploymentAppForE2E(ctx.Request.Context()); err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{errKey: err.Error()})
			return
		}
	}
	ctx.JSON(http.StatusOK, gin.H{"reset": true})
}
