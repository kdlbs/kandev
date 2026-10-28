package github

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"go.uber.org/zap"
)

type TaskPRObservationCommitter func(context.Context, *TaskPR, bool, int64) (*TaskPR, bool, error)

// GetTaskPRByRegistrationIdentityIncludingDetached reads the durable
// association row used to reconcile message status after a missed detach
// event.
func (s *Service) GetTaskPRByRegistrationIdentityIncludingDetached(
	ctx context.Context, taskID, providerHost, connectionScope, providerRepositoryID string, prNumber int,
) (*TaskPR, error) {
	if s == nil || s.store == nil {
		return nil, errStoreUnavailable
	}
	return s.store.GetTaskPRByRegistrationIdentityIncludingDetached(
		ctx, taskID, providerHost, connectionScope, providerRepositoryID, prNumber)
}

// RegisterExistingPRByURLForWorkspace stores a verified PR as an observation-only
// task association. It never selects a comparison target or enables automation.
func (s *Service) RegisterExistingPRByURLForWorkspace(
	ctx context.Context, workspaceID, userID, taskID, repositoryID, prURL string,
	restoreDetached bool, operationID string,
) (*TaskPR, error) {
	return s.RegisterExistingPRByURLForWorkspaceWithCommit(ctx, workspaceID, userID, taskID, repositoryID, prURL,
		restoreDetached, operationID, func(ctx context.Context, registration *TaskPR, restore bool, expectedRevision int64) (*TaskPR, bool, error) {
			return s.store.RegisterTaskPRObservation(ctx, registration, restore, expectedRevision)
		})
}

// RegisterExistingPRByURLForWorkspaceWithCommit separates provider reads from
// the caller-owned association commit. The committer runs only after the PR
// identity has been verified and must return after its transaction commits.
func (s *Service) RegisterExistingPRByURLForWorkspaceWithCommit(
	ctx context.Context, workspaceID, userID, taskID, repositoryID, prURL string,
	restoreDetached bool, operationID string, commit TaskPRObservationCommitter,
) (*TaskPR, error) {
	return s.registerExistingPRByURLForWorkspaceWithCommit(ctx, workspaceID, userID, taskID, repositoryID,
		prURL, restoreDetached, operationID, nil, commit)
}

// RegisterExistingPRByURLForWorkspaceAtRevision uses the association revision
// observed when the user selected a message reference. Retries keep that fence,
// so a later unlink cannot be undone by background recovery.
func (s *Service) RegisterExistingPRByURLForWorkspaceAtRevision(
	ctx context.Context, workspaceID, userID, taskID, repositoryID, prURL string,
	expectedRevision int64, restoreDetached bool, operationID string, commit TaskPRObservationCommitter,
) (*TaskPR, error) {
	return s.registerExistingPRByURLForWorkspaceWithCommit(ctx, workspaceID, userID, taskID, repositoryID,
		prURL, restoreDetached, operationID, &expectedRevision, commit)
}

func (s *Service) registerExistingPRByURLForWorkspaceWithCommit(
	ctx context.Context, workspaceID, userID, taskID, repositoryID, prURL string,
	restoreDetached bool, operationID string, acceptedRevision *int64, commit TaskPRObservationCommitter,
) (*TaskPR, error) {
	if s == nil || s.store == nil {
		return nil, errStoreUnavailable
	}
	if commit == nil {
		return nil, errors.New("GitHub registration committer is required")
	}
	target, err := s.resolveTaskPRRegistrationTarget(ctx, workspaceID, taskID, repositoryID, prURL)
	if err != nil {
		return nil, err
	}
	if restoreDetached && strings.TrimSpace(operationID) == "" {
		return nil, fmt.Errorf("explicit restoration requires an operation ID")
	}
	current, expectedRevision, err := s.taskPRRegistrationFence(ctx, workspaceID, taskID, target, acceptedRevision, restoreDetached)
	if err != nil {
		return nil, err
	}
	if current != nil && current.DetachedAt == nil {
		return current, nil
	}
	pr, err := s.fetchTaskPRForRegistration(ctx, workspaceID, userID, target)
	if err != nil {
		return nil, err
	}
	if operationID == "" {
		operationID = fmt.Sprintf("agent-registration:%s:%d", taskID, time.Now().UTC().UnixNano())
	}
	registration := taskPRObservationFromProviderPR(workspaceID, taskID, target, operationID, expectedRevision, pr, prURL)
	registered, changed, err := commit(ctx, registration, restoreDetached, expectedRevision)
	if err != nil {
		return nil, err
	}
	if changed {
		s.publishRegisteredTaskPR(ctx, registered)
	}
	return registered, nil
}

type taskPRRegistrationTarget struct {
	repositoryID  string
	owner         string
	repository    string
	providerID    string
	canonicalHost string
	number        int
}

func (s *Service) resolveTaskPRRegistrationTarget(
	ctx context.Context,
	workspaceID, taskID, repositoryID, prURL string,
) (taskPRRegistrationTarget, error) {
	var target taskPRRegistrationTarget
	owner, repo, number, err := parsePRURL(prURL)
	if err != nil {
		return target, fmt.Errorf("%w: %w", ErrInvalidPRURL, err)
	}
	if err := s.validateTaskRepositoryID(ctx, taskID, repositoryID); err != nil {
		return target, err
	}
	taskStore := s.getTaskIssueStore()
	if taskStore == nil {
		return target, errStoreUnavailable
	}
	repository, err := taskStore.GetRepository(ctx, repositoryID)
	if err != nil || repository == nil || repository.WorkspaceID != workspaceID || !strings.EqualFold(repository.Provider, "github") {
		return target, ErrTaskPRRepositoryMismatch
	}
	providerID := strings.TrimSpace(repository.ProviderRepoID)
	if providerID == "" {
		return target, ErrTaskPRRegistrationIdentityUnresolved
	}
	canonicalHost, err := normalizeRegistrationGitHubHost(repository.ProviderHost)
	if err != nil || !strings.EqualFold(canonicalHost, strings.ToLower(strings.TrimSuffix(parsedPRHost(prURL), "."))) ||
		!strings.EqualFold(strings.Trim(repository.ProviderOwner, "/"), owner) ||
		!strings.EqualFold(strings.Trim(repository.ProviderName, "/"), repo) {
		return target, ErrTaskPRRepositoryMismatch
	}
	return taskPRRegistrationTarget{
		repositoryID: repositoryID, owner: owner, repository: repo, providerID: providerID,
		canonicalHost: canonicalHost, number: number,
	}, nil
}

func (s *Service) taskPRRegistrationFence(
	ctx context.Context,
	workspaceID, taskID string,
	target taskPRRegistrationTarget,
	acceptedRevision *int64,
	restoreDetached bool,
) (*TaskPR, int64, error) {
	current, err := s.store.GetTaskPRByRegistrationIdentityIncludingDetached(
		ctx, taskID, "https://"+target.canonicalHost, "builtin:"+workspaceID, target.providerID, target.number)
	if err == nil && current == nil {
		current, err = s.store.GetTaskPRByRepoAndNumberIncludingDetached(ctx, taskID, target.repositoryID, target.number)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("read current GitHub PR association: %w", err)
	}
	if current == nil {
		if acceptedRevision != nil && *acceptedRevision != 0 {
			return nil, 0, ErrTaskPRRegistrationRevisionChanged
		}
		return nil, acceptedRegistrationRevision(acceptedRevision), nil
	}
	if strings.TrimSpace(current.ProviderRepositoryID) == "" {
		return nil, 0, ErrTaskPRRegistrationIdentityUnresolved
	}
	if current.ProviderRepositoryID != target.providerID || !strings.EqualFold(current.ProviderHost, "https://"+target.canonicalHost) ||
		current.ConnectionScope != "builtin:"+workspaceID {
		return nil, 0, ErrTaskPRRegistrationIdentityConflict
	}
	if acceptedRevision != nil && current.AssociationRevision != *acceptedRevision {
		return nil, 0, ErrTaskPRRegistrationRevisionChanged
	}
	if current.DetachedAt != nil && !restoreDetached {
		return nil, 0, ErrTaskPRRegistrationDetached
	}
	return current, current.AssociationRevision, nil
}

func acceptedRegistrationRevision(revision *int64) int64 {
	if revision == nil {
		return 0
	}
	return *revision
}

func (s *Service) fetchTaskPRForRegistration(
	ctx context.Context,
	workspaceID, userID string,
	target taskPRRegistrationTarget,
) (*PR, error) {
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, target.owner, target.repository); err != nil {
		return nil, err
	}
	resolved, err := s.resolvePersonalReadClient(ctx, workspaceID, userID, target.owner, target.repository)
	if err != nil {
		return nil, err
	}
	pr, err := resolved.Client.GetPR(ctx, target.owner, target.repository, target.number)
	if err != nil {
		return nil, fmt.Errorf("fetch GitHub PR for registration: %w", err)
	}
	if err := validateRegistrationPRIdentity(pr, target.providerID, target.owner, target.repository, target.number); err != nil {
		return nil, err
	}
	return pr, nil
}

func taskPRObservationFromProviderPR(
	workspaceID, taskID string,
	target taskPRRegistrationTarget,
	operationID string,
	expectedRevision int64,
	pr *PR,
	requestedURL string,
) *TaskPR {
	if pr.HTMLURL == "" {
		pr.HTMLURL = requestedURL
	}
	now := time.Now().UTC()
	return &TaskPR{
		WorkspaceID: workspaceID, TaskID: taskID, RepositoryID: target.repositoryID,
		Owner: pr.RepoOwner, Repo: pr.RepoName, PRNumber: target.number, PRURL: pr.HTMLURL, PRTitle: pr.Title,
		HeadBranch: pr.HeadBranch, BaseBranch: pr.BaseBranch, HeadSHA: pr.HeadSHA,
		AuthorLogin: pr.AuthorLogin, State: pr.State, MergeableState: effectivePRMergeableState(pr),
		HasMergeConflicts: observedTaskPRMergeConflict(nil, pr, pr.MergeableState),
		Additions:         pr.Additions, Deletions: pr.Deletions, CreatedAt: pr.CreatedAt,
		MergedAt: pr.MergedAt, ClosedAt: pr.ClosedAt, LastSyncedAt: &now,
		Source: TaskPRSourceAgentRegistration, ProviderHost: "https://" + target.canonicalHost,
		ConnectionScope: "builtin:" + workspaceID, ProviderRepositoryID: target.providerID,
		RepositoryPath:   strings.Trim(target.owner+"/"+target.repository, "/"),
		ComparisonPolicy: TaskPRComparisonPolicyPreserve, AssociationRevision: expectedRevision,
		FirstOriginKind: "agent_registration", FirstOriginID: operationID,
		FirstOriginRevision: strconv.FormatInt(expectedRevision, 10),
	}
}

func (s *Service) publishRegisteredTaskPR(ctx context.Context, registered *TaskPR) {
	if s.eventBus == nil {
		return
	}
	event := bus.NewEvent(events.GitHubTaskPRUpdated, "github", registered)
	if err := s.eventBus.Publish(ctx, events.GitHubTaskPRUpdated, event); err != nil && s.logger != nil {
		s.logger.Debug("failed to publish registered GitHub PR update", zap.Error(err))
	}
}

func (s *Service) RegisterTaskPRObservationTx(
	ctx context.Context, tx *sqlx.Tx, registration *TaskPR, restoreDetached bool, expectedRevision int64,
) (*TaskPR, bool, error) {
	if s == nil || s.store == nil {
		return nil, false, errStoreUnavailable
	}
	return s.store.RegisterTaskPRObservationTx(ctx, tx, registration, restoreDetached, expectedRevision)
}

func parsedPRHost(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil {
		return ""
	}
	return parsed.Hostname()
}

func normalizeRegistrationGitHubHost(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", fmt.Errorf("GitHub repository host is required")
	}
	if !strings.Contains(host, "://") {
		host = "https://" + strings.TrimPrefix(host, "//")
	}
	parsed, err := url.Parse(host)
	if err != nil || parsed == nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Host == "" ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Port() != "" {
		return "", fmt.Errorf("invalid GitHub repository host")
	}
	name := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if name != defaultGitHubHost && !strings.HasSuffix(name, "."+defaultGitHubHost) {
		return "", fmt.Errorf("repository host is not GitHub")
	}
	return name, nil
}

func validateRegistrationPRIdentity(pr *PR, providerRepositoryID, owner, repo string, number int) error {
	if pr == nil || pr.Number != number || !strings.EqualFold(pr.RepoOwner, owner) ||
		!strings.EqualFold(pr.RepoName, repo) || pr.BaseRepoID <= 0 ||
		strconv.FormatInt(pr.BaseRepoID, 10) != providerRepositoryID {
		return ErrTaskPRRepositoryMismatch
	}
	return nil
}
