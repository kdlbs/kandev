package gitlab

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

var (
	// ErrInvalidMRURL marks malformed MR URLs and URLs outside the configured host.
	ErrInvalidMRURL = errors.New("invalid GitLab merge request URL")
	// ErrTaskMRNotFound covers absent and cross-workspace task-MR resources.
	ErrTaskMRNotFound = errors.New("task merge request association not found")
	// ErrTaskMRRepositoryRequired prevents ambiguous links on multi-repository tasks.
	ErrTaskMRRepositoryRequired = errors.New("repository_id is required for multi-repository tasks")
	// ErrTaskMRRepositoryMismatch prevents an MR from being linked to a repository
	// whose durable provider origin and project identity do not match the MR.
	ErrTaskMRRepositoryMismatch = errors.New("repository does not match GitLab merge request")
)

func parseMRURLForHost(rawURL, configuredHost string) (string, int, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return "", 0, ErrInvalidMRURL
	}
	origin, err := normalizeHostOrigin(parsed.Scheme + "://" + parsed.Host)
	if err != nil {
		return "", 0, ErrInvalidMRURL
	}
	expectedOrigin, err := normalizeHostOrigin(configuredHost)
	if err != nil || !strings.EqualFold(origin, expectedOrigin) {
		return "", 0, fmt.Errorf("%w: host does not match workspace connection", ErrInvalidMRURL)
	}
	const marker = "/-/merge_requests/"
	path := strings.TrimRight(parsed.Path, "/")
	markerIndex := strings.LastIndex(path, marker)
	if markerIndex <= 0 {
		return "", 0, ErrInvalidMRURL
	}
	projectPath := strings.Trim(path[:markerIndex], "/")
	iidText := path[markerIndex+len(marker):]
	if projectPath == "" || iidText == "" || strings.Contains(iidText, "/") {
		return "", 0, ErrInvalidMRURL
	}
	iid, err := strconv.Atoi(iidText)
	if err != nil || iid <= 0 {
		return "", 0, ErrInvalidMRURL
	}
	return projectPath, iid, nil
}

// AssociateExistingMRByURL validates a workspace-owned task/repository pair,
// fetches the configured-host MR, and idempotently persists its association.
func (s *Service) AssociateExistingMRByURL(
	ctx context.Context,
	workspaceID, taskID, repositoryID, mrURL string,
) (*TaskMR, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errors.New("gitlab store not configured")
	}
	client, err := s.ClientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	projectPath, iid, err := parseMRURLForHost(mrURL, client.Host())
	if err != nil {
		return nil, err
	}
	repositoryID, err = store.ResolveTaskMRRepository(ctx, workspaceID, taskID, repositoryID)
	if err != nil {
		return nil, err
	}
	if err := store.ValidateTaskMRRepositoryIdentity(
		ctx, workspaceID, taskID, repositoryID, client.Host(), projectPath,
	); err != nil {
		return nil, err
	}
	status, err := client.GetMRStatus(ctx, projectPath, iid)
	if err != nil {
		return nil, fmt.Errorf("fetch merge request: %w", err)
	}
	if err := validateReturnedMRIdentity(status, client.Host(), projectPath, iid); err != nil {
		return nil, ErrTaskMRNotFound
	}
	association := taskMRFromStatus(taskID, repositoryID, client.Host(), projectPath, status)
	if err := store.UpsertTaskMR(ctx, association); err != nil {
		return nil, fmt.Errorf("upsert task MR: %w", err)
	}
	s.publishTaskMRUpdated(ctx, workspaceID, association)
	return association, nil
}

func validateReturnedMRIdentity(status *MRStatus, host, projectPath string, iid int) error {
	if status == nil || status.MR == nil {
		return ErrTaskMRNotFound
	}
	returnedProjectPath, returnedIID, err := parseMRURLForHost(status.MR.WebURL, host)
	if err != nil || status.MR.IID != iid || returnedIID != iid {
		return ErrTaskMRNotFound
	}
	if !strings.EqualFold(returnedProjectPath, projectPath) {
		return ErrTaskMRNotFound
	}
	if status.MR.ProjectPath != "" &&
		!strings.EqualFold(strings.Trim(status.MR.ProjectPath, "/"), projectPath) {
		return ErrTaskMRNotFound
	}
	return nil
}

// UnlinkTaskMR removes one association and its matching refresh watch without
// mutating the task, other associations, or the upstream merge request.
func (s *Service) UnlinkTaskMR(ctx context.Context, workspaceID, associationID string) error {
	store := s.requireStore()
	if store == nil {
		return errors.New("gitlab store not configured")
	}
	return store.DeleteTaskMRForWorkspace(ctx, workspaceID, associationID)
}

func taskMRFromStatus(taskID, repositoryID, host, projectPath string, status *MRStatus) *TaskMR {
	mr := status.MR
	now := time.Now().UTC()
	return &TaskMR{
		TaskID: taskID, RepositoryID: repositoryID, Host: host,
		ProjectPath: projectPath, MRIID: mr.IID, MRURL: mr.WebURL, MRTitle: mr.Title,
		HeadBranch: mr.HeadBranch, BaseBranch: mr.BaseBranch, AuthorUsername: mr.AuthorUsername,
		State: mr.State, ApprovalState: status.ApprovalState, PipelineState: status.PipelineState,
		MergeStatus: status.MergeStatus, Draft: mr.Draft, ApprovalCount: status.ApprovalCount,
		RequiredApprovals: status.RequiredApprovals, PipelineJobsTotal: status.PipelineJobsTotal,
		PipelineJobsPass: status.PipelineJobsPassing, CreatedAt: mr.CreatedAt, MergedAt: mr.MergedAt,
		ClosedAt: mr.ClosedAt, LastSyncedAt: &now,
	}
}

type taskMRUpdatedEvent struct {
	WorkspaceID string `json:"workspace_id"`
	*TaskMR
}

func (s *Service) publishTaskMRUpdated(ctx context.Context, workspaceID string, association *TaskMR) {
	s.mu.RLock()
	eventBus := s.eventBus
	s.mu.RUnlock()
	if eventBus == nil {
		return
	}
	event := bus.NewEvent(events.GitLabTaskMRUpdated, eventSource, &taskMRUpdatedEvent{
		WorkspaceID: workspaceID,
		TaskMR:      association,
	})
	if err := eventBus.Publish(ctx, events.GitLabTaskMRUpdated, event); err != nil {
		s.logger.Debug("publish GitLab task MR update", zap.Error(err))
	}
}
