# GitHub Integration Plan

## Context

Kandev needs GitHub integration to support two automation workflows:

1. **PR Monitor** - When an agent creates a PR during a session, poll GitHub for CI failures and review comments. The user manually reviews this data in the session UI and can choose to send specific feedback to the agent for fixing.
2. **PR Review Queue** - A background service that periodically polls GitHub for PRs awaiting the user's review, creates tasks in a workflow for each, and optionally auto-starts an agent to review them.

**Additional requirements**:
- PRs must be associated with tasks in the DB (show PR icon on task list)
- Session detail UI: show PR, comments, CI checks, approval status, pending review count, time-pending-review
- PR analytics/stats: PRs created, reviewed, comments, CI results

**Design decisions** (confirmed with user):
- Auth: `gh` CLI primary (existing `gh auth login`), PAT in secrets as fallback
- PR detection: Auto-detect from git push events (no manual workflow config)
- PR monitor: User manually reviews feedback, NOT auto-sent to agent
- Review queue: Standalone background service with a manual "pull now" topbar button
- Scope: Full stack (backend + frontend)

---

## Phase 1: GitHub Client Package

**New package**: `apps/backend/internal/github/`

### 1.1 `models.go` - Data models

```go
// PR represents a GitHub Pull Request
type PR struct {
    Number      int
    Title       string
    URL         string
    HTMLURL     string
    State       string // open, closed, merged
    HeadBranch  string
    BaseBranch  string
    AuthorLogin string
    RepoOwner   string
    RepoName    string
    Draft       bool
    Mergeable   bool
    Additions   int
    Deletions   int
    CreatedAt   time.Time
    UpdatedAt   time.Time
    MergedAt    *time.Time
    ClosedAt    *time.Time
}

// PRReview represents a review on a PR
type PRReview struct {
    ID        int64
    Author    string
    State     string // APPROVED, CHANGES_REQUESTED, COMMENTED, PENDING, DISMISSED
    Body      string
    CreatedAt time.Time
}

// PRComment represents a review comment on specific code
type PRComment struct {
    ID        int64
    Author    string
    Body      string
    Path      string
    Line      int
    Side      string // LEFT, RIGHT
    CreatedAt time.Time
    UpdatedAt time.Time
    InReplyTo *int64
}

// CheckRun represents a CI check result
type CheckRun struct {
    Name       string
    Status     string // queued, in_progress, completed
    Conclusion string // success, failure, neutral, cancelled, timed_out, action_required, skipped
    HTMLURL    string
    Output     string // summary text
    StartedAt  *time.Time
    CompletedAt *time.Time
}

// PRFeedback aggregates all feedback for a PR (fetched live)
type PRFeedback struct {
    PR        *PR
    Reviews   []PRReview
    Comments  []PRComment
    Checks    []CheckRun
    HasIssues bool // true if failing checks or changes_requested
}

// PRWatch tracks active PR monitoring (session → PR)
type PRWatch struct {
    ID              string
    SessionID       string
    TaskID          string
    Owner           string
    Repo            string
    PRNumber        int
    Branch          string
    LastCheckedAt   *time.Time
    LastCommentAt   *time.Time
    LastCheckStatus string
    CreatedAt       time.Time
    UpdatedAt       time.Time
}

// ReviewWatch configures periodic polling for PRs needing user's review
type ReviewWatch struct {
    ID                  string
    WorkspaceID         string
    WorkflowID          string
    WorkflowStepID      string
    RepoOwner           string
    RepoName            string
    AgentProfileID      string
    ExecutorProfileID   string
    Prompt              string
    AutoStart           bool
    Enabled             bool
    PollIntervalSeconds int
    LastPolledAt         *time.Time
    CreatedAt           time.Time
    UpdatedAt           time.Time
}

// GitHubStatus represents GitHub connection status
type GitHubStatus struct {
    Authenticated bool
    Username      string
    AuthMethod    string // "gh_cli", "pat", "none"
}
```

### 1.2 `client.go` - Interface

```go
type Client interface {
    // Auth
    IsAuthenticated(ctx context.Context) (bool, error)
    GetAuthenticatedUser(ctx context.Context) (string, error)

    // PR queries
    GetPR(ctx context.Context, owner, repo string, number int) (*PR, error)
    FindPRByBranch(ctx context.Context, owner, repo, branch string) (*PR, error)
    ListAuthoredPRs(ctx context.Context, owner, repo string) ([]*PR, error)
    ListReviewRequestedPRs(ctx context.Context, username string) ([]*PR, error)

    // PR details
    ListPRReviews(ctx context.Context, owner, repo string, number int) ([]PRReview, error)
    ListPRComments(ctx context.Context, owner, repo string, number int, since *time.Time) ([]PRComment, error)
    ListCheckRuns(ctx context.Context, owner, repo, ref string) ([]CheckRun, error)

    // Aggregated
    GetPRFeedback(ctx context.Context, owner, repo string, number int) (*PRFeedback, error)
}
```

### 1.3 `gh_client.go` - Primary (gh CLI)

Executes `gh` commands:
- `gh pr list --json number,title,url,headRefName,baseRefName,author,state,createdAt,updatedAt,additions,deletions,isDraft,mergeable`
- `gh pr view <number> --json reviews,comments`
- `gh api repos/{owner}/{repo}/commits/{ref}/check-runs`
- `gh api search/issues?q=type:pr+review-requested:@me+state:open`

Detect availability via `gh --version`. Parse JSON stdout.

### 1.4 `pat_client.go` - Fallback (PAT via net/http)

Uses `GITHUB_TOKEN` secret from secrets store. `Authorization: token <PAT>`. Rate limit header awareness.

### 1.5 `factory.go` - Client factory

Try `gh` CLI first, fall back to PAT from secrets store.

### 1.6 `provider.go` - DI provider

Standard `Provide(cfg, log, secretsSvc, db) (*Service, cleanup, error)` pattern.

---

## Phase 2: Service + Persistence

### 2.1 `service.go` - Business logic

```go
type Service struct {
    client   Client
    store    *Store
    eventBus bus.EventBus
    logger   *logger.Logger
}

// PR Watch operations (for PR monitoring)
func (s *Service) CreatePRWatch(ctx, sessionID, taskID, owner, repo string, prNumber int, branch string) (*PRWatch, error)
func (s *Service) DeletePRWatch(ctx, id string) error
func (s *Service) GetPRWatchBySession(ctx, sessionID string) (*PRWatch, error)
func (s *Service) ListActivePRWatches(ctx) ([]*PRWatch, error)

// Task-PR association
func (s *Service) AssociatePRWithTask(ctx, taskID, owner, repo string, prNumber int, prURL, prTitle, authorLogin string) (*TaskPR, error)
func (s *Service) GetTaskPR(ctx, taskID string) (*TaskPR, error)
func (s *Service) ListTaskPRs(ctx, taskIDs []string) (map[string]*TaskPR, error)
func (s *Service) UpdateTaskPR(ctx, id string, state, mergeState string, ...) error

// PR feedback (fetched live, not stored)
func (s *Service) GetPRFeedback(ctx, owner, repo string, number int) (*PRFeedback, error)

// Review Watch operations (for review queue)
func (s *Service) CreateReviewWatch(ctx, req *CreateReviewWatchRequest) (*ReviewWatch, error)
func (s *Service) UpdateReviewWatch(ctx, id string, req *UpdateReviewWatchRequest) error
func (s *Service) DeleteReviewWatch(ctx, id string) error
func (s *Service) ListReviewWatches(ctx, workspaceID string) ([]*ReviewWatch, error)

// Polling
func (s *Service) CheckPRWatch(ctx, watch *PRWatch) (*PRFeedback, bool, error) // returns hasNewFeedback
func (s *Service) CheckReviewWatch(ctx, watch *ReviewWatch) ([]*PR, error) // returns new PRs

// Manual fetch
func (s *Service) TriggerReviewCheck(ctx, watchID string) ([]*PR, error)
func (s *Service) TriggerAllReviewChecks(ctx, workspaceID string) (int, error) // returns count of new PRs found

// Status
func (s *Service) GetStatus(ctx) (*GitHubStatus, error)
```

### 2.2 `store.go` - SQLite persistence

Four tables (owned by github package):

```sql
-- Tracks active PR monitoring (session → PR)
CREATE TABLE IF NOT EXISTS github_pr_watches (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL UNIQUE,
    task_id TEXT NOT NULL,
    owner TEXT NOT NULL,
    repo TEXT NOT NULL,
    pr_number INTEGER NOT NULL,
    branch TEXT NOT NULL,
    last_checked_at DATETIME,
    last_comment_at DATETIME,
    last_check_status TEXT DEFAULT '',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

-- Associates a PR with a task (1:1 for now, could be 1:many later)
-- Used for: PR icon on task list, session detail PR view, stats
CREATE TABLE IF NOT EXISTS github_task_prs (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    owner TEXT NOT NULL,
    repo TEXT NOT NULL,
    pr_number INTEGER NOT NULL,
    pr_url TEXT NOT NULL,
    pr_title TEXT NOT NULL,
    head_branch TEXT NOT NULL,
    base_branch TEXT NOT NULL,
    author_login TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'open',        -- open, closed, merged
    review_state TEXT NOT NULL DEFAULT '',      -- approved, changes_requested, pending, ''
    checks_state TEXT NOT NULL DEFAULT '',      -- success, failure, pending, ''
    review_count INTEGER DEFAULT 0,
    pending_review_count INTEGER DEFAULT 0,
    comment_count INTEGER DEFAULT 0,
    additions INTEGER DEFAULT 0,
    deletions INTEGER DEFAULT 0,
    created_at DATETIME NOT NULL,              -- PR creation time on GitHub
    merged_at DATETIME,
    closed_at DATETIME,
    last_synced_at DATETIME,
    updated_at DATETIME NOT NULL,
    UNIQUE(task_id, pr_number)
);

-- Review watch configurations (workspace → repo polling)
CREATE TABLE IF NOT EXISTS github_review_watches (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    workflow_id TEXT NOT NULL,
    workflow_step_id TEXT NOT NULL,
    repo_owner TEXT NOT NULL,
    repo_name TEXT NOT NULL,
    agent_profile_id TEXT NOT NULL,
    executor_profile_id TEXT NOT NULL,
    prompt TEXT DEFAULT '',
    auto_start BOOLEAN DEFAULT 0,
    enabled BOOLEAN DEFAULT 1,
    poll_interval_seconds INTEGER DEFAULT 300,
    last_polled_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

-- Deduplication: which PRs we already created tasks for
CREATE TABLE IF NOT EXISTS github_review_pr_tasks (
    id TEXT PRIMARY KEY,
    review_watch_id TEXT NOT NULL,
    pr_number INTEGER NOT NULL,
    pr_url TEXT NOT NULL,
    task_id TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    UNIQUE(review_watch_id, pr_number)
);
```

---

## Phase 3: Background Poller

### 3.1 `poller.go`

Two goroutine loops:

**PR Monitor Loop** (every 2 min):
1. List all active `github_pr_watches`
2. For each watch, fetch PR feedback (comments + checks) via client
3. Compare with last known state (last_comment_at, last_check_status)
4. If new comments or check state changes:
   - Update the `github_task_prs` record (review_state, checks_state, comment_count, etc.)
   - Publish `github.pr_feedback` event to event bus (for UI notification, NOT auto-prompt)
   - Update watch timestamps
5. Also sync PR state (merged, closed) to `github_task_prs`

**Review Queue Loop** (every 5 min, per-watch configurable):
1. List all enabled `github_review_watches`
2. For each watch, call `ListReviewRequestedPRs` filtered to the watch's repo
3. For each PR not already in `github_review_pr_tasks`:
   - Publish `github.new_pr_to_review` event
   - Orchestrator handler creates the task + associates the PR

Methods: `Start(ctx)`, `Stop()`, `TriggerReviewCheck(ctx, watchID)`, `TriggerAllReviewChecks(ctx, workspaceID)`.

---

## Phase 4: Event Bus + Orchestrator Integration

### 4.1 New event types

**File**: `apps/backend/internal/events/types.go`

```go
// Event types for GitHub integration
const (
    GitHubPRFeedback       = "github.pr_feedback"       // PR has new feedback (UI notification only)
    GitHubPRStateChanged   = "github.pr_state_changed"   // PR state changed (merged, closed, etc.)
    GitHubNewReviewPR      = "github.new_pr_to_review"   // New PR found needing review
    GitHubWatchEvent       = "github.watch.event"         // Watch created/deleted
    GitHubTaskPRUpdated    = "github.task_pr.updated"     // TaskPR record updated (for UI refresh)
)
```

### 4.2 PR auto-detection from git events

**File**: `apps/backend/internal/orchestrator/event_handlers_git.go`

In `handleGitStatusUpdate()`, detect push completion:
- Track per-session state: when `Ahead` goes from `>0` to `0` with a `RemoteBranch` set, a push happened
- After detecting push, call `githubService.Client().FindPRByBranch()` to check if a PR exists
- If PR found and no watch exists for this session:
  1. Auto-create `PRWatch` via `githubService.CreatePRWatch()`
  2. Auto-create `TaskPR` association via `githubService.AssociatePRWithTask()`
  3. Publish `github.task_pr.updated` event (so task list shows PR icon)

**Lookup chain**:
- `repoStore.GetTaskSession(sessionID)` → `RepositoryID`
- `repoStore.GetRepository(repositoryID)` → `ProviderOwner`, `ProviderName`
- `FindPRByBranch(providerOwner, providerName, branch)`

### 4.3 Orchestrator GitHub event handlers

**New file**: `apps/backend/internal/orchestrator/event_handlers_github.go`

```go
// handlePRFeedback forwards PR feedback events to the frontend via WS.
// The user manually decides whether to send feedback to the agent.
// This does NOT auto-prompt the agent.
func (s *Service) handlePRFeedback(ctx context.Context, event *bus.Event) {
    // Extract sessionID, feedback summary from event
    // Forward to WS for session UI notification
    // Frontend shows updated PR data in session detail panel
}

// handleNewReviewPR creates a task for a PR needing review.
func (s *Service) handleNewReviewPR(ctx context.Context, event *bus.Event) {
    // Extract PR data, ReviewWatch config
    // Create task in the watch's workflow at configured step
    // Associate PR with task via githubService.AssociatePRWithTask()
    // Record in github_review_pr_tasks for dedup
    // If auto_start, enqueue the task for agent execution
}
```

### 4.4 "Send to Agent" action

The session detail UI will have a "Send to Agent" button that lets the user compose a message to the agent including selected PR comments and/or CI failures. This uses the existing `PromptTask()` flow — the frontend sends a user message with the selected feedback context embedded.

### 4.5 Orchestrator wiring

**File**: `apps/backend/internal/orchestrator/service.go`
- Add `githubService` field + `SetGitHubService()` setter
- Add `pushTracker sync.Map` for per-session ahead-count tracking
- Subscribe to `github.pr_feedback` and `github.new_pr_to_review` events in `Start()`

**File**: `apps/backend/cmd/kandev/services.go`
- Add GitHub service to `Services` struct, initialize via `github.Provide()`
- Wire into orchestrator, start poller

**File**: `apps/backend/cmd/kandev/main.go`
- Register GitHub HTTP routes

---

## Phase 5: HTTP API

### 5.1 `controller.go`

**Route group**: `/api/v1/github/`

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/status` | GET | Auth status (username, auth method) |
| `/prs/authored` | GET | User's open PRs (query: owner, repo) |
| `/prs/review-requested` | GET | PRs awaiting user's review |
| `/prs/{owner}/{repo}/{number}` | GET | PR details with full feedback |
| `/prs/{owner}/{repo}/{number}/checks` | GET | CI check status only |
| `/prs/{owner}/{repo}/{number}/comments` | GET | PR comments only |
| `/task-prs` | GET | List task-PR associations (query: task_ids) |
| `/task-prs/{taskId}` | GET | Get PR association for a task |
| `/watches/pr` | GET | List active PR watches |
| `/watches/pr/{id}` | DELETE | Remove a PR watch |
| `/watches/review` | GET | List review watches (query: workspace_id) |
| `/watches/review` | POST | Create review watch |
| `/watches/review/{id}` | PUT | Update review watch |
| `/watches/review/{id}` | DELETE | Delete review watch |
| `/watches/review/{id}/trigger` | POST | Manual "pull now" for one watch |
| `/watches/review/trigger-all` | POST | Trigger all for workspace |
| `/stats` | GET | PR stats (query: workspace_id, date range) |

### 5.2 `handlers.go` - WS handlers

| Action | Description |
|--------|-------------|
| `github.status` | Auth status |
| `github.task_prs.list` | Get task-PR associations (batch) |
| `github.pr_feedback.get` | Get live PR feedback for session |
| `github.review_watches.list` | List review watches |
| `github.review_watches.create` | Create review watch |
| `github.review_watches.update` | Update review watch |
| `github.review_watches.delete` | Delete review watch |
| `github.review_watches.trigger` | Trigger single check |
| `github.review_watches.trigger_all` | Trigger all for workspace |

---

## Phase 6: Frontend

### 6.1 Types: `apps/web/lib/types/github.ts`

```typescript
interface GitHubStatus {
  authenticated: boolean;
  username: string;
  authMethod: 'gh_cli' | 'pat' | 'none';
}

interface GitHubPR {
  number: number; title: string; url: string; htmlUrl: string;
  state: 'open' | 'closed' | 'merged';
  headBranch: string; baseBranch: string;
  authorLogin: string; repoOwner: string; repoName: string;
  draft: boolean; additions: number; deletions: number;
  createdAt: string; updatedAt: string;
  mergedAt: string | null; closedAt: string | null;
}

interface TaskPR {
  id: string; taskId: string;
  owner: string; repo: string; prNumber: number;
  prUrl: string; prTitle: string;
  headBranch: string; baseBranch: string; authorLogin: string;
  state: 'open' | 'closed' | 'merged';
  reviewState: 'approved' | 'changes_requested' | 'pending' | '';
  checksState: 'success' | 'failure' | 'pending' | '';
  reviewCount: number; pendingReviewCount: number; commentCount: number;
  additions: number; deletions: number;
  createdAt: string; mergedAt: string | null; closedAt: string | null;
  lastSyncedAt: string | null;
}

interface PRReview { id: number; author: string; state: string; body: string; createdAt: string; }
interface PRComment { id: number; author: string; body: string; path: string; line: number; side: string; createdAt: string; inReplyTo: number | null; }
interface CheckRun { name: string; status: string; conclusion: string; htmlUrl: string; output: string; }
interface PRFeedback { pr: GitHubPR; reviews: PRReview[]; comments: PRComment[]; checks: CheckRun[]; hasIssues: boolean; }

interface PRWatch { id: string; sessionId: string; taskId: string; owner: string; repo: string; prNumber: number; branch: string; lastCheckedAt: string | null; }
interface ReviewWatch { id: string; workspaceId: string; workflowId: string; workflowStepId: string; repoOwner: string; repoName: string; agentProfileId: string; executorProfileId: string; prompt: string; autoStart: boolean; enabled: boolean; pollIntervalSeconds: number; lastPolledAt: string | null; }
```

### 6.2 API Client: `apps/web/lib/api/domains/github-api.ts`

Methods mirroring all HTTP endpoints.

### 6.3 Store: `apps/web/lib/state/slices/github/github-slice.ts`

Zustand slice:
- `github.status` - auth status
- `github.taskPRs.byTaskId` - PR associations per task (for task list PR icon)
- `github.prWatches.bySessionId` - active PR watches per session
- `github.reviewWatches.byWorkspaceId` - review watch configs

### 6.4 Hooks: `apps/web/hooks/domains/github/`

- `use-github-status.ts` - auth status
- `use-task-pr.ts` - PR association for a task (or batch for task list)
- `use-pr-feedback.ts` - live PR feedback for session detail
- `use-review-watches.ts` - CRUD for review watches
- `use-pr-watch.ts` - PR monitoring status per session

### 6.5 Task List: PR Icon

**File**: Modify task card/row components to show a PR icon (e.g., git-pull-request icon from lucide) when `taskPR` exists for that task. Color-coded: green (approved/passing), red (changes requested/failing), yellow (pending).

### 6.6 Settings Page: `apps/web/app/settings/general/github/page.tsx`

- **Connection Status card**: Auth method, username, `gh` CLI availability
- **Review Watches table**: List watches with repo, workflow, status, last polled
- **Create/Edit Review Watch dialog**: Repo selector (from workspace repos with `provider: "github"`), workflow+step selector, agent profile, executor profile, custom prompt, auto-start toggle, poll interval, enable/disable

### 6.7 Session Detail: PR Info in Topbar + PR Panel

**Session topbar** (`apps/web/components/github/pr-session-topbar.tsx`):
In the session detail topbar (not the kanban topbar), show a compact PR summary when a PR is associated:
- PR number + link (e.g., `#42`)
- State badge (open/merged/closed)
- CI status indicator (green check / red X / yellow spinner)
- Review status (e.g., "2/3 approved", "changes requested")
- Comment count badge
- Clicking opens the full PR detail panel

**Full PR detail panel** (`apps/web/components/github/pr-detail-panel.tsx`):
Expandable/drawer panel accessible from the session topbar:
- **PR header**: Title, number, link, state badge, draft indicator
- **Stats bar**: +additions/-deletions, time since created, time pending review
- **Reviews section**: List of reviews with state (approved/changes_requested), author, body
- **Pending reviewers**: Who hasn't reviewed yet, how long pending
- **CI Checks section**: List of check runs with status badge (pass/fail/pending), link to details
- **Comments section**: Threaded review comments with file path, line number, author
- **"Send to Agent" button**: Opens a dialog to compose a message to the agent, with checkboxes to include specific comments/CI failures as context. Sends via normal `PromptTask` flow.

### 6.8 Kanban Homepage Topbar: Manual Pull Button

**Component**: `apps/web/components/github/refresh-reviews-button.tsx`

Button in the **kanban/homepage topbar** (NOT the session detail topbar):
- Triggers `POST /watches/review/trigger-all` for active workspace
- Shows toast with results ("Found 3 new PRs to review")
- Only visible when review watches exist for the workspace

### 6.9 Stats

**Route**: `/api/v1/github/stats` (backend) + stats section in settings or dedicated page

**Backend** (`internal/github/stats.go`):

```go
type PRStats struct {
    // Counts
    TotalPRsCreated      int     // PRs authored by user (from github_task_prs)
    TotalPRsReviewed     int     // PRs reviewed (tasks from review watches)
    TotalComments        int     // Total review comments across all PRs
    TotalChecksRun       int     // Total CI check runs tracked

    // Rates
    CIPassRate           float64 // % of PRs where all checks passed
    ApprovalRate         float64 // % of PRs approved on first review cycle

    // Timing (averages)
    AvgTimeToFirstReview time.Duration // Time from PR creation to first review
    AvgTimeToMerge       time.Duration // Time from PR creation to merge
    AvgTimePendingReview time.Duration // Time PRs spent waiting for review

    // By period (for charts)
    PRsByDay             []DailyCount // PRs created per day
    ReviewsByDay         []DailyCount // PRs reviewed per day
    CommentsByDay        []DailyCount // Comments per day
}

type DailyCount struct {
    Date  string `json:"date"` // "2026-02-22"
    Count int    `json:"count"`
}
```

Data sourced from `github_task_prs` table aggregations with date range filtering.

**Frontend** (`apps/web/components/github/pr-stats.tsx`):
- Stats cards: PRs created, reviewed, avg time to merge, CI pass rate
- Line charts: PRs over time, comments over time
- Uses `@kandev/ui` Card, Badge for layout
- Could be embedded in GitHub settings page or as a section in a workspace dashboard

---

## Files to Create

```
apps/backend/internal/github/
  models.go           # Data models (PR, Review, Comment, Check, Watch, TaskPR)
  client.go           # Client interface
  gh_client.go        # gh CLI implementation
  pat_client.go       # PAT HTTP implementation
  factory.go          # Client factory (gh CLI → PAT fallback)
  store.go            # SQLite persistence (watches, task_prs, dedup)
  service.go          # Business logic
  poller.go           # Background polling loops
  controller.go       # HTTP endpoints
  handlers.go         # WS handlers
  provider.go         # DI provider
  stats.go            # PR analytics queries

apps/web/
  lib/types/github.ts
  lib/api/domains/github-api.ts
  lib/state/slices/github/
    index.ts
    github-slice.ts
  hooks/domains/github/
    use-github-status.ts
    use-task-pr.ts
    use-pr-feedback.ts
    use-review-watches.ts
    use-pr-watch.ts
  app/settings/general/github/
    page.tsx
  components/github/
    github-status.tsx
    review-watch-table.tsx
    review-watch-dialog.tsx
    pr-detail-panel.tsx        # Session detail PR view (expandable panel)
    pr-session-topbar.tsx      # Compact PR info in session topbar
    pr-task-icon.tsx            # PR icon for task list
    refresh-reviews-button.tsx  # Topbar manual pull button
    send-to-agent-dialog.tsx    # Compose feedback to send to agent
    pr-stats.tsx                # PR analytics cards + charts
```

## Files to Modify

```
apps/backend/internal/events/types.go                      # Add GitHub event constants
apps/backend/internal/orchestrator/service.go               # Add githubService field + setter + push tracker
apps/backend/internal/orchestrator/event_handlers_git.go    # PR auto-detection on push
apps/backend/cmd/kandev/services.go                         # Wire GitHub service
apps/backend/cmd/kandev/types.go                            # Add GitHub to Services struct
apps/backend/cmd/kandev/main.go                             # Register GitHub routes

apps/web/lib/state/store.ts                                 # Add github slice
apps/web/lib/state/hydration/merge-strategies.ts            # Add github hydration
apps/web/app/settings/general/ (layout or nav)              # Add GitHub settings link
apps/web/components/ (task card/row)                         # Add PR icon to task list
apps/web/components/ (session detail panel)                  # Add PR detail panel
```

## New file: `apps/backend/internal/orchestrator/event_handlers_github.go`

Orchestrator event handlers for GitHub events (handlePRFeedback, handleNewReviewPR).

## Key Patterns to Reuse

- **Provider pattern**: `Provide(cfg, log, ...) (*Service, cleanup, error)` — see `apps/backend/internal/secrets/provider.go`
- **Event bus**: `bus.EventBus` publish/subscribe — see `apps/backend/internal/events/types.go`
- **Setter injection**: `SetGitHubService()` — see `orchestrator/service.go` `SetWorkflowStepGetter()`
- **Repository sub-interfaces**: Split interfaces in `apps/backend/internal/task/repository/interface.go`
- **Task service Repos struct**: `taskservice.Repos{}` — see `apps/backend/cmd/kandev/services.go:38`
- **Git event handling**: `handleGitStatusUpdate()` in `apps/backend/internal/orchestrator/event_handlers_git.go`
- **Repository model provider fields**: `Provider`, `ProviderOwner`, `ProviderName` in `apps/backend/internal/task/models/models.go:285`
- **Secrets store for PAT**: `apps/backend/internal/secrets/service.go` (Reveal method)
- **Credential env provider**: `GITHUB_TOKEN` already listed in `apps/backend/internal/agent/credentials/env_provider.go:25`
- **WS handler pattern**: See existing handlers in `apps/backend/internal/task/handlers/`
- **Frontend store slices**: Domain pattern in `apps/web/lib/state/slices/`
- **Frontend hooks**: Domain pattern in `apps/web/hooks/domains/`
- **Settings page layout**: Existing pages at `apps/web/app/settings/general/{editors,secrets,notifications}/`
- **Shadcn components**: Import from `@kandev/ui` — Table, Dialog, Button, Badge, etc.

## Implementation Order

### Milestone 1: GitHub Client (Backend)
1. `models.go` - data models
2. `client.go` - interface
3. `gh_client.go` - gh CLI implementation
4. `pat_client.go` - PAT fallback
5. `factory.go` - client factory
6. Unit tests for client JSON parsing

### Milestone 2: Persistence + Service (Backend)
7. `store.go` - SQLite tables + CRUD
8. `service.go` - business logic
9. `provider.go` - DI provider
10. Wire into `cmd/kandev/services.go`

### Milestone 3: API + Handlers (Backend)
11. `controller.go` - HTTP endpoints
12. `handlers.go` - WS handlers
13. Add event types to `events/types.go`
14. Wire routes in `cmd/kandev/main.go`

### Milestone 4: Poller + Orchestrator (Backend)
15. `poller.go` - background polling
16. Push detection in `event_handlers_git.go`
17. `event_handlers_github.go` - orchestrator GitHub event handlers
18. Wire GitHub service into orchestrator
19. Integration tests

### Milestone 5: Frontend Core
20. TypeScript types
21. API client
22. Store slice
23. Domain hooks

### Milestone 6: Frontend UI
24. Settings page (GitHub config + review watches)
25. Task list PR icon
26. Session detail PR panel (PR, comments, CI, approvals, "Send to Agent")
27. Topbar refresh button

### Milestone 7: Stats
28. `stats.go` - PR analytics queries (aggregations on `github_task_prs`)
29. Stats HTTP endpoint (`/api/v1/github/stats`)
30. Frontend stats component (`pr-stats.tsx`) with cards + charts
31. Integrate stats into GitHub settings page or dashboard

## Verification

1. **Backend unit tests**: Client JSON parsing, store CRUD, poller logic
2. **Integration**: `make -C apps/backend test`
3. **Lint/typecheck**: `make -C apps/backend lint` + `cd apps && pnpm --filter @kandev/web typecheck`
4. **Manual**: Check `/api/v1/github/status`, create review watch, trigger manual pull
5. **E2E**: Agent pushes → PR auto-detected → PR watch created → task shows PR icon → session detail shows PR panel → user reviews and sends feedback to agent
