package gitlab

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
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
	if !sameConfiguredOrigin(origin, configuredHost) {
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

// parseGitLabRemoteURLIdentity extracts a normalized (host origin, project
// path) pair from a git remote URL, accepting HTTPS, ssh://, and scp-style
// (git@host:path) forms. SSH remotes are normalized to an HTTPS-style origin
// so they can be host-compared against a configured GitLab connection host,
// which is always stored as an HTTP(S) origin. Returns empty strings when the
// URL cannot be parsed as an owner/repo remote.
func parseGitLabRemoteURLIdentity(remoteURL string) (host, projectPath string) {
	const sshScheme = "ssh"
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return "", ""
	}
	scheme, hostname, path, ok := splitRemoteURLIdentity(remoteURL)
	if !ok {
		return "", ""
	}
	scheme = strings.ToLower(scheme)
	if scheme != mentionHTTPScheme && scheme != mentionHTTPSScheme && scheme != sshScheme {
		return "", ""
	}
	if scheme == sshScheme {
		scheme = mentionHTTPSScheme
	}
	path = strings.TrimSuffix(strings.Trim(strings.TrimSpace(path), "/"), ".git")
	if hostname == "" || path == "" || !strings.Contains(path, "/") {
		return "", ""
	}
	return scheme + "://" + hostname, path
}

// splitRemoteURLIdentity resolves the (scheme, hostname, path) triple for
// either a scp-style shorthand (git@host:path) or a URL-form remote
// (https://, ssh://, etc). Extracted from parseGitLabRemoteURLIdentity to
// keep its cyclomatic complexity within the linter budget.
func splitRemoteURLIdentity(remoteURL string) (scheme, hostname, path string, ok bool) {
	const sshScheme = "ssh"
	if strings.Contains(remoteURL, "@") && !strings.Contains(remoteURL, "://") {
		// scp-style shorthand: git@host:group/sub/project.git (host may be a
		// bracketed IPv6 literal, e.g. git@[::1]:group/project.git, whose
		// embedded colons must not be mistaken for the host/path separator).
		_, rest, _ := strings.Cut(remoteURL, "@")
		h, p, cutOK := cutSCPHostPath(rest)
		if !cutOK {
			return "", "", "", false
		}
		return sshScheme, h, p, true
	}
	parsed, err := url.Parse(remoteURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "", "", false
	}
	hostname = remoteHost(parsed)
	if strings.EqualFold(parsed.Scheme, sshScheme) {
		// SSH transport ports (e.g. ssh://git@host:2222/...) are unrelated to
		// the GitLab web origin's port, so compare hostname only. Bracket
		// IPv6 literals so the synthesized origin stays a valid URL (an
		// unbracketed "https://::1" is not).
		hostname = bracketedHostname(parsed.Hostname())
	}
	return parsed.Scheme, hostname, parsed.Path, true
}

// cutSCPHostPath splits an scp-style remote's "host:path" segment (the part
// after "git@"), honoring bracketed IPv6 literals such as
// "[::1]:group/project.git" whose embedded colons would otherwise be
// mistaken for the host/path separator by a plain strings.Cut on ":".
func cutSCPHostPath(rest string) (host, path string, ok bool) {
	if strings.HasPrefix(rest, "[") {
		closeIdx := strings.Index(rest, "]")
		if closeIdx < 0 || closeIdx+1 >= len(rest) || rest[closeIdx+1] != ':' {
			return "", "", false
		}
		return rest[:closeIdx+1], rest[closeIdx+2:], true
	}
	return strings.Cut(rest, ":")
}

func remoteHost(parsed *url.URL) string {
	hostname := parsed.Hostname()
	if hostname == "" {
		return ""
	}
	if port := parsed.Port(); port != "" {
		return net.JoinHostPort(hostname, port)
	}
	return bracketedHostname(hostname)
}

// bracketedHostname wraps an IPv6 literal in brackets (as required for a
// valid URL host) and returns other hostnames unchanged.
func bracketedHostname(hostname string) string {
	if hostname == "" || !strings.Contains(hostname, ":") {
		return hostname
	}
	return "[" + hostname + "]"
}

// AssociateExistingMRByURL validates a workspace-owned task/repository pair,
// fetches the configured-host MR, and idempotently persists its association.
// `repositoryID` is repositories.ID (or empty to auto-resolve the task's
// single repository) — the same id space AssociatePRWithTask uses on the
// GitHub side. A row id from task_repositories instead fails closed here
// (ValidateTaskMRRepositoryIdentity / ResolveTaskMRRepository reject it)
// rather than silently duplicating the association.
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
	s.reconcileComparisonTarget(ctx, taskID, client.Host(), status.MR)
	s.publishTaskMRUpdated(ctx, workspaceID, association)
	return association, nil
}

// AssociateExistingMRByURLForSession wraps AssociateExistingMRByURL for
// callers that have a concrete session to key a refresh watch with — the
// Create-MR action and manual URL linking triggered from a session's git
// activity. This mirrors GitHub's split between AssociatePRByURLForWorkspace
// (session-aware, creates a watch) and AssociateExistingPRByURLForWorkspace
// (the workspace-level HTTP endpoint, no session, no watch). Watch creation
// is best-effort: the association already succeeded, so a watch failure is
// logged rather than surfaced, matching ensureWatchForLinkedMR's convention.
func (s *Service) AssociateExistingMRByURLForSession(
	ctx context.Context,
	workspaceID, sessionID, taskID, repositoryID, mrURL string,
) (*TaskMR, error) {
	association, err := s.AssociateExistingMRByURL(ctx, workspaceID, taskID, repositoryID, mrURL)
	if err != nil {
		return nil, err
	}
	// A cancelable ctx (e.g. an HTTP request context that times out right
	// after the association commits) must not skip this: the association
	// already succeeded and is returned as a success below, so a canceled
	// EnsureMRWatch would silently leave that MR with no refresh watch until
	// another push recreates it. Detach from ctx's cancellation the same way
	// DeleteReviewWatch does for its own post-commit side effect, but bound
	// it with the same timeout so a stalled store call can't hang forever.
	watchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), watchDeleteTimeout)
	defer cancel()
	if _, err := s.EnsureMRWatch(
		watchCtx, sessionID, taskID, association.RepositoryID, association.ProjectPath,
		association.MRIID, association.HeadBranch,
	); err != nil {
		s.logger.Warn("failed to ensure MR watch after URL association",
			zap.String("session_id", sessionID),
			zap.String("task_id", taskID),
			zap.Int("mr_iid", association.MRIID),
			zap.Error(err))
	}
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

// IsConfiguredGitLabHost reports whether remoteURL's host matches the
// workspace's own configured GitLab connection, self-managed or gitlab.com.
// Used by push-detection provider routing to recognize self-managed GitLab
// repositories: unlike github.com/gitlab.com, they never get a durable
// "gitlab" provider tag at discovery time (see
// task/service.resolveRepositoryProviderIdentity), so remote_url is their
// only durable identity signal, and it must be compared against the actual
// configured host rather than a github.com/gitlab.com hostname allowlist.
//
// Best-effort: returns false on any lookup failure (unconfigured workspace,
// unparsable remote) rather than erroring, since callers use this only to
// pick a routing branch — ValidateTaskMRRepositoryIdentity is still the real
// security boundary and still runs before any GitLab API call.
func (s *Service) IsConfiguredGitLabHost(ctx context.Context, workspaceID, remoteURL string) bool {
	host, _ := parseGitLabRemoteURLIdentity(remoteURL)
	if host == "" {
		return false
	}
	cfg, err := s.GetConfigForWorkspace(ctx, workspaceID)
	if err != nil || cfg == nil || cfg.Host == "" {
		return false
	}
	return sameGitLabHost(host, cfg.Host)
}

// UnlinkTaskMR removes one association and its matching refresh watch without
// mutating the task, other associations, or the upstream merge request.
func (s *Service) UnlinkTaskMR(ctx context.Context, workspaceID, associationID string) error {
	store := s.requireStore()
	if store == nil {
		return errors.New("gitlab store not configured")
	}
	association, lookupErr := store.GetTaskMRByID(ctx, associationID)
	if lookupErr != nil {
		return lookupErr
	}
	if association != nil && s.comparisonTargetObserver != nil && association.RepositoryID != "" {
		if err := s.comparisonTargetObserver.RemoveComparisonTargetForChange(
			ctx,
			association.TaskID,
			association.RepositoryID,
			taskmodels.ComparisonTargetProviderGitLab,
			taskmodels.ComparisonTargetKindMergeRequest,
			association.MRIID,
		); err != nil {
			if s.logger != nil {
				s.logger.Warn("GitLab comparison target detach cleanup failed",
					zap.String("task_id", association.TaskID), zap.Int("mr_iid", association.MRIID), zap.Error(err))
			}
			return err
		}
	}
	if err := store.DeleteTaskMRForWorkspace(ctx, workspaceID, associationID); err != nil {
		return err
	}
	return nil
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
		DetailedMergeStatus: status.DetailedMergeStatus, ReviewerCount: status.ReviewerCount,
		UnapprovedReviewers: status.UnapprovedReviewers,
		// UnresolvedDiscussions is deliberately NOT taken from status (it is
		// always 0 there — GetMRStatus skips discussions, see MRStatus's type
		// doc). The auto-fix/auto-merge evaluation pass persists it separately
		// via Store.UpdateTaskMRUnresolvedDiscussions for automation-subscribed
		// MRs only; overwriting it here on every lifecycle sync would clobber
		// that value back to 0 on the very next poll.
	}
}

// TaskMRUpdatedEvent is the payload published on events.GitLabTaskMRUpdated.
// Exported (not just the embedded *TaskMR) so orchestrator-side consumers —
// notably the MR lifecycle automation pass — can type-assert event.Data
// without reaching into an unexported gitlab-package type.
type TaskMRUpdatedEvent struct {
	WorkspaceID    string       `json:"workspace_id"`
	Reviewers      []MRReviewer `json:"reviewers"`
	ReviewersValid bool         `json:"reviewers_valid,omitempty"`
	*TaskMR
}

// GetWorkspaceID implements the websocket broadcaster's workspace-routing
// interface (internal/gateway/websocket.extractWorkspaceID). Without it, the
// broadcaster's map-only field extractor cannot see WorkspaceID on this
// struct payload and always treats the event as workspace-unknown.
func (e *TaskMRUpdatedEvent) GetWorkspaceID() string {
	if e == nil {
		return ""
	}
	return e.WorkspaceID
}

// publishTaskMRLifecycleSyncEvent publishes a TaskMRUpdatedEvent after the
// poller's lifecycle sync pass refreshes a linked MR (AC22). Unlike
// publishTaskMRUpdated (an HTTP/link-flow caller that already has a trusted
// workspace ID), this path resolves the workspace itself — a lookup failure
// skips publishing rather than emitting an event the websocket layer could
// broadcast instance-wide; the next poll retries.
func (s *Service) publishTaskMRLifecycleSyncEvent(
	ctx context.Context, mr *TaskMR, reviewers []MRReviewer, reviewersValid bool,
) {
	if mr == nil {
		return
	}
	s.mu.RLock()
	eventBus := s.eventBus
	store := s.store
	s.mu.RUnlock()
	if eventBus == nil || store == nil {
		return
	}
	workspaceID, err := store.WorkspaceIDForTask(ctx, mr.TaskID)
	if err != nil {
		s.logger.Debug("gitlab: resolve workspace for MR lifecycle sync event",
			zap.String("task_id", mr.TaskID), zap.Error(err))
		return
	}
	event := bus.NewEvent(events.GitLabTaskMRUpdated, eventSource, &TaskMRUpdatedEvent{
		WorkspaceID:    workspaceID,
		Reviewers:      reviewers,
		ReviewersValid: reviewersValid,
		TaskMR:         mr,
	})
	if err := eventBus.Publish(ctx, events.GitLabTaskMRUpdated, event); err != nil {
		s.logger.Debug("publish GitLab task MR lifecycle sync event", zap.Error(err))
	}
}

func (s *Service) publishTaskMRUpdated(ctx context.Context, workspaceID string, association *TaskMR) {
	s.mu.RLock()
	eventBus := s.eventBus
	s.mu.RUnlock()
	if eventBus == nil {
		return
	}
	event := bus.NewEvent(events.GitLabTaskMRUpdated, eventSource, &TaskMRUpdatedEvent{
		WorkspaceID: workspaceID,
		TaskMR:      association,
	})
	if err := eventBus.Publish(ctx, events.GitLabTaskMRUpdated, event); err != nil {
		s.logger.Debug("publish GitLab task MR update", zap.Error(err))
	}
}
