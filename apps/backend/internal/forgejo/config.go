package forgejo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

var (
	ErrWorkspaceRequired   = errors.New("forgejo: workspace_id required")
	ErrNotConfigured       = errors.New("forgejo: workspace not configured")
	ErrUnsupported         = errors.New("forgejo: capability unavailable on this Forgejo client")
	ErrHeadBranchNotPushed = errors.New("forgejo: pull request source branch is not pushed")
)

type Config struct {
	WorkspaceID      string     `json:"workspace_id" db:"workspace_id"`
	Origin           string     `json:"origin" db:"origin"`
	Username         string     `json:"username" db:"username"`
	HasSecret        bool       `json:"has_secret" db:"-"`
	HasWebhookSecret bool       `json:"has_webhook_secret" db:"-"`
	LastOK           bool       `json:"last_ok" db:"last_ok"`
	LastError        string     `json:"last_error,omitempty" db:"last_error"`
	LastCheckedAt    *time.Time `json:"last_checked_at,omitempty" db:"last_checked_at"`
	Revision         int64      `json:"revision" db:"revision"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`
}

type SetConfigRequest struct {
	Origin        string `json:"origin"`
	Token         string `json:"token,omitempty"`
	WebhookSecret string `json:"webhook_secret,omitempty"`
}

type TestConnectionResult struct {
	OK       bool   `json:"ok"`
	Username string `json:"username,omitempty"`
	Error    string `json:"error,omitempty"`
}

type WorkspaceSecretStore interface {
	Reveal(context.Context, string) (string, error)
	Set(context.Context, string, string, string) error
	Delete(context.Context, string) error
	Exists(context.Context, string) (bool, error)
}

func SecretKeyForWorkspace(workspaceID string) string {
	return "forgejo:" + strings.TrimSpace(workspaceID) + ":token"
}

func WebhookSecretKeyForWorkspace(workspaceID string) string {
	return "forgejo:" + strings.TrimSpace(workspaceID) + ":webhook_secret"
}

type Store struct {
	db *sqlx.DB
	ro *sqlx.DB
}

func NewStore(db, ro *sqlx.DB) (*Store, error) {
	if db == nil || ro == nil {
		return nil, errors.New("forgejo store requires database handles")
	}
	store := &Store{db: db, ro: ro}
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS forgejo_configs (
		workspace_id TEXT PRIMARY KEY,
		origin TEXT NOT NULL,
		username TEXT NOT NULL DEFAULT '',
		last_ok INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		last_checked_at DATETIME,
		revision INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	)`)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS forgejo_task_issues (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		repository_id TEXT NOT NULL DEFAULT '',
		origin TEXT NOT NULL,
		owner TEXT NOT NULL,
		repo TEXT NOT NULL,
		issue_number INTEGER NOT NULL,
		issue_url TEXT NOT NULL,
		title TEXT NOT NULL,
		state TEXT NOT NULL,
		last_synced_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		UNIQUE(task_id, repository_id, owner, repo, issue_number)
	)`)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS forgejo_task_prs (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		repository_id TEXT NOT NULL DEFAULT '',
		origin TEXT NOT NULL,
		owner TEXT NOT NULL,
		repo TEXT NOT NULL,
		pr_number INTEGER NOT NULL,
		pr_url TEXT NOT NULL,
		pr_title TEXT NOT NULL,
		head_branch TEXT NOT NULL,
		base_branch TEXT NOT NULL,
		state TEXT NOT NULL,
		draft INTEGER NOT NULL DEFAULT 0,
		mergeable INTEGER NOT NULL DEFAULT 0,
		ci_state TEXT NOT NULL DEFAULT '',
		last_synced_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		UNIQUE(task_id, repository_id, owner, repo, pr_number)
	)`)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS forgejo_issue_watches (
		id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, workflow_id TEXT NOT NULL DEFAULT '', workflow_step_id TEXT NOT NULL DEFAULT '', repository_id TEXT NOT NULL DEFAULT '', base_branch TEXT NOT NULL DEFAULT '', prompt TEXT NOT NULL DEFAULT '', agent_profile_id TEXT NOT NULL DEFAULT '', executor_profile_id TEXT NOT NULL DEFAULT '', cleanup_policy TEXT NOT NULL DEFAULT 'auto', inflight_limit INTEGER NOT NULL DEFAULT 0, owner TEXT NOT NULL, repo TEXT NOT NULL,
		labels TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1,
		poll_interval_seconds INTEGER NOT NULL DEFAULT 300, last_polled_at DATETIME,
		last_error TEXT NOT NULL DEFAULT '', created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL,
		UNIQUE(workspace_id, owner, repo, labels)
	)`)
	// Forward-compatible migrations for databases created before watch task context.
	for _, statement := range []string{
		`ALTER TABLE forgejo_issue_watches ADD COLUMN workflow_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE forgejo_issue_watches ADD COLUMN workflow_step_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE forgejo_issue_watches ADD COLUMN repository_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE forgejo_issue_watches ADD COLUMN base_branch TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE forgejo_issue_watches ADD COLUMN prompt TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE forgejo_issue_watches ADD COLUMN agent_profile_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE forgejo_issue_watches ADD COLUMN executor_profile_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE forgejo_issue_watches ADD COLUMN cleanup_policy TEXT NOT NULL DEFAULT 'auto'`,
		`ALTER TABLE forgejo_issue_watches ADD COLUMN inflight_limit INTEGER NOT NULL DEFAULT 0`,
	} {
		_, _ = db.Exec(statement)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS forgejo_issue_watch_tasks (
		watch_id TEXT NOT NULL, owner TEXT NOT NULL, repo TEXT NOT NULL, issue_number INTEGER NOT NULL,
		task_id TEXT NOT NULL, created_at DATETIME NOT NULL,
		PRIMARY KEY (watch_id, owner, repo, issue_number)
	)`)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS forgejo_webhook_deliveries (
		workspace_id TEXT NOT NULL, delivery_id TEXT NOT NULL, payload_hash TEXT NOT NULL,
		created_at DATETIME NOT NULL, PRIMARY KEY(workspace_id, delivery_id)
	)`)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS forgejo_review_watches (
		id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, workflow_id TEXT NOT NULL DEFAULT '', workflow_step_id TEXT NOT NULL DEFAULT '', repository_id TEXT NOT NULL DEFAULT '', base_branch TEXT NOT NULL DEFAULT '', prompt TEXT NOT NULL DEFAULT '', agent_profile_id TEXT NOT NULL DEFAULT '', owner TEXT NOT NULL, repo TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 1, poll_interval_seconds INTEGER NOT NULL DEFAULT 300, last_polled_at DATETIME, last_error TEXT NOT NULL DEFAULT '', created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, UNIQUE(workspace_id, owner, repo)
	)`)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS forgejo_review_watch_tasks (watch_id TEXT NOT NULL, owner TEXT NOT NULL, repo TEXT NOT NULL, pr_number INTEGER NOT NULL, task_id TEXT NOT NULL DEFAULT '', created_at DATETIME NOT NULL, PRIMARY KEY(watch_id, owner, repo, pr_number))`)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS forgejo_action_presets (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, kind TEXT NOT NULL, name TEXT NOT NULL, instructions TEXT NOT NULL DEFAULT '', created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, UNIQUE(workspace_id, kind, name))`)
	return store, err
}

func (s *Store) GetConfig(ctx context.Context, workspaceID string) (*Config, error) {
	var config Config
	err := s.ro.GetContext(ctx, &config, `SELECT workspace_id, origin, username, last_ok, last_error, last_checked_at, revision, created_at, updated_at FROM forgejo_configs WHERE workspace_id = ?`, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (s *Store) SaveConfig(ctx context.Context, config *Config) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `INSERT INTO forgejo_configs (workspace_id, origin, username, last_ok, last_error, last_checked_at, revision, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET origin = excluded.origin, username = excluded.username, last_ok = excluded.last_ok, last_error = excluded.last_error, last_checked_at = excluded.last_checked_at, revision = forgejo_configs.revision + 1, updated_at = excluded.updated_at`,
		config.WorkspaceID, config.Origin, config.Username, config.LastOK, config.LastError, config.LastCheckedAt, now, now)
	return err
}

func (s *Store) DeleteConfig(ctx context.Context, workspaceID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM forgejo_configs WHERE workspace_id = ?`, workspaceID)
	return err
}

func (s *Store) UpdateHealth(ctx context.Context, workspaceID string, ok bool, message, username string, checkedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE forgejo_configs SET last_ok = ?, last_error = ?, username = CASE WHEN ? <> '' THEN ? ELSE username END, last_checked_at = ?, revision = revision + 1, updated_at = ? WHERE workspace_id = ?`, ok, message, username, username, checkedAt, checkedAt, workspaceID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotConfigured
	}
	return nil
}

type Service struct {
	store             *Store
	secrets           WorkspaceSecretStore
	issueTaskCreator  IssueTaskCreator
	reviewTaskCreator ReviewTaskCreator
	eventBus          bus.EventBus
}

// IssueTaskCreator creates a Kandev task for a watched Forgejo issue. It is
// injected by backendapp to keep this provider package independent of task.
type IssueTaskCreator func(context.Context, *IssueWatch, Issue) (string, error)
type ReviewTaskCreator func(context.Context, *ReviewWatch, PullRequest) (string, error)

func NewService(store *Store, secrets WorkspaceSecretStore) *Service {
	return &Service{store: store, secrets: secrets}
}

func (s *Service) SetIssueTaskCreator(creator IssueTaskCreator) {
	s.issueTaskCreator = creator
}
func (s *Service) SetReviewTaskCreator(creator ReviewTaskCreator) { s.reviewTaskCreator = creator }
func (s *Service) SetEventBus(eventBus bus.EventBus)              { s.eventBus = eventBus }

func (s *Service) publishConfigUpdated(ctx context.Context, config *Config) {
	if s.eventBus == nil || config == nil {
		return
	}
	_ = s.eventBus.Publish(ctx, events.ForgejoConfigUpdated, bus.NewEvent(events.ForgejoConfigUpdated, "forgejo", config))
}

type taskLinksUpdatedEvent struct {
	WorkspaceID string `json:"workspace_id"`
	TaskID      string `json:"task_id"`
}

func (s *Service) publishTaskLinksUpdated(ctx context.Context, workspaceID, taskID string) {
	if s.eventBus == nil || workspaceID == "" || taskID == "" {
		return
	}
	event := bus.NewEvent(events.ForgejoTaskLinksUpdated, "forgejo", &taskLinksUpdatedEvent{WorkspaceID: workspaceID, TaskID: taskID})
	_ = s.eventBus.Publish(ctx, events.ForgejoTaskLinksUpdated, event)
}

func (s *Service) GetConfig(ctx context.Context, workspaceID string) (*Config, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, ErrWorkspaceRequired
	}
	config, err := s.store.GetConfig(ctx, workspaceID)
	if err != nil || config == nil || s.secrets == nil {
		return config, err
	}
	config.HasSecret, err = s.secrets.Exists(ctx, SecretKeyForWorkspace(workspaceID))
	if err != nil {
		return config, err
	}
	config.HasWebhookSecret, err = s.secrets.Exists(ctx, WebhookSecretKeyForWorkspace(workspaceID))
	return config, err
}

// ClientForWorkspace resolves a Forgejo client using only workspace-owned
// metadata and the workspace secret store. The PAT never leaves this package
// through an HTTP response or agent environment.
func (s *Service) ClientForWorkspace(ctx context.Context, workspaceID string) (Client, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, ErrWorkspaceRequired
	}
	if s.secrets == nil {
		return nil, errors.New("Forgejo secret store unavailable")
	}
	config, err := s.store.GetConfig(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, ErrNotConfigured
	}
	token, err := s.secrets.Reveal(ctx, SecretKeyForWorkspace(workspaceID))
	if err != nil || strings.TrimSpace(token) == "" {
		return nil, ErrNotConfigured
	}
	return NewPATClient(config.Origin, token)
}

// RefreshConnection persists the result of a live identity probe. This is
// suitable for a periodic integration-health poller and never exposes a PAT.
func (s *Service) RefreshConnection(ctx context.Context, workspaceID string) (*Config, error) {
	client, err := s.ClientForWorkspace(ctx, workspaceID)
	checkedAt := time.Now().UTC()
	if err != nil {
		return nil, err
	}
	user, err := client.GetAuthenticatedUser(ctx)
	if err != nil {
		_ = s.store.UpdateHealth(ctx, workspaceID, false, "Forgejo connection check failed", "", checkedAt)
		return nil, err
	}
	if err := s.store.UpdateHealth(ctx, workspaceID, true, "", user.Login, checkedAt); err != nil {
		return nil, err
	}
	config, err := s.GetConfig(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func (s *Service) ListRepositories(ctx context.Context, workspaceID string, page, limit int) ([]Repository, int, error) {
	client, err := s.ClientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, 0, err
	}
	return client.ListRepositories(ctx, page, limit)
}

func (s *Service) ListIssues(ctx context.Context, workspaceID, owner, repo string, page, limit int) ([]Issue, int, error) {
	client, err := s.ClientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, 0, err
	}
	return client.ListIssues(ctx, owner, repo, page, limit)
}

func (s *Service) ListPullRequests(ctx context.Context, workspaceID, owner, repo string, page, limit int) ([]PullRequest, int, error) {
	client, err := s.ClientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, 0, err
	}
	return client.ListPullRequests(ctx, owner, repo, page, limit)
}

type PullRequestDetails struct {
	Owner       string               `json:"owner"`
	Repo        string               `json:"repo"`
	PullRequest *PullRequest         `json:"pull_request"`
	Commits     []PullRequestCommit  `json:"commits"`
	Files       []PullRequestFile    `json:"files"`
	Comments    []PullRequestComment `json:"comments"`
	Reviews     []PullRequestReview  `json:"reviews"`
	ActionRuns  []ActionRun          `json:"action_runs"`
}

func (s *Service) GetPullRequestDetails(ctx context.Context, workspaceID, owner, repo string, number int) (*PullRequestDetails, error) {
	client, err := s.ClientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	detailsClient, ok := client.(PullRequestDetailsClient)
	if !ok {
		return nil, ErrUnsupported
	}
	pull, err := client.GetPullRequest(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	commits, err := detailsClient.ListPullRequestCommits(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	files, err := detailsClient.ListPullRequestFiles(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	comments, err := detailsClient.ListPullRequestComments(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	reviews, err := detailsClient.ListPullRequestReviews(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	var runs []ActionRun
	if actions, ok := client.(ActionsClient); ok {
		runs, _, err = actions.ListActionRuns(ctx, owner, repo, 1, 100)
		if err != nil {
			return nil, err
		}
	}
	return &PullRequestDetails{Owner: owner, Repo: repo, PullRequest: pull, Commits: commits, Files: files, Comments: comments, Reviews: reviews, ActionRuns: filterActionRunsForBranch(runs, pull.Head)}, nil
}

func filterActionRunsForBranch(runs []ActionRun, branch string) []ActionRun {
	result := make([]ActionRun, 0, len(runs))
	for _, run := range runs {
		if run.HeadBranch == branch {
			result = append(result, run)
		}
	}
	return result
}

func (s *Service) CreatePullRequestComment(ctx context.Context, workspaceID, owner, repo string, number int, body string) (*PullRequestComment, error) {
	client, err := s.ClientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	writer, ok := client.(PullRequestReviewWriter)
	if !ok {
		return nil, ErrUnsupported
	}
	return writer.CreatePullRequestComment(ctx, owner, repo, number, body)
}

func (s *Service) SubmitPullRequestReview(ctx context.Context, workspaceID string, input SubmitPullRequestReviewInput) (*PullRequestReview, error) {
	client, err := s.ClientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	writer, ok := client.(PullRequestReviewWriter)
	if !ok {
		return nil, ErrUnsupported
	}
	return writer.SubmitPullRequestReview(ctx, input)
}

type QueueIssue struct {
	Repository Repository `json:"repository"`
	Issue      Issue      `json:"issue"`
}
type QueuePullRequest struct {
	Repository  Repository  `json:"repository"`
	PullRequest PullRequest `json:"pull_request"`
}

// ListWorkspaceQueue provides the personal Forgejo queue for a workspace by
// collecting open issues and pull requests from every repository visible to
// the configured token. It is deliberately bounded to one server-sized page
// per repository; future saved watches provide narrower, persistent scopes.
func (s *Service) ListWorkspaceQueue(ctx context.Context, workspaceID string) ([]QueueIssue, []QueuePullRequest, error) {
	client, err := s.ClientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	repositories, _, err := client.ListRepositories(ctx, 1, 100)
	if err != nil {
		return nil, nil, err
	}
	issues := make([]QueueIssue, 0)
	pulls := make([]QueuePullRequest, 0)
	for _, repository := range repositories {
		repositoryIssues, _, issueErr := client.ListIssues(ctx, repository.Owner, repository.Name, 1, 100)
		if issueErr != nil {
			return nil, nil, fmt.Errorf("list queue issues for %s: %w", repository.FullName, issueErr)
		}
		for _, issue := range repositoryIssues {
			issues = append(issues, QueueIssue{Repository: repository, Issue: issue})
		}
		repositoryPulls, _, pullErr := client.ListPullRequests(ctx, repository.Owner, repository.Name, 1, 100)
		if pullErr != nil {
			return nil, nil, fmt.Errorf("list queue pull requests for %s: %w", repository.FullName, pullErr)
		}
		for _, pull := range repositoryPulls {
			pulls = append(pulls, QueuePullRequest{Repository: repository, PullRequest: pull})
		}
	}
	return issues, pulls, nil
}

func (s *Service) AssociatePullRequest(ctx context.Context, workspaceID, taskID, repositoryID, owner, repo string, number int) (*TaskPR, error) {
	if err := s.store.assertTaskWorkspace(ctx, workspaceID, taskID); err != nil {
		return nil, err
	}
	client, err := s.ClientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	pull, err := client.GetPullRequest(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	config, err := s.store.GetConfig(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	ciState := ""
	if actions, ok := client.(ActionsClient); ok {
		if runs, _, actionsErr := actions.ListActionRuns(ctx, owner, repo, 1, 100); actionsErr == nil {
			ciState = actionRunState(filterActionRunsForBranch(runs, pull.Head))
		}
	}
	link := &TaskPR{TaskID: taskID, RepositoryID: repositoryID, Origin: config.Origin, Owner: owner, Repo: repo, PRNumber: pull.Number, PRURL: pull.HTMLURL, PRTitle: pull.Title, HeadBranch: pull.Head, BaseBranch: pull.Base, State: pull.State, Draft: pull.Draft, Mergeable: pull.Mergeable, CIState: ciState, LastSyncedAt: &now}
	if err := s.store.UpsertTaskPR(ctx, link); err != nil {
		return nil, err
	}
	links, err := s.store.ListTaskPRs(ctx, workspaceID, taskID)
	if err != nil {
		return nil, err
	}
	for _, candidate := range links {
		if candidate.Owner == owner && candidate.Repo == repo && candidate.PRNumber == pull.Number {
			s.publishTaskLinksUpdated(ctx, workspaceID, taskID)
			return candidate, nil
		}
	}
	return nil, ErrTaskLinkNotFound
}

func actionRunState(runs []ActionRun) string {
	if len(runs) == 0 {
		return ""
	}
	allSuccess := true
	for _, run := range runs {
		if run.Status != "completed" {
			return "pending"
		}
		if run.Conclusion != "success" {
			allSuccess = false
		}
	}
	if allSuccess {
		return "success"
	}
	return "failure"
}

func (s *Service) AssociateIssue(ctx context.Context, workspaceID, taskID, repositoryID, owner, repo string, number int) (*TaskIssue, error) {
	if err := s.store.assertTaskWorkspace(ctx, workspaceID, taskID); err != nil {
		return nil, err
	}
	client, err := s.ClientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	issue, err := client.GetIssue(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	config, err := s.store.GetConfig(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	link := &TaskIssue{TaskID: taskID, RepositoryID: repositoryID, Origin: config.Origin, Owner: owner, Repo: repo, IssueNumber: issue.Number, IssueURL: issue.HTMLURL, Title: issue.Title, State: issue.State, LastSyncedAt: &now}
	if err := s.store.UpsertTaskIssue(ctx, link); err != nil {
		return nil, err
	}
	links, err := s.store.ListTaskIssues(ctx, workspaceID, taskID)
	if err != nil {
		return nil, err
	}
	for _, candidate := range links {
		if candidate.Owner == owner && candidate.Repo == repo && candidate.IssueNumber == issue.Number {
			s.publishTaskLinksUpdated(ctx, workspaceID, taskID)
			return candidate, nil
		}
	}
	return nil, ErrTaskLinkNotFound
}

func (s *Service) CreateTaskPullRequest(ctx context.Context, workspaceID, taskID, repositoryID string, input CreatePullRequestInput) (*TaskPR, error) {
	if err := s.store.assertTaskWorkspace(ctx, workspaceID, taskID); err != nil {
		return nil, err
	}
	client, err := s.ClientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	branches, ok := client.(BranchLookupClient)
	if !ok {
		return nil, ErrUnsupported
	}
	if err := branches.GetBranch(ctx, input.Owner, input.Repo, input.Head); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%w: push %q to Forgejo before creating a pull request", ErrHeadBranchNotPushed, input.Head)
		}
		return nil, fmt.Errorf("verify Forgejo source branch: %w", err)
	}
	pull, err := client.CreatePullRequest(ctx, input)
	if err != nil {
		return nil, err
	}
	config, err := s.store.GetConfig(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	link := &TaskPR{TaskID: taskID, RepositoryID: repositoryID, Origin: config.Origin, Owner: input.Owner, Repo: input.Repo, PRNumber: pull.Number, PRURL: pull.HTMLURL, PRTitle: pull.Title, HeadBranch: pull.Head, BaseBranch: pull.Base, State: pull.State, Draft: pull.Draft, LastSyncedAt: &now}
	if err := s.store.UpsertTaskPR(ctx, link); err != nil {
		return nil, err
	}
	links, err := s.store.ListTaskPRs(ctx, workspaceID, taskID)
	if err != nil {
		return nil, err
	}
	for _, candidate := range links {
		if candidate.Owner == input.Owner && candidate.Repo == input.Repo && candidate.PRNumber == pull.Number {
			s.publishTaskLinksUpdated(ctx, workspaceID, taskID)
			return candidate, nil
		}
	}
	return nil, ErrTaskLinkNotFound
}

func (s *Service) UnlinkTaskIssue(ctx context.Context, workspaceID, linkID string) error {
	link, err := s.store.GetTaskIssueLink(ctx, workspaceID, linkID)
	if err != nil {
		return err
	}
	if err := s.store.DeleteTaskIssue(ctx, workspaceID, linkID); err != nil {
		return err
	}
	s.publishTaskLinksUpdated(ctx, workspaceID, link.TaskID)
	return nil
}

func (s *Service) SaveIssueWatch(ctx context.Context, workspaceID string, watch *IssueWatch) error {
	if watch == nil {
		return errors.New("forgejo issue watch required")
	}
	watch.WorkspaceID = workspaceID
	return s.store.UpsertIssueWatch(ctx, watch)
}
func (s *Service) ListIssueWatches(ctx context.Context, workspaceID string) ([]*IssueWatch, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, ErrWorkspaceRequired
	}
	return s.store.ListIssueWatches(ctx, workspaceID)
}

func (s *Service) ListAllIssueWatches(ctx context.Context) ([]*IssueWatch, error) {
	return s.store.ListAllIssueWatches(ctx)
}
func (s *Service) DeleteIssueWatch(ctx context.Context, workspaceID, id string) error {
	if strings.TrimSpace(workspaceID) == "" {
		return ErrWorkspaceRequired
	}
	return s.store.DeleteIssueWatch(ctx, workspaceID, id)
}

func (s *Service) PollIssueWatch(ctx context.Context, workspaceID, watchID string) ([]Issue, error) {
	watch, err := s.store.GetIssueWatch(ctx, workspaceID, watchID)
	if err != nil {
		return nil, err
	}
	client, err := s.ClientForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	issues, _, err := client.ListIssues(ctx, watch.Owner, watch.Repo, 1, 100)
	now := time.Now().UTC()
	if err != nil {
		_ = s.store.MarkIssueWatchPolled(ctx, watch.ID, now, "Forgejo issue watch poll failed")
		return nil, err
	}
	matching := filterWatchedIssues(issues, watch.Labels)
	if s.issueTaskCreator != nil && watch.Enabled && watch.WorkflowID != "" {
		for _, issue := range matching {
			if watch.InflightLimit > 0 {
				inflight, countErr := s.store.CountIssueWatchInflightTasks(ctx, watch.ID)
				if countErr != nil {
					return nil, countErr
				}
				if inflight >= watch.InflightLimit {
					break
				}
			}
			claimed, claimErr := s.store.ReserveIssueWatchTask(ctx, watch.ID, watch.Owner, watch.Repo, issue.Number)
			if claimErr != nil {
				return nil, claimErr
			}
			if !claimed {
				continue
			}
			taskID, createErr := s.issueTaskCreator(ctx, watch, issue)
			if createErr != nil {
				_ = s.store.ReleaseIssueWatchTask(ctx, watch.ID, watch.Owner, watch.Repo, issue.Number)
				_ = s.store.MarkIssueWatchPolled(ctx, watch.ID, now, "Forgejo issue watch task creation failed")
				return nil, createErr
			}
			if err := s.recordWatchedIssue(ctx, workspaceID, taskID, watch, issue); err != nil {
				_ = s.store.ReleaseIssueWatchTask(ctx, watch.ID, watch.Owner, watch.Repo, issue.Number)
				_ = s.store.MarkIssueWatchPolled(ctx, watch.ID, now, "Forgejo issue watch task link failed")
				return nil, err
			}
			if err := s.store.CompleteIssueWatchTask(ctx, watch.ID, watch.Owner, watch.Repo, issue.Number, taskID); err != nil {
				return nil, err
			}
		}
	}
	if err := s.store.MarkIssueWatchPolled(ctx, watch.ID, now, ""); err != nil {
		return nil, err
	}
	return matching, nil
}

func (s *Service) recordWatchedIssue(ctx context.Context, workspaceID, taskID string, watch *IssueWatch, issue Issue) error {
	config, err := s.store.GetConfig(ctx, workspaceID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	err = s.store.UpsertTaskIssue(ctx, &TaskIssue{TaskID: taskID, RepositoryID: watch.RepositoryID, Origin: config.Origin, Owner: watch.Owner, Repo: watch.Repo, IssueNumber: issue.Number, IssueURL: issue.HTMLURL, Title: issue.Title, State: issue.State, LastSyncedAt: &now})
	if err == nil {
		s.publishTaskLinksUpdated(ctx, workspaceID, taskID)
	}
	return err
}

func (s *Service) RefreshTaskIssue(ctx context.Context, workspaceID, linkID string) (*TaskIssue, error) {
	link, err := s.store.GetTaskIssueLink(ctx, workspaceID, linkID)
	if err != nil {
		return nil, err
	}
	return s.AssociateIssue(ctx, workspaceID, link.TaskID, link.RepositoryID, link.Owner, link.Repo, link.IssueNumber)
}

func (s *Service) RefreshTaskPullRequest(ctx context.Context, workspaceID, linkID string) (*TaskPR, error) {
	link, err := s.store.GetTaskPRLink(ctx, workspaceID, linkID)
	if err != nil {
		return nil, err
	}
	return s.AssociatePullRequest(ctx, workspaceID, link.TaskID, link.RepositoryID, link.Owner, link.Repo, link.PRNumber)
}
func (s *Service) UnlinkTaskPullRequest(ctx context.Context, workspaceID, linkID string) error {
	link, err := s.store.GetTaskPRLink(ctx, workspaceID, linkID)
	if err != nil {
		return err
	}
	if err := s.store.DeleteTaskPR(ctx, workspaceID, linkID); err != nil {
		return err
	}
	s.publishTaskLinksUpdated(ctx, workspaceID, link.TaskID)
	return nil
}

func (s *Service) SetConfig(ctx context.Context, workspaceID string, request *SetConfigRequest) (*Config, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, ErrWorkspaceRequired
	}
	if request == nil {
		return nil, errors.New("forgejo configuration required")
	}
	token := strings.TrimSpace(request.Token)
	if token == "" {
		if s.secrets == nil {
			return nil, errors.New("Forgejo secret store unavailable")
		}
		var err error
		token, err = s.secrets.Reveal(ctx, SecretKeyForWorkspace(workspaceID))
		if err != nil {
			return nil, errors.New("Forgejo token required")
		}
	}
	client, err := NewPATClient(request.Origin, token)
	if err != nil {
		return nil, err
	}
	user, err := client.GetAuthenticatedUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("test Forgejo connection: %w", err)
	}
	config := &Config{WorkspaceID: workspaceID, Origin: client.origin.String(), Username: user.Login, LastOK: true}
	now := time.Now().UTC()
	config.LastCheckedAt = &now
	if s.secrets == nil {
		return nil, errors.New("Forgejo secret store unavailable")
	}
	if strings.TrimSpace(request.Token) != "" {
		if err := s.secrets.Set(ctx, SecretKeyForWorkspace(workspaceID), "Forgejo token", token); err != nil {
			return nil, err
		}
	}
	if secret := strings.TrimSpace(request.WebhookSecret); secret != "" {
		if err := s.secrets.Set(ctx, WebhookSecretKeyForWorkspace(workspaceID), "Forgejo webhook secret", secret); err != nil {
			return nil, err
		}
	}
	if err := s.store.SaveConfig(ctx, config); err != nil {
		return nil, err
	}
	stored, err := s.GetConfig(ctx, workspaceID)
	if err == nil {
		s.publishConfigUpdated(ctx, stored)
	}
	return stored, err
}

func (s *Service) TestConfig(ctx context.Context, request *SetConfigRequest) *TestConnectionResult {
	if request == nil {
		return &TestConnectionResult{Error: "Forgejo configuration required"}
	}
	client, err := NewPATClient(request.Origin, request.Token)
	if err != nil {
		return &TestConnectionResult{Error: "invalid Forgejo origin"}
	}
	user, err := client.GetAuthenticatedUser(ctx)
	if err != nil {
		return &TestConnectionResult{Error: "Forgejo connection test failed"}
	}
	return &TestConnectionResult{OK: true, Username: user.Login}
}

func (s *Service) DeleteConfig(ctx context.Context, workspaceID string) error {
	if strings.TrimSpace(workspaceID) == "" {
		return ErrWorkspaceRequired
	}
	if s.secrets != nil {
		if err := s.secrets.Delete(ctx, SecretKeyForWorkspace(workspaceID)); err != nil {
			return err
		}
		if err := s.secrets.Delete(ctx, WebhookSecretKeyForWorkspace(workspaceID)); err != nil {
			return err
		}
	}
	if err := s.store.DeleteConfig(ctx, workspaceID); err != nil {
		return err
	}
	s.publishConfigUpdated(ctx, &Config{WorkspaceID: workspaceID})
	return nil
}
