package backendapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	commonlogger "github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	mcp "github.com/kandev/kandev/internal/mcp/handlers"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"go.uber.org/zap"
)

// taskChangeLinkCoordinator is the provider-neutral MCP seam. It keeps task
// reach checks and repository identity resolution at the host boundary, then
// delegates persistence and provider reads to the established services.
type taskChangeLinkCoordinator struct {
	tasks               *taskservice.Service
	db                  *sqlx.DB
	registrationIntents taskChangeRegistrationIntentWriter
	github              githubChangeLinkProvider
	gitlab              gitlabChangeLinkProvider
	logger              *commonlogger.Logger
	singleUserIdentity  func() (authn.Identity, bool)
}

func newTaskChangeLinkCoordinator(p routeParams) taskChangeLinkCoordinator {
	coordinator := taskChangeLinkCoordinator{
		tasks: p.taskSvc, logger: p.log, singleUserIdentity: taskChangeLinkIdentityResolver(p.authSvc),
	}
	if p.services != nil {
		coordinator.github = p.services.GitHub
		coordinator.gitlab = p.services.GitLab
	}
	if p.dbPool != nil && p.taskRepo != nil {
		coordinator.db = p.dbPool.Writer()
		coordinator.registrationIntents = p.taskRepo
	}
	return coordinator
}

func (c taskChangeLinkCoordinator) ManageTaskChangeRequest(
	ctx context.Context, req mcp.TaskChangeLinkRequest,
) (mcp.TaskChangeLinkMutationResult, error) {
	switch req.Operation {
	case "", "link":
		return c.simpleMutation(ctx, req, c.LinkTaskChange)
	case "unlink":
		return c.simpleMutation(ctx, req, c.UnlinkTaskChange)
	case "replace":
		return c.replaceTaskChangeResult(ctx, req)
	default:
		return c.taskChangeMutationFailure(req.TaskID, fmt.Errorf("unsupported task change operation %q", req.Operation))
	}
}

func (c taskChangeLinkCoordinator) simpleMutation(
	ctx context.Context,
	req mcp.TaskChangeLinkRequest,
	apply func(context.Context, mcp.TaskChangeLinkRequest) ([]mcp.TaskChangeLink, error),
) (mcp.TaskChangeLinkMutationResult, error) {
	links, err := apply(ctx, req)
	if err != nil {
		return c.taskChangeMutationFailure(req.TaskID, err)
	}
	return mcp.TaskChangeLinkMutationResult{TaskID: req.TaskID, Links: links, StateKnown: true}, nil
}

const (
	taskChangeMutationOperationError = "task change request operation failed"
	taskChangeMutationRollbackError  = "task change request rollback could not be completed"
	taskChangeMutationReadbackError  = "task change request state readback failed"
)

func (c taskChangeLinkCoordinator) taskChangeMutationFailure(taskID string, err error) (mcp.TaskChangeLinkMutationResult, error) {
	c.logMutationError(taskID, "operation", err)
	return mcp.TaskChangeLinkMutationResult{
		TaskID: taskID, OperationError: taskChangeMutationOperationError, StateKnown: false,
	}, err
}

func (c taskChangeLinkCoordinator) logMutationError(taskID, stage string, err error) {
	if c.logger == nil || err == nil {
		return
	}
	c.logger.Error("task change request mutation failed",
		zap.String("task_id", taskID), zap.String("stage", stage), zap.Error(err))
}

const (
	taskChangeProviderGitHub = "github"
	taskChangeProviderGitLab = "gitlab"
)

type githubChangeLinkProvider interface {
	AssociateExistingPRByURLForWorkspace(context.Context, string, string, string, string, string) (*github.TaskPR, error)
	DetachTaskPR(context.Context, string, string) (*github.TaskPR, error)
	ListTaskPRs(context.Context, []string) (map[string][]*github.TaskPR, error)
}

type gitlabChangeLinkProvider interface {
	AssociateExistingMRByURL(context.Context, string, string, string, string) (*gitlab.TaskMR, error)
	ListTaskMRsByTask(context.Context, string) ([]*gitlab.TaskMR, error)
	UnlinkTaskMR(context.Context, string, string) error
}

type githubObservationChangeLinkProvider interface {
	RegisterExistingPRByURLForWorkspace(context.Context, string, string, string, string, string, bool, string) (*github.TaskPR, error)
}

type gitlabObservationChangeLinkProvider interface {
	RegisterExistingMRByURL(context.Context, string, string, string, string, bool, string) (*gitlab.TaskMR, error)
}

type githubTransactionalObservationProvider interface {
	RegisterExistingPRByURLForWorkspaceWithCommit(context.Context, string, string, string, string, string, bool, string, github.TaskPRObservationCommitter) (*github.TaskPR, error)
	RegisterTaskPRObservationTx(context.Context, *sqlx.Tx, *github.TaskPR, bool, int64) (*github.TaskPR, bool, error)
}

type gitlabTransactionalObservationProvider interface {
	RegisterExistingMRByURLWithCommit(context.Context, string, string, string, string, bool, string, gitlab.TaskMRObservationCommitter) (*gitlab.TaskMR, error)
	RegisterTaskMRObservationTx(context.Context, *sqlx.Tx, *gitlab.TaskMR, bool, int64) (*gitlab.TaskMR, bool, error)
}

type taskChangeRegistrationIntentWriter interface {
	CreateTaskChangeRegistrationIntentTx(context.Context, *sqlx.Tx, *tasksqlite.TaskChangeRegistrationIntent) (*tasksqlite.TaskChangeRegistrationIntent, bool, error)
}

func (c taskChangeLinkCoordinator) LinkTaskChange(ctx context.Context, req mcp.TaskChangeLinkRequest) ([]mcp.TaskChangeLink, error) {
	if req.URL != "" {
		resolved, err := c.resolveTaskChangeURL(ctx, req.TaskID, req.URL)
		if err != nil {
			return nil, err
		}
		if err := assertTaskChangeLinkMatchesURL(req.Link, resolved); err != nil {
			return nil, err
		}
		req.Link = resolved
	}
	if err := c.link(ctx, req); err != nil {
		return nil, err
	}
	return c.list(ctx, req.TaskID)
}

func (c taskChangeLinkCoordinator) UnlinkTaskChange(ctx context.Context, req mcp.TaskChangeLinkRequest) ([]mcp.TaskChangeLink, error) {
	if req.URL != "" {
		resolved, err := c.resolveTaskChangeURL(ctx, req.TaskID, req.URL)
		if err != nil {
			return nil, err
		}
		if err := assertTaskChangeLinkMatchesURL(req.Link, resolved); err != nil {
			return nil, err
		}
		req.Link = resolved
	}
	if err := c.unlink(ctx, req.TaskID, req.Link); err != nil {
		return nil, err
	}
	return c.list(ctx, req.TaskID)
}

func (c taskChangeLinkCoordinator) ReplaceTaskChange(ctx context.Context, req mcp.TaskChangeLinkRequest) ([]mcp.TaskChangeLink, error) {
	result, err := c.replaceTaskChangeResult(ctx, req)
	return result.Links, err
}

func (c taskChangeLinkCoordinator) replaceTaskChangeResult(
	ctx context.Context, req mcp.TaskChangeLinkRequest,
) (mcp.TaskChangeLinkMutationResult, error) {
	result := mcp.TaskChangeLinkMutationResult{TaskID: req.TaskID}
	if req.Old == nil {
		return c.taskChangeMutationFailure(req.TaskID, fmt.Errorf("current task change identity is required"))
	}
	if req.URL != "" {
		resolved, err := c.resolveTaskChangeURL(ctx, req.TaskID, req.URL)
		if err != nil {
			return c.taskChangeMutationFailure(req.TaskID, err)
		}
		if err := assertTaskChangeLinkMatchesURL(req.Link, resolved); err != nil {
			return c.taskChangeMutationFailure(req.TaskID, err)
		}
		req.Link = resolved
	}
	// Replacing an association with the exact same identity is already the
	// requested final state. Do not detach it after the idempotent link.
	if req.Link == *req.Old {
		links, err := c.list(ctx, req.TaskID)
		if err != nil {
			return c.taskChangeMutationFailure(req.TaskID, err)
		}
		result.Links, result.StateKnown = links, true
		return result, nil
	}
	if req.Link.Provider != req.Old.Provider {
		return c.taskChangeMutationFailure(req.TaskID, fmt.Errorf("cross-provider task change replacement is not supported"))
	}
	before, err := c.list(ctx, req.TaskID)
	if err != nil {
		return c.taskChangeMutationFailure(req.TaskID, err)
	}
	result.Links, result.StateKnown = before, true
	newAlreadyLinked := taskChangeLinksContain(before, req.Link)
	// Resolve and persist the incoming association first. A failed provider
	// fetch therefore leaves the current association untouched.
	if err := c.link(ctx, req); err != nil {
		c.logMutationError(req.TaskID, "link replacement", err)
		result.OperationError = taskChangeMutationOperationError
		return result, err
	}
	if err := c.unlink(ctx, req.TaskID, *req.Old); err != nil {
		if newAlreadyLinked {
			return c.readReplacementFailureState(ctx, result, err)
		}
		return c.rollbackReplacementResult(ctx, req, result, err)
	}
	links, listErr := c.list(ctx, req.TaskID)
	if listErr != nil {
		c.logMutationError(req.TaskID, "replacement readback", listErr)
		result.OperationError = taskChangeMutationReadbackError
		result.StateKnown = false
		result.Links = nil
		return result, listErr
	}
	result.Links, result.StateKnown = links, true
	return result, nil
}

func assertTaskChangeLinkMatchesURL(asserted, resolved mcp.TaskChangeLink) error {
	if (asserted.Provider != "" && asserted.Provider != resolved.Provider) ||
		(asserted.RepositoryID != "" && asserted.RepositoryID != resolved.RepositoryID) ||
		(asserted.Number != 0 && asserted.Number != resolved.Number) {
		return fmt.Errorf("URL does not match the asserted task change identity")
	}
	return nil
}

func (c taskChangeLinkCoordinator) readReplacementFailureState(
	ctx context.Context, result mcp.TaskChangeLinkMutationResult, operationErr error,
) (mcp.TaskChangeLinkMutationResult, error) {
	links, stateErr := c.list(ctx, result.TaskID)
	c.logMutationError(result.TaskID, "replacement operation", operationErr)
	result.OperationError = taskChangeMutationOperationError
	if stateErr != nil {
		c.logMutationError(result.TaskID, "replacement readback", stateErr)
		result.StateKnown = false
		result.Links = nil
		return result, errors.Join(operationErr, fmt.Errorf("active task change links unavailable after failed replacement: %w", stateErr))
	}
	result.Links, result.StateKnown = links, true
	return result, operationErr
}

func (c taskChangeLinkCoordinator) rollbackReplacementResult(
	ctx context.Context,
	req mcp.TaskChangeLinkRequest,
	result mcp.TaskChangeLinkMutationResult,
	operationErr error,
) (mcp.TaskChangeLinkMutationResult, error) {
	rollbackErr := c.unlink(ctx, req.TaskID, req.Link)
	c.logMutationError(req.TaskID, "replacement operation", operationErr)
	result.OperationError = taskChangeMutationOperationError
	if rollbackErr == nil {
		links, stateErr := c.list(ctx, req.TaskID)
		if stateErr != nil {
			c.logMutationError(req.TaskID, "rollback readback", stateErr)
			result.OperationError = taskChangeMutationReadbackError
			result.StateKnown = false
			result.Links = nil
			return result, errors.Join(operationErr, fmt.Errorf("active task change links unavailable after rollback: %w", stateErr))
		}
		result.Links, result.StateKnown = links, true
		return result, operationErr
	}
	c.logMutationError(req.TaskID, "replacement rollback", rollbackErr)
	result.RollbackError = taskChangeMutationRollbackError
	activeLinks, stateErr := c.list(ctx, req.TaskID)
	if stateErr != nil {
		c.logMutationError(req.TaskID, "rollback readback", stateErr)
		result.StateKnown = false
		result.Links = nil
		return result, errors.Join(
			operationErr,
			fmt.Errorf("rollback new task change link: %w", rollbackErr),
			fmt.Errorf("active task change links unavailable after failed replacement: %w", stateErr),
		)
	}
	result.Links, result.StateKnown = activeLinks, true
	return result, errors.Join(
		operationErr,
		fmt.Errorf("rollback new task change link: %w", rollbackErr),
		fmt.Errorf("active task change links after failed replacement: %v", activeLinks),
	)
}

func taskChangeLinksContain(links []mcp.TaskChangeLink, target mcp.TaskChangeLink) bool {
	for _, link := range links {
		if link.Provider == target.Provider && link.RepositoryID == target.RepositoryID && link.Number == target.Number {
			return true
		}
	}
	return false
}

func (c taskChangeLinkCoordinator) link(ctx context.Context, req mcp.TaskChangeLinkRequest) error {
	task, repo, err := c.taskRepository(ctx, req.TaskID, req.Link.RepositoryID)
	if err != nil {
		return err
	}
	switch req.Link.Provider {
	case taskChangeProviderGitHub:
		return c.linkGitHub(ctx, req, task, repo)
	case taskChangeProviderGitLab:
		return c.linkGitLab(ctx, req, task, repo)
	default:
		return fmt.Errorf("unsupported provider %q", req.Link.Provider)
	}
}

func (c taskChangeLinkCoordinator) linkGitHub(
	ctx context.Context, req mcp.TaskChangeLinkRequest, task *models.Task, repo *models.Repository,
) error {
	if c.github == nil {
		return fmt.Errorf("GitHub PR links are not available")
	}
	changeURL, err := githubChangeURL(repo.ProviderHost, repo.ProviderOwner, repo.ProviderName, req.Link.Number)
	if err != nil {
		return err
	}
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok && c.singleUserIdentity != nil {
		identity, ok = c.singleUserIdentity()
	}
	if !ok || strings.TrimSpace(identity.UserID) == "" {
		return fmt.Errorf("authenticated user identity is required for GitHub PR links")
	}
	if req.AssociationMode == "observation" || req.URL != "" {
		return c.registerGitHubObservation(ctx, req, task, repo, identity.UserID, changeURL)
	}
	_, err = c.github.AssociateExistingPRByURLForWorkspace(
		ctx, task.WorkspaceID, identity.UserID, task.ID, repo.ID, changeURL,
	)
	return err
}

func (c taskChangeLinkCoordinator) registerGitHubObservation(
	ctx context.Context,
	req mcp.TaskChangeLinkRequest,
	task *models.Task,
	repo *models.Repository,
	userID, changeURL string,
) error {
	provider, ok := c.github.(githubObservationChangeLinkProvider)
	if !ok {
		return fmt.Errorf("GitHub provider does not support observation-only registration")
	}
	restore := req.RestoreDetached != nil && *req.RestoreDetached
	if c.db == nil || c.registrationIntents == nil {
		_, err := provider.RegisterExistingPRByURLForWorkspace(
			ctx, task.WorkspaceID, userID, task.ID, repo.ID, changeURL, restore, req.OperationID,
		)
		return err
	}
	transactional, ok := c.github.(githubTransactionalObservationProvider)
	if !ok {
		return fmt.Errorf("GitHub provider does not support transactional registration")
	}
	_, err := transactional.RegisterExistingPRByURLForWorkspaceWithCommit(
		ctx, task.WorkspaceID, userID, task.ID, repo.ID, changeURL, restore, req.OperationID,
		func(commitCtx context.Context, registration *github.TaskPR, restore bool, revision int64) (*github.TaskPR, bool, error) {
			return c.commitGitHubRegistration(commitCtx, transactional, registration, restore, revision, req.OperationID)
		},
	)
	return err
}

func (c taskChangeLinkCoordinator) commitGitHubRegistration(
	ctx context.Context,
	provider githubTransactionalObservationProvider,
	registration *github.TaskPR,
	restore bool,
	revision int64,
	operationID string,
) (*github.TaskPR, bool, error) {
	tx, err := c.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	registered, changed, err := provider.RegisterTaskPRObservationTx(ctx, tx, registration, restore, revision)
	if err != nil {
		return nil, false, err
	}
	if err := c.persistGitHubRegistrationIntent(ctx, tx, registration, registered, revision, restore, operationID); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return registered, changed, nil
}

func (c taskChangeLinkCoordinator) linkGitLab(
	ctx context.Context, req mcp.TaskChangeLinkRequest, task *models.Task, repo *models.Repository,
) error {
	if c.gitlab == nil {
		return fmt.Errorf("GitLab MR links are not available")
	}
	changeURL, err := gitlabChangeURL(repo.ProviderHost, repo.ProviderOwner, repo.ProviderName, req.Link.Number)
	if err != nil {
		return err
	}
	if req.AssociationMode == "observation" || req.URL != "" {
		return c.registerGitLabObservation(ctx, req, task, repo, changeURL)
	}
	_, err = c.gitlab.AssociateExistingMRByURL(ctx, task.WorkspaceID, task.ID, repo.ID, changeURL)
	return err
}

func (c taskChangeLinkCoordinator) registerGitLabObservation(
	ctx context.Context,
	req mcp.TaskChangeLinkRequest,
	task *models.Task,
	repo *models.Repository,
	changeURL string,
) error {
	provider, ok := c.gitlab.(gitlabObservationChangeLinkProvider)
	if !ok {
		return fmt.Errorf("GitLab provider does not support observation-only registration")
	}
	restore := req.RestoreDetached != nil && *req.RestoreDetached
	if c.db == nil || c.registrationIntents == nil {
		_, err := provider.RegisterExistingMRByURL(ctx, task.WorkspaceID, task.ID, repo.ID, changeURL, restore, req.OperationID)
		return err
	}
	transactional, ok := c.gitlab.(gitlabTransactionalObservationProvider)
	if !ok {
		return fmt.Errorf("GitLab provider does not support transactional registration")
	}
	_, err := transactional.RegisterExistingMRByURLWithCommit(
		ctx, task.WorkspaceID, task.ID, repo.ID, changeURL, restore, req.OperationID,
		func(commitCtx context.Context, registration *gitlab.TaskMR, restore bool, revision int64) (*gitlab.TaskMR, bool, error) {
			return c.commitGitLabRegistration(commitCtx, transactional, registration, restore, revision, req.OperationID)
		},
	)
	return err
}

func (c taskChangeLinkCoordinator) commitGitLabRegistration(
	ctx context.Context,
	provider gitlabTransactionalObservationProvider,
	registration *gitlab.TaskMR,
	restore bool,
	revision int64,
	operationID string,
) (*gitlab.TaskMR, bool, error) {
	tx, err := c.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	registered, changed, err := provider.RegisterTaskMRObservationTx(ctx, tx, registration, restore, revision)
	if err != nil {
		return nil, false, err
	}
	if err := c.persistGitLabRegistrationIntent(ctx, tx, registration, registered, revision, restore, operationID); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return registered, changed, nil
}

func (c taskChangeLinkCoordinator) persistGitHubRegistrationIntent(
	ctx context.Context, tx *sqlx.Tx, requested, registered *github.TaskPR, expectedRevision int64, restore bool, operationID string,
) error {
	snapshot, err := json.Marshal(map[string]any{
		"url": requested.PRURL, "repository_path": requested.RepositoryPath,
		"provider_repository_id": requested.ProviderRepositoryID, "number": requested.PRNumber,
	})
	if err != nil {
		return err
	}
	return c.persistTaskChangeRegistrationIntent(ctx, tx, &tasksqlite.TaskChangeRegistrationIntent{
		TaskID: requested.TaskID, WorkspaceID: requested.WorkspaceID, SourceKind: "agent_registration",
		OriginID: requested.FirstOriginID, SourceRevision: strconv.FormatInt(expectedRevision, 10),
		ReferenceRef:      fmt.Sprintf("github:%s#%d", requested.ProviderRepositoryID, requested.PRNumber),
		ReferenceSnapshot: string(snapshot), Provider: "github", ProviderHost: requested.ProviderHost,
		ConnectionScope: requested.ConnectionScope, ProviderRepositoryID: requested.ProviderRepositoryID,
		ChangeRequestID: strconv.Itoa(requested.PRNumber), ResolvedIdentity: string(snapshot),
		ExpectedRevision: int64Pointer(expectedRevision), AssociationRevision: int64Pointer(registered.AssociationRevision),
		OperationID: operationID, OperationFingerprint: taskChangeRegistrationFingerprint(
			requested.TaskID, requested.ProviderHost, requested.ConnectionScope, requested.ProviderRepositoryID,
			strconv.Itoa(requested.PRNumber), strconv.FormatInt(expectedRevision, 10), strconv.FormatBool(restore)),
		OperationReceipt: registered.ID, State: "succeeded",
	})
}

func (c taskChangeLinkCoordinator) persistGitLabRegistrationIntent(
	ctx context.Context, tx *sqlx.Tx, requested, registered *gitlab.TaskMR, expectedRevision int64, restore bool, operationID string,
) error {
	snapshot, err := json.Marshal(map[string]any{
		"url": requested.MRURL, "repository_path": requested.RepositoryPath,
		"provider_repository_id": requested.ProviderRepositoryID, "iid": requested.MRIID,
	})
	if err != nil {
		return err
	}
	return c.persistTaskChangeRegistrationIntent(ctx, tx, &tasksqlite.TaskChangeRegistrationIntent{
		TaskID: requested.TaskID, WorkspaceID: strings.TrimPrefix(requested.ConnectionScope, "builtin:"),
		SourceKind: "agent_registration", OriginID: requested.FirstOriginID,
		SourceRevision:    strconv.FormatInt(expectedRevision, 10),
		ReferenceRef:      fmt.Sprintf("gitlab:%s#%d", requested.ProviderRepositoryID, requested.MRIID),
		ReferenceSnapshot: string(snapshot), Provider: "gitlab", ProviderHost: requested.Host,
		ConnectionScope: requested.ConnectionScope, ProviderRepositoryID: requested.ProviderRepositoryID,
		ChangeRequestID: strconv.FormatInt(int64(requested.MRIID), 10), ResolvedIdentity: string(snapshot),
		ExpectedRevision: int64Pointer(expectedRevision), AssociationRevision: int64Pointer(registered.AssociationRevision),
		OperationID: operationID, OperationFingerprint: taskChangeRegistrationFingerprint(
			requested.TaskID, requested.Host, requested.ConnectionScope, requested.ProviderRepositoryID,
			strconv.Itoa(requested.MRIID), strconv.FormatInt(expectedRevision, 10), strconv.FormatBool(restore)),
		OperationReceipt: registered.ID, State: "succeeded",
	})
}

func (c taskChangeLinkCoordinator) persistTaskChangeRegistrationIntent(
	ctx context.Context, tx *sqlx.Tx, intent *tasksqlite.TaskChangeRegistrationIntent,
) error {
	_, _, err := c.registrationIntents.CreateTaskChangeRegistrationIntentTx(ctx, tx, intent)
	return err
}

func int64Pointer(value int64) *int64 {
	return &value
}

func taskChangeRegistrationFingerprint(values ...string) string {
	encoded, _ := json.Marshal(values)
	fingerprint := sha256.Sum256(encoded)
	return hex.EncodeToString(fingerprint[:])
}

type taskChangeURLIdentity struct {
	provider string
	scheme   string
	host     string
	owner    string
	repo     string
	number   int
}

func (c taskChangeLinkCoordinator) resolveTaskChangeURL(
	ctx context.Context, taskID, rawURL string,
) (mcp.TaskChangeLink, error) {
	task, _, err := c.taskRepository(ctx, taskID, "")
	if err != nil {
		return mcp.TaskChangeLink{}, err
	}
	identity, err := parseTaskChangeURL(rawURL)
	if err != nil {
		return mcp.TaskChangeLink{}, err
	}
	var matches []mcp.TaskChangeLink
	for _, taskRepo := range task.Repositories {
		repositoryID := strings.TrimSpace(taskRepo.RepositoryID)
		if repositoryID == "" {
			continue
		}
		repo, getErr := c.tasks.GetRepository(ctx, repositoryID)
		if getErr != nil || repo == nil || repo.WorkspaceID != task.WorkspaceID {
			continue
		}
		if taskChangeURLMatchesRepository(identity, repo) {
			matches = append(matches, mcp.TaskChangeLink{
				Provider: identity.provider, RepositoryID: repo.ID, Number: identity.number,
			})
		}
	}
	if len(matches) == 0 {
		return mcp.TaskChangeLink{}, fmt.Errorf("task change URL does not match an attached repository")
	}
	if len(matches) > 1 {
		return mcp.TaskChangeLink{}, fmt.Errorf("task change URL matches more than one attached repository")
	}
	return matches[0], nil
}

const (
	taskChangeURLSchemeHTTP  = "http"
	taskChangeURLSchemeHTTPS = "https"
)

func parseTaskChangeURL(rawURL string) (taskChangeURLIdentity, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil || parsed.User != nil || parsed.Host == "" ||
		(parsed.Scheme != taskChangeURLSchemeHTTPS && parsed.Scheme != taskChangeURLSchemeHTTP) {
		return taskChangeURLIdentity{}, fmt.Errorf("invalid task change URL")
	}
	if parsed.RawPath != "" || strings.Contains(parsed.EscapedPath(), "%") {
		return taskChangeURLIdentity{}, fmt.Errorf("encoded task change paths are not supported")
	}
	identity := taskChangeURLIdentity{scheme: strings.ToLower(parsed.Scheme), host: normalizedURLHost(parsed)}
	path := strings.Trim(parsed.Path, "/")
	parts := strings.Split(path, "/")
	switch {
	case isGitHubPullRequestPath(parts):
		identity, err = parseGitHubPullRequestPath(identity, parts)
	case isGitLabMergeRequestPath(parts):
		identity, err = parseGitLabMergeRequestPath(identity, parts)
	default:
		return taskChangeURLIdentity{}, fmt.Errorf("URL does not identify a supported pull request or merge request")
	}
	if err != nil || identity.number <= 0 || identity.host == "" {
		return taskChangeURLIdentity{}, fmt.Errorf("invalid task change URL identity")
	}
	return identity, nil
}

func isGitHubPullRequestPath(parts []string) bool {
	return len(parts) >= 4 && parts[2] == "pull"
}

func parseGitHubPullRequestPath(
	identity taskChangeURLIdentity, parts []string,
) (taskChangeURLIdentity, error) {
	validDepth := len(parts) == 4 || (len(parts) == 5 && (parts[4] == "files" || parts[4] == "commits"))
	if !validDepth || !validTaskChangePathPart(parts[0]) || !validTaskChangePathPart(parts[1]) {
		return taskChangeURLIdentity{}, fmt.Errorf("invalid GitHub pull request URL")
	}
	number, err := strconv.Atoi(parts[3])
	identity.provider, identity.owner, identity.repo, identity.number = taskChangeProviderGitHub, parts[0], parts[1], number
	return identity, err
}

func isGitLabMergeRequestPath(parts []string) bool {
	return len(parts) >= 5 && parts[len(parts)-3] == "-" && parts[len(parts)-2] == "merge_requests"
}

func parseGitLabMergeRequestPath(
	identity taskChangeURLIdentity, parts []string,
) (taskChangeURLIdentity, error) {
	projectPath := strings.Join(parts[:len(parts)-3], "/")
	if projectPath == "" || strings.ContainsAny(projectPath, "?#") {
		return taskChangeURLIdentity{}, fmt.Errorf("invalid GitLab project path")
	}
	number, err := strconv.Atoi(parts[len(parts)-1])
	identity.provider, identity.owner, identity.number = taskChangeProviderGitLab, projectPath, number
	return identity, err
}

func normalizedURLHost(parsed *url.URL) string {
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if port := parsed.Port(); port != "" {
		return host + ":" + port
	}
	return host
}

func validTaskChangePathPart(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.ContainsAny(value, "\\?#")
}

func taskChangeURLMatchesRepository(identity taskChangeURLIdentity, repo *models.Repository) bool {
	if strings.ToLower(strings.TrimSpace(repo.Provider)) != identity.provider {
		return false
	}
	providerHost := strings.TrimSpace(repo.ProviderHost)
	if providerHost == "" || !strings.Contains(providerHost, "://") {
		providerHost = "https://" + strings.TrimPrefix(providerHost, "//")
	}
	parsedHost, err := url.Parse(providerHost)
	if err != nil || parsedHost.Host == "" || parsedHost.User != nil ||
		!strings.EqualFold(parsedHost.Scheme, identity.scheme) || normalizedURLHost(parsedHost) != identity.host {
		return false
	}
	owner, name := strings.Trim(strings.TrimSpace(repo.ProviderOwner), "/"), strings.Trim(strings.TrimSpace(repo.ProviderName), "/")
	if owner == "" || name == "" {
		return false
	}
	if identity.provider == taskChangeProviderGitHub {
		return strings.EqualFold(owner, identity.owner) && strings.EqualFold(name, identity.repo)
	}
	return strings.EqualFold(owner+"/"+name, identity.owner)
}

func (c taskChangeLinkCoordinator) unlink(ctx context.Context, taskID string, link mcp.TaskChangeLink) error {
	task, _, err := c.taskRepository(ctx, taskID, "")
	if err != nil {
		return err
	}
	switch link.Provider {
	case taskChangeProviderGitHub:
		if c.github == nil {
			return fmt.Errorf("GitHub PR links are not available")
		}
		prs, err := c.githubLinks(ctx, task.ID)
		if err != nil {
			return err
		}
		for _, pr := range prs {
			if pr.RepositoryID == link.RepositoryID && pr.PRNumber == link.Number {
				_, err = c.github.DetachTaskPR(ctx, task.WorkspaceID, pr.ID)
				return err
			}
		}
		return nil
	case taskChangeProviderGitLab:
		if c.gitlab == nil {
			return fmt.Errorf("GitLab MR links are not available")
		}
		mrs, err := c.gitlab.ListTaskMRsByTask(ctx, task.ID)
		if err != nil {
			return err
		}
		identityResolver := gitlabTaskChangeRequestAdapter{tasks: c.tasks}
		targetRepositoryID := strings.TrimSpace(link.RepositoryID)
		// A persisted canonical identity is sufficient to identify the exact
		// association, even when its repository was later removed from the task.
		// Scan these rows before resolving legacy rows so an unrelated legacy MR
		// with the same IID cannot prevent cleanup of the requested association.
		for _, mr := range mrs {
			if mr == nil || mr.MRIID != link.Number {
				continue
			}
			if strings.TrimSpace(mr.RepositoryID) == targetRepositoryID {
				return c.gitlab.UnlinkTaskMR(ctx, task.WorkspaceID, mr.ID)
			}
		}
		for _, mr := range mrs {
			if mr == nil || mr.MRIID != link.Number || strings.TrimSpace(mr.RepositoryID) != "" {
				continue
			}
			repositoryID, identityStatus, resolveErr := identityResolver.resolveGitLabRepositoryIdentityWithError(ctx, task, mr)
			if resolveErr != nil {
				return fmt.Errorf("GitLab MR identity lookup failed: %w", resolveErr)
			}
			if identityStatus == mcp.TaskChangeRequestIdentityAmbiguous {
				candidates := identityResolver.gitLabRepositoryCandidates(ctx, task, mr)
				if _, matchesTarget := candidates[targetRepositoryID]; matchesTarget {
					return fmt.Errorf("GitLab MR identity is ambiguous")
				}
			}
			if identityStatus == mcp.TaskChangeRequestIdentityResolved && repositoryID == targetRepositoryID {
				return c.gitlab.UnlinkTaskMR(ctx, task.WorkspaceID, mr.ID)
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported provider %q", link.Provider)
	}
}

func (c taskChangeLinkCoordinator) list(ctx context.Context, taskID string) ([]mcp.TaskChangeLink, error) {
	task, _, err := c.taskRepository(ctx, taskID, "")
	if err != nil {
		return nil, err
	}
	links := []mcp.TaskChangeLink{}
	if c.github != nil {
		prs, err := c.githubLinks(ctx, taskID)
		if err != nil {
			return nil, err
		}
		for _, pr := range prs {
			links = append(links, mcp.TaskChangeLink{Provider: taskChangeProviderGitHub, RepositoryID: pr.RepositoryID, Number: pr.PRNumber})
		}
	}
	if c.gitlab != nil {
		mrs, err := c.gitlab.ListTaskMRsByTask(ctx, taskID)
		if err != nil {
			return nil, err
		}
		identityResolver := gitlabTaskChangeRequestAdapter{tasks: c.tasks}
		for _, mr := range mrs {
			if mr == nil {
				continue
			}
			repositoryID := strings.TrimSpace(mr.RepositoryID)
			if repositoryID == "" {
				resolvedID, identityStatus := identityResolver.resolveGitLabRepositoryIdentity(ctx, task, mr)
				if identityStatus == mcp.TaskChangeRequestIdentityResolved {
					repositoryID = resolvedID
				}
			}
			links = append(links, mcp.TaskChangeLink{Provider: taskChangeProviderGitLab, RepositoryID: repositoryID, Number: mr.MRIID})
		}
	}
	return links, nil
}

func (c taskChangeLinkCoordinator) githubLinks(ctx context.Context, taskID string) ([]*github.TaskPR, error) {
	byTask, err := c.github.ListTaskPRs(ctx, []string{taskID})
	if err != nil {
		return nil, err
	}
	return byTask[taskID], nil
}

func (c taskChangeLinkCoordinator) taskRepository(ctx context.Context, taskID, repositoryID string) (*models.Task, *models.Repository, error) {
	if c.tasks == nil {
		return nil, nil, fmt.Errorf("task service is not available")
	}
	task, err := c.tasks.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return nil, nil, fmt.Errorf("task not found")
	}
	if repositoryID == "" {
		return task, nil, nil
	}
	for _, taskRepo := range task.Repositories {
		if taskRepo.RepositoryID != repositoryID {
			continue
		}
		repo, getErr := c.tasks.GetRepository(ctx, repositoryID)
		if getErr != nil || repo == nil || repo.WorkspaceID != task.WorkspaceID {
			return nil, nil, fmt.Errorf("repository not found for task")
		}
		return task, repo, nil
	}
	return nil, nil, fmt.Errorf("repository not found for task")
}

func githubChangeURL(host, owner, name string, number int) (string, error) {
	normalizedHost, err := normalizedGitHubHost(host)
	if err != nil {
		return "", err
	}
	owner = strings.TrimSpace(owner)
	name = strings.TrimSpace(name)
	if owner == "" || name == "" {
		return "", fmt.Errorf("repository has no canonical GitHub identity")
	}
	return fmt.Sprintf("https://%s/%s/%s/pull/%d", normalizedHost, owner, name, number), nil
}

func normalizedGitHubHost(host string) (string, error) {
	rawHost := strings.TrimSpace(host)
	if rawHost == "" {
		return "", fmt.Errorf("repository has no canonical GitHub host")
	}
	if !strings.Contains(rawHost, "://") {
		rawHost = "https://" + rawHost
	}
	parsed, err := url.Parse(rawHost)
	if err != nil || parsed.User != nil || parsed.Host == "" || parsed.Path != "" && parsed.Path != "/" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("repository has invalid GitHub host")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Port() != "" {
		return "", fmt.Errorf("repository has invalid GitHub host")
	}
	normalizedHost := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if normalizedHost != "github.com" && !strings.HasSuffix(normalizedHost, ".github.com") {
		return "", fmt.Errorf("repository is not a GitHub repository")
	}
	return normalizedHost, nil
}

func gitlabChangeURL(host, owner, name string, number int) (string, error) {
	host = strings.TrimRight(strings.TrimSpace(host), "/")
	if host == "" || strings.TrimSpace(owner) == "" || strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("repository has no canonical GitLab identity")
	}
	if !strings.HasPrefix(host, "https://") && !strings.HasPrefix(host, "http://") {
		return "", fmt.Errorf("repository has invalid GitLab host")
	}
	return fmt.Sprintf("%s/%s/%s/-/merge_requests/%d", host, owner, name, number), nil
}
