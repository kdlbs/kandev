package improvekandev

import (
	"context"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/logger/buffer"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// remoteResolver resolves a local repo's origin remote into provider/owner/name.
// Injectable so tests can avoid touching real git directories.
type remoteResolver func(localPath string) (provider, owner, name string)

// Constants identifying the canonical kandev repository and the workflow
// template loaded from apps/backend/config/workflows/improve-kandev.yml.
const (
	repoCloneURL  = "https://github.com/kdlbs/kandev"
	repoOwner     = "kdlbs"
	repoName      = "kandev"
	repoProvider  = "github"
	defaultBranch = "main"

	templateID   = "improve-kandev"
	workflowName = "Improve Kandev"
	workflowDesc = "Hidden workflow for filing improvements via the in-app entry point."
)

// Cloner is the minimal subset of repoclone.Cloner the bootstrap endpoint uses.
// Defined as an interface so tests can substitute a fake without network access.
type Cloner interface {
	EnsureCloned(ctx context.Context, cloneURL, owner, name string) (string, error)
}

// Handler exposes the improve-kandev HTTP endpoints.
type Handler struct {
	taskSvc *taskservice.Service
	cloner  Cloner
	log     *logger.Logger
	version string
	// snapshot returns the current backend log buffer; defaults to logger's
	// process-wide buffer but can be overridden for tests.
	snapshot func() []buffer.Entry
	// gh resolves the authenticated user's login and write access. Defaults
	// to a gh-CLI shell-out; tests can substitute a fake.
	gh GitHubInfo
	// resolveRemote resolves a local repo path's origin remote. Defaults to
	// service.ResolveGitRemoteProvider; tests can substitute a fake.
	resolveRemote remoteResolver
}

// NewHandler constructs a Handler. version is embedded into bundle metadata.
func NewHandler(taskSvc *taskservice.Service, cloner Cloner, version string, log *logger.Logger) *Handler {
	return &Handler{
		taskSvc:       taskSvc,
		cloner:        cloner,
		log:           log,
		version:       version,
		snapshot:      func() []buffer.Entry { return buffer.Default().Snapshot() },
		gh:            newDefaultGitHubInfo(),
		resolveRemote: taskservice.ResolveGitRemoteProvider,
	}
}

// RegisterRoutes registers the bootstrap and frontend-log endpoints.
func RegisterRoutes(router *gin.Engine, h *Handler) {
	if h == nil {
		return
	}
	api := router.Group("/api/v1/system/improve-kandev")
	api.POST("/bootstrap", h.httpBootstrap)
	api.POST("/bundle/frontend-log", h.httpFrontendLog)
}

// BootstrapRequest is the JSON body for POST /bootstrap.
type BootstrapRequest struct {
	WorkspaceID string `json:"workspace_id"`
}

// ForkStatus reports the result of the bootstrap fork-capability probe. The
// frontend uses it to surface a clear error before the user invests time in
// drafting a contribution that the GitHub side will block.
type ForkStatus string

const (
	// ForkStatusWritable: user has push access on the upstream repo, no fork
	// is needed.
	ForkStatusWritable ForkStatus = "writable"
	// ForkStatusReady: user already has a fork at github.com/{login}/kandev,
	// so the PR step can push to it without forking again.
	ForkStatusReady ForkStatus = "ready"
	// ForkStatusBlockedEMU: the authenticated user looks like an Enterprise
	// Managed User. EMU accounts cannot fork repositories outside their
	// owning enterprise, so the contribution flow will fail at the PR step.
	ForkStatusBlockedEMU ForkStatus = "blocked_emu"
	// ForkStatusUnknown: bootstrap could not determine fork eligibility
	// (e.g., gh CLI lookup failed). Frontend should proceed and rely on the
	// PR step to surface any errors.
	ForkStatusUnknown ForkStatus = "unknown"
)

// BootstrapResponse describes the artifacts the dialog needs to submit a task.
type BootstrapResponse struct {
	RepositoryID   string            `json:"repository_id"`
	WorkflowID     string            `json:"workflow_id"`
	Branch         string            `json:"branch"`
	BundleDir      string            `json:"bundle_dir"`
	BundleFiles    map[string]string `json:"bundle_files"`
	GitHubLogin    string            `json:"github_login"`
	HasWriteAccess bool              `json:"has_write_access"`
	ForkStatus     ForkStatus        `json:"fork_status"`
	ForkMessage    string            `json:"fork_message,omitempty"`
}

func (h *Handler) httpBootstrap(c *gin.Context) {
	var req BootstrapRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.WorkspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}

	ctx := c.Request.Context()
	repo, err := h.resolveOrCloneRepo(ctx, req.WorkspaceID)
	if err != nil {
		h.log.Error("improve-kandev: repository upsert failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to register kandev repository"})
		return
	}

	workflow, err := h.ensureWorkflow(ctx, req.WorkspaceID)
	if err != nil {
		h.log.Error("improve-kandev: workflow upsert failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to ensure improve-kandev workflow"})
		return
	}

	dir, err := createBundleDir()
	if err != nil {
		h.log.Error("improve-kandev: bundle dir creation failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create bundle dir"})
		return
	}
	if err := writeMetadata(dir, h.version, nil); err != nil {
		h.log.Error("improve-kandev: metadata write failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to write metadata"})
		return
	}
	if err := writeBackendLog(dir, h.snapshot()); err != nil {
		h.log.Error("improve-kandev: backend log write failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to write backend log"})
		return
	}

	access := h.resolveGitHubAccess(ctx)

	c.JSON(http.StatusOK, BootstrapResponse{
		RepositoryID: repo.ID,
		WorkflowID:   workflow.ID,
		Branch:       defaultBranch,
		BundleDir:    dir,
		BundleFiles: map[string]string{
			"metadata":     filepath.Join(dir, "metadata.json"),
			"backend_log":  filepath.Join(dir, "backend.log"),
			"frontend_log": filepath.Join(dir, "frontend.log"),
		},
		GitHubLogin:    access.login,
		HasWriteAccess: access.hasWrite,
		ForkStatus:     access.forkStatus,
		ForkMessage:    access.forkMessage,
	})
}

// resolveOrCloneRepo returns the workspace's kandev repository, preferring an
// existing entry — even one the user added themselves with no provider info —
// over cloning a managed copy into ~/.kandev/repos. Order:
//  1. Match by provider info (workspace + github + kdlbs + kandev).
//  2. Match by scanning workspace repos whose origin remote resolves to
//     kdlbs/kandev; backfill provider info on the match.
//  3. Fall back to cloning into the managed location and registering it.
func (h *Handler) resolveOrCloneRepo(ctx context.Context, workspaceID string) (*taskmodels.Repository, error) {
	if existing, err := h.taskSvc.GetRepositoryByProviderInfo(ctx, workspaceID, repoProvider, repoOwner, repoName); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil
	}

	repos, err := h.taskSvc.ListRepositories(ctx, workspaceID)
	if err != nil {
		// Listing failed — log and fall through to clone path; we'd rather
		// risk a duplicate than fail the bootstrap entirely.
		h.log.Warn("improve-kandev: list repositories failed; falling back to clone", zap.Error(err))
	} else if match := findKandevRepoByLocalRemote(repos, h.resolveRemote); match != nil {
		h.backfillKandevProviderInfo(ctx, match)
		return match, nil
	}

	localPath, err := h.cloner.EnsureCloned(ctx, repoCloneURL, repoOwner, repoName)
	if err != nil {
		return nil, err
	}
	return h.taskSvc.FindOrCreateRepository(ctx, &taskservice.FindOrCreateRepositoryRequest{
		WorkspaceID:   workspaceID,
		Provider:      repoProvider,
		ProviderOwner: repoOwner,
		ProviderName:  repoName,
		DefaultBranch: defaultBranch,
		LocalPath:     localPath,
	})
}

// findKandevRepoByLocalRemote returns the first repo with a local path whose
// origin remote resolves to kdlbs/kandev. Skips rows that already declare
// non-matching provider info to avoid hijacking unrelated entries.
func findKandevRepoByLocalRemote(repos []*taskmodels.Repository, resolve remoteResolver) *taskmodels.Repository {
	if resolve == nil {
		return nil
	}
	for _, r := range repos {
		if r == nil || r.LocalPath == "" {
			continue
		}
		if r.Provider != "" && (r.Provider != repoProvider || r.ProviderOwner != repoOwner || r.ProviderName != repoName) {
			continue
		}
		p, o, n := resolve(r.LocalPath)
		if p == repoProvider && o == repoOwner && n == repoName {
			return r
		}
	}
	return nil
}

// backfillKandevProviderInfo fills missing provider/owner/name on an existing
// repo so subsequent lookups by provider info hit the fast path. Failures are
// logged but non-fatal — we still return the matched repo to the caller.
func (h *Handler) backfillKandevProviderInfo(ctx context.Context, repo *taskmodels.Repository) {
	if repo.Provider == repoProvider && repo.ProviderOwner == repoOwner && repo.ProviderName == repoName {
		return
	}
	provider, owner, name := repoProvider, repoOwner, repoName
	branch := repo.DefaultBranch
	if branch == "" {
		branch = defaultBranch
	}
	if _, err := h.taskSvc.UpdateRepository(ctx, repo.ID, &taskservice.UpdateRepositoryRequest{
		Provider:      &provider,
		ProviderOwner: &owner,
		ProviderName:  &name,
		DefaultBranch: &branch,
	}); err != nil {
		h.log.Warn("improve-kandev: backfill provider info failed",
			zap.String("repository_id", repo.ID), zap.Error(err))
		return
	}
	repo.Provider = provider
	repo.ProviderOwner = owner
	repo.ProviderName = name
	repo.DefaultBranch = branch
}

// emuBlockedMessage explains the EMU-restriction case to the contributor in
// terms they can act on. Surfaced in the dialog when ForkStatusBlockedEMU is
// returned.
const emuBlockedMessage = "Your GitHub account appears to be an Enterprise Managed User (EMU) account, " +
	"which typically cannot fork repositories outside your owning enterprise. " +
	"The PR step would fail when forking kdlbs/kandev. Contact your GitHub admin " +
	"if you'd like to enable this, or contribute via another account."

// githubAccess is the resolved bootstrap GitHub state.
type githubAccess struct {
	login       string
	hasWrite    bool
	forkStatus  ForkStatus
	forkMessage string
}

// resolveGitHubAccess resolves the authenticated user's login, push access,
// and a fork-capability hint. Failures are logged at debug level and return
// safe defaults so bootstrap never fails on gh issues; the frontend treats
// ForkStatusUnknown as "proceed and rely on the PR step".
func (h *Handler) resolveGitHubAccess(ctx context.Context) githubAccess {
	out := githubAccess{forkStatus: ForkStatusUnknown}
	if h.gh == nil {
		return out
	}
	login, err := h.gh.GetAuthenticatedLogin(ctx)
	if err != nil {
		h.log.Debug("improve-kandev: gh login lookup failed", zap.Error(err))
		return out
	}
	out.login = login
	hasWrite, err := h.gh.HasRepoWriteAccess(ctx, repoOwner, repoName)
	if err != nil {
		h.log.Debug("improve-kandev: gh write-access check failed", zap.Error(err))
		return out
	}
	out.hasWrite = hasWrite
	if hasWrite {
		out.forkStatus = ForkStatusWritable
		return out
	}
	hasFork, err := h.gh.UserHasFork(ctx, login, repoName)
	if err != nil {
		h.log.Debug("improve-kandev: gh fork lookup failed", zap.Error(err))
		return out
	}
	if hasFork {
		out.forkStatus = ForkStatusReady
		return out
	}
	if isEMULogin(login) {
		out.forkStatus = ForkStatusBlockedEMU
		out.forkMessage = emuBlockedMessage
	}
	return out
}

// ensureWorkflow finds or creates the hidden improve-kandev workflow in the
// given workspace. Idempotent: matches by WorkflowTemplateID == templateID.
func (h *Handler) ensureWorkflow(ctx context.Context, workspaceID string) (*taskmodels.Workflow, error) {
	existing, err := h.taskSvc.ListWorkflows(ctx, workspaceID, true)
	if err != nil {
		return nil, err
	}
	for _, w := range existing {
		if w.WorkflowTemplateID != nil && *w.WorkflowTemplateID == templateID {
			// Heal records created before the workflow honored Hidden on insert.
			// Best-effort: a DB failure here must not block the caller from getting
			// their workflow ID, since the workflow itself is already usable.
			if !w.Hidden {
				if err := h.taskSvc.SetWorkflowHidden(ctx, w.ID, true); err != nil {
					h.log.Warn("improve-kandev: failed to heal hidden flag on stale record",
						zap.String("workflow_id", w.ID), zap.Error(err))
				} else {
					w.Hidden = true
				}
			}
			return w, nil
		}
	}
	tmplID := templateID
	return h.taskSvc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{
		WorkspaceID:        workspaceID,
		Name:               workflowName,
		Description:        workflowDesc,
		WorkflowTemplateID: &tmplID,
		Hidden:             true,
	})
}

// FrontendLogRequest is the JSON body for POST /bundle/frontend-log.
type FrontendLogRequest struct {
	BundleDir string             `json:"bundle_dir"`
	Entries   []FrontendLogEntry `json:"entries"`
}

func (h *Handler) httpFrontendLog(c *gin.Context) {
	var req FrontendLogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	dir, err := validateBundleDir(req.BundleDir)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := writeFrontendLog(dir, req.Entries); err != nil {
		h.log.Error("improve-kandev: frontend log write failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to write frontend log"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"path": filepath.Join(dir, "frontend.log")})
}
