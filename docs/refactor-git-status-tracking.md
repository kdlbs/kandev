# Git Status & Diff Tracking Refactor

**Date:** 2026-01-23  
**Status:** Design Proposal  
**Goal:** Fix stale diff data and enable persistent git status tracking with full commit history visibility

---

## Current Problems

### 1. **Stale Diff Content**
- Diff content is generated once by `WorkspaceTracker.enrichWithDiffData()` and stored in `FileInfo.Diff`
- When git operations (stage, commit) modify `.git/index`, the filesystem watcher doesn't detect these changes
- The diff remains stale even though `file.staged` boolean updates correctly
- This causes:
  - Invalid hunk headers that crash `<DiffView>` component
  - Mismatch between staged status and actual diff content
  - Confusion when viewing changes after staging

### 2. **DiffView Component Crash**
- `<DiffView>` expects `hunks` as an array of diff strings (one per hunk)
- Current code passes `hunks: [file.diff]` where `file.diff` is already a complete unified diff
- The library tries to parse the entire diff as a single hunk header, causing "Invalid hunk header format" errors

### 3. **Transient Git Status Storage**
- Git status is stored in:
  - **Runtime:** `gitStatus.bySessionId[sessionId]` in Zustand store (frontend)
  - **Persistence:** `task_sessions.metadata.git_status` (backend database)
- This approach loses historical context:
  - Can't see what files were changed before a commit
  - Can't track progression of work across multiple commits
  - Can't show cumulative diff from base branch after intermediate commits

### 4. **Missing Commit Tracking**
- No record of commits made during a session
- Can't show "total changes from base branch" vs "uncommitted changes"
- Can't reconstruct session history after commits

---

## Proposed Solution

### Architecture Changes

#### 1. **New Database Table: `task_session_git_snapshots`**

Store git status snapshots at key moments (on every git status update):

```sql
CREATE TABLE IF NOT EXISTS task_session_git_snapshots (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    snapshot_type TEXT NOT NULL,  -- 'status_update', 'pre_commit', 'post_commit', 'pre_stage', 'post_stage'
    
    -- Git state
    branch TEXT NOT NULL,
    remote_branch TEXT DEFAULT '',
    head_commit TEXT DEFAULT '',  -- Current HEAD SHA
    base_commit TEXT DEFAULT '',  -- Base branch HEAD SHA (for comparison)
    ahead INTEGER DEFAULT 0,
    behind INTEGER DEFAULT 0,
    
    -- File changes (JSON)
    files TEXT DEFAULT '{}',  -- JSON: map[string]FileInfo with diff content
    
    -- Metadata
    triggered_by TEXT DEFAULT '',  -- 'filesystem_watcher', 'git_stage', 'git_commit', etc.
    metadata TEXT DEFAULT '{}',
    created_at DATETIME NOT NULL,
    
    FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_git_snapshots_session ON task_session_git_snapshots(session_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_git_snapshots_type ON task_session_git_snapshots(session_id, snapshot_type);
```

**Key Design Decisions:**
- Store complete `files` JSON with diff content for each snapshot
- Use `snapshot_type` to distinguish different trigger points
- Keep `head_commit` and `base_commit` to enable diff reconstruction
- Index by `(session_id, created_at DESC)` for efficient timeline queries

#### 2. **New Database Table: `task_session_commits`**

Track commits made during the session:

```sql
CREATE TABLE IF NOT EXISTS task_session_commits (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    commit_sha TEXT NOT NULL,
    parent_sha TEXT DEFAULT '',
    author_name TEXT DEFAULT '',
    author_email TEXT DEFAULT '',
    commit_message TEXT DEFAULT '',
    committed_at DATETIME NOT NULL,

    -- Snapshot reference (optional)
    pre_commit_snapshot_id TEXT DEFAULT '',   -- Snapshot before commit
    post_commit_snapshot_id TEXT DEFAULT '',  -- Snapshot after commit

    -- File stats
    files_changed INTEGER DEFAULT 0,
    insertions INTEGER DEFAULT 0,
    deletions INTEGER DEFAULT 0,

    created_at DATETIME NOT NULL,

    FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_session_commits_session ON task_session_commits(session_id, committed_at DESC);
CREATE INDEX IF NOT EXISTS idx_session_commits_sha ON task_session_commits(commit_sha);
```

**Purpose:**
- Track all commits made during a session
- Link to snapshots for before/after comparison
- Enable "session timeline" view showing commits + file changes
- Support cumulative diff calculation across commits

#### 3. **Remove Git Status from `task_sessions.metadata`**

**Current State:**
- `task_sessions.metadata.git_status` stores latest git status
- Duplicates data that should be in snapshots table
- Makes queries inefficient (JSON parsing)

**Migration:**
- Move git status out of `metadata` field
- Use `task_session_git_snapshots` as source of truth
- Keep `metadata` for truly ephemeral data (context_window, etc.)

---

## Backend Implementation Plan

### Phase 1: Database Layer

#### 1.1 Create Migration

**File:** `apps/backend/internal/task/repository/sqlite/migrations/XXX_add_git_snapshots.sql`

```sql
-- Add git snapshots table
CREATE TABLE IF NOT EXISTS task_session_git_snapshots (...);
CREATE INDEX ...;

-- Add commits table
CREATE TABLE IF NOT EXISTS task_session_commits (...);
CREATE INDEX ...;

-- Migrate existing git_status from metadata to snapshots
INSERT INTO task_session_git_snapshots (id, session_id, snapshot_type, branch, ...)
SELECT
    lower(hex(randomblob(16))),
    id,
    'status_update',
    json_extract(metadata, '$.git_status.branch'),
    ...
FROM task_sessions
WHERE json_extract(metadata, '$.git_status') IS NOT NULL;
```

#### 1.2 Add Repository Methods

**File:** `apps/backend/internal/task/repository/sqlite/git_snapshots.go` (new file)

```go
// CreateGitSnapshot stores a new git status snapshot
func (r *Repository) CreateGitSnapshot(ctx context.Context, snapshot *models.GitSnapshot) error

// GetLatestGitSnapshot retrieves the most recent snapshot for a session
func (r *Repository) GetLatestGitSnapshot(ctx context.Context, sessionID string) (*models.GitSnapshot, error)

// GetGitSnapshotsBySession retrieves all snapshots for a session (ordered by created_at DESC)
func (r *Repository) GetGitSnapshotsBySession(ctx context.Context, sessionID string) ([]*models.GitSnapshot, error)

// GetGitSnapshotByType retrieves the latest snapshot of a specific type
func (r *Repository) GetGitSnapshotByType(ctx context.Context, sessionID string, snapshotType string) (*models.GitSnapshot, error)

// CreateCommitRecord stores a commit made during the session
func (r *Repository) CreateCommitRecord(ctx context.Context, commit *models.SessionCommit) error

// GetCommitsBySession retrieves all commits for a session
func (r *Repository) GetCommitsBySession(ctx context.Context, sessionID string) ([]*models.SessionCommit, error)

// GetCumulativeDiff computes diff from base_commit to current HEAD
func (r *Repository) GetCumulativeDiff(ctx context.Context, sessionID string) (*models.CumulativeDiff, error)
```

#### 1.3 Add Models

**File:** `apps/backend/internal/task/models/git.go` (new file)

```go
package models

import "time"

type SnapshotType string

const (
    SnapshotTypeStatusUpdate SnapshotType = "status_update"
    SnapshotTypePreCommit    SnapshotType = "pre_commit"
    SnapshotTypePostCommit   SnapshotType = "post_commit"
    SnapshotTypePreStage     SnapshotType = "pre_stage"
    SnapshotTypePostStage    SnapshotType = "post_stage"
)

type GitSnapshot struct {
    ID            string                 `json:"id"`
    SessionID     string                 `json:"session_id"`
    SnapshotType  SnapshotType           `json:"snapshot_type"`
    Branch        string                 `json:"branch"`
    RemoteBranch  string                 `json:"remote_branch"`
    HeadCommit    string                 `json:"head_commit"`
    BaseCommit    string                 `json:"base_commit"`
    Ahead         int                    `json:"ahead"`
    Behind        int                    `json:"behind"`
    Files         map[string]interface{} `json:"files"` // FileInfo objects
    TriggeredBy   string                 `json:"triggered_by"`
    Metadata      map[string]interface{} `json:"metadata,omitempty"`
    CreatedAt     time.Time              `json:"created_at"`
}

type SessionCommit struct {
    ID                    string    `json:"id"`
    SessionID             string    `json:"session_id"`
    CommitSHA             string    `json:"commit_sha"`
    ParentSHA             string    `json:"parent_sha"`
    AuthorName            string    `json:"author_name"`
    AuthorEmail           string    `json:"author_email"`
    CommitMessage         string    `json:"commit_message"`
    CommittedAt           time.Time `json:"committed_at"`
    PreCommitSnapshotID   string    `json:"pre_commit_snapshot_id"`
    PostCommitSnapshotID  string    `json:"post_commit_snapshot_id"`
    FilesChanged          int       `json:"files_changed"`
    Insertions            int       `json:"insertions"`
    Deletions             int       `json:"deletions"`
    CreatedAt             time.Time `json:"created_at"`
}

type CumulativeDiff struct {
    SessionID    string                 `json:"session_id"`
    BaseCommit   string                 `json:"base_commit"`
    HeadCommit   string                 `json:"head_commit"`
    TotalCommits int                    `json:"total_commits"`
    Files        map[string]interface{} `json:"files"` // Cumulative file changes
}
```

### Phase 2: Workspace Tracker Changes

#### 2.1 Modify `WorkspaceTracker` to Publish Git Status

**File:** `apps/backend/internal/agentctl/server/process/workspace_tracker.go`

**Important:** `WorkspaceTracker` runs in agentctl (container/standalone) and has **NO database access**.

**Changes:**
1. Keep existing logic: detect git status changes
2. Publish git status via WebSocket to backend
3. **Do NOT** create snapshots here (no DB access)

**Key Methods:**

```go
// updateGitStatus - NO CHANGES to snapshot creation
// This already publishes to WebSocket, which is correct
func (wt *WorkspaceTracker) updateGitStatus(ctx context.Context) {
    status, err := wt.getGitStatus(ctx)
    if err != nil {
        return
    }

    wt.mu.Lock()
    wt.currentStatus = status
    wt.mu.Unlock()

    // This publishes to WebSocket → backend receives it
    wt.notifyWorkspaceStreamGitStatus(status)
}
```

**No changes needed here** - abstraction is already correct.

#### 2.2 Git Operations - No Changes Needed

**File:** `apps/backend/internal/agentctl/server/process/git.go`

**Important:** `GitOperator` also runs in agentctl and has **NO database access**.

**Current behavior is correct:**
1. Git operations (stage, commit) modify the working tree
2. `WorkspaceTracker` detects changes via fsnotify
3. Git status published to backend via WebSocket
4. Backend receives event and creates snapshot

**No changes needed here** - the existing flow respects abstraction layers.

**Note:** For commit operations, we need to publish commit metadata separately (see Phase 3).

### Phase 3: Event Bus & Orchestrator Changes

**This is where snapshot creation happens** - orchestrator has access to repository layer.

#### 3.1 Update Event Handlers

**File:** `apps/backend/internal/orchestrator/event_handlers.go`

**Modify `handleGitStatusUpdated`:**

```go
func (s *Service) handleGitStatusUpdated(ctx context.Context, data watcher.GitStatusData) {
    s.logger.Debug("handling git status update",
        zap.String("task_id", data.TaskID),
        zap.String("branch", data.Branch))

    if data.TaskSessionID == "" {
        return
    }

    // Create git snapshot instead of storing in session metadata
    snapshot := &models.GitSnapshot{
        ID:           uuid.New().String(),
        SessionID:    data.TaskSessionID,
        SnapshotType: models.SnapshotTypeStatusUpdate,
        Branch:       data.Branch,
        RemoteBranch: data.RemoteBranch,
        HeadCommit:   data.HeadCommit,
        BaseCommit:   data.BaseCommit,
        Ahead:        data.Ahead,
        Behind:       data.Behind,
        Files:        data.Files,
        TriggeredBy:  "git_status_event",
        CreatedAt:    time.Now().UTC(),
    }

    // Store snapshot via repository layer (respects abstraction)
    go func() {
        if err := s.repo.CreateGitSnapshot(context.Background(), snapshot); err != nil {
            s.logger.Error("failed to create git snapshot",
                zap.String("session_id", data.TaskSessionID),
                zap.Error(err))
        }
    }()

    // Remove the old session.Metadata["git_status"] update
}
```

#### 3.2 Add Commit Event Handler

**File:** `apps/backend/internal/orchestrator/event_handlers.go`

**New handler for commit events:**

```go
// handleGitCommitCreated handles git commit events and creates commit records
func (s *Service) handleGitCommitCreated(ctx context.Context, data watcher.GitCommitData) {
    s.logger.Debug("handling git commit",
        zap.String("task_id", data.TaskID),
        zap.String("commit_sha", data.CommitSHA))

    if data.TaskSessionID == "" {
        return
    }

    // Create commit record
    commit := &models.SessionCommit{
        ID:            uuid.New().String(),
        SessionID:     data.TaskSessionID,
        CommitSHA:     data.CommitSHA,
        ParentSHA:     data.ParentSHA,
        AuthorName:    data.AuthorName,
        AuthorEmail:   data.AuthorEmail,
        CommitMessage: data.Message,
        CommittedAt:   data.CommittedAt,
        FilesChanged:  data.FilesChanged,
        Insertions:    data.Insertions,
        Deletions:     data.Deletions,
        CreatedAt:     time.Now().UTC(),
    }

    // Store via repository layer
    go func() {
        if err := s.repo.CreateCommitRecord(context.Background(), commit); err != nil {
            s.logger.Error("failed to create commit record",
                zap.String("session_id", data.TaskSessionID),
                zap.Error(err))
        }
    }()
}
```

**Register the handler:**

```go
// In orchestrator/watcher setup
watcher.SetHandlers(watcher.Handlers{
    OnGitStatusUpdated: s.handleGitStatusUpdated,
    OnGitCommitCreated: s.handleGitCommitCreated, // NEW
    // ... other handlers
})
```

#### 3.3 Add Commit Event Publishing from Agentctl

**File:** `apps/backend/internal/agentctl/server/process/git.go`

**Modify commit operation to publish commit metadata:**

```go
// Commit - publish commit event after successful commit
func (g *GitOperator) Commit(ctx context.Context, message string) error {
    // Perform commit
    err := g.runGitCommand(ctx, "commit", "-m", message)
    if err != nil {
        return err
    }

    // Get commit metadata
    commitSHA := g.getHeadCommitSHA(ctx)
    parentSHA := g.getParentCommitSHA(ctx)
    stats := g.getCommitStats(ctx, commitSHA)

    // Publish commit event via WebSocket
    g.publishCommitEvent(CommitEvent{
        CommitSHA:     commitSHA,
        ParentSHA:     parentSHA,
        Message:       message,
        AuthorName:    stats.AuthorName,
        AuthorEmail:   stats.AuthorEmail,
        CommittedAt:   time.Now(),
        FilesChanged:  stats.FilesChanged,
        Insertions:    stats.Insertions,
        Deletions:     stats.Deletions,
    })

    // Trigger workspace tracker refresh (git status will update)
    g.workspaceTracker.ForceRefresh()

    return nil
}
```

**This respects abstraction:**
- Agentctl publishes event via WebSocket
- Backend receives event
- Orchestrator creates commit record via repository

#### 3.4 Add New Event Types

**File:** `apps/backend/internal/events/events.go`

```go
const (
    // ... existing events

    // Git commit events
    GitCommitCreated = "git.commit.created"
)
```

**File:** `apps/backend/internal/orchestrator/watcher/watcher.go`

```go
type GitCommitData struct {
    TaskID        string
    TaskSessionID string
    CommitSHA     string
    ParentSHA     string
    Message       string
    AuthorName    string
    AuthorEmail   string
    CommittedAt   time.Time
    FilesChanged  int
    Insertions    int
    Deletions     int
}
```

### Phase 4: API Layer

#### 4.1 Add Snapshot Endpoints

**File:** `apps/backend/internal/agent/handlers/git_handlers.go`

**New WebSocket Actions:**

```go
// Register new handlers
d.RegisterFunc(ws.ActionSessionGitSnapshots, h.wsGetSnapshots)
d.RegisterFunc(ws.ActionSessionGitCommits, h.wsGetCommits)
d.RegisterFunc(ws.ActionSessionCumulativeDiff, h.wsGetCumulativeDiff)

// Handler implementations
func (h *GitHandlers) wsGetSnapshots(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
    var req struct {
        SessionID string `json:"session_id"`
    }
    if err := json.Unmarshal(msg.Payload, &req); err != nil {
        return nil, err
    }

    snapshots, err := h.repo.GetGitSnapshotsBySession(ctx, req.SessionID)
    if err != nil {
        return nil, err
    }

    return ws.NewResponse(msg.ID, map[string]interface{}{
        "snapshots": snapshots,
    })
}

func (h *GitHandlers) wsGetCommits(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
    var req struct {
        SessionID string `json:"session_id"`
    }
    if err := json.Unmarshal(msg.Payload, &req); err != nil {
        return nil, err
    }

    commits, err := h.repo.GetCommitsBySession(ctx, req.SessionID)
    if err != nil {
        return nil, err
    }

    return ws.NewResponse(msg.ID, map[string]interface{}{
        "commits": commits,
    })
}

func (h *GitHandlers) wsGetCumulativeDiff(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
    var req struct {
        SessionID string `json:"session_id"`
    }
    if err := json.Unmarshal(msg.Payload, &req); err != nil {
        return nil, err
    }

    diff, err := h.repo.GetCumulativeDiff(ctx, req.SessionID)
    if err != nil {
        return nil, err
    }

    return ws.NewResponse(msg.ID, map[string]interface{}{
        "cumulative_diff": diff,
    })
}
```

#### 4.2 Update WebSocket Action Constants

**File:** `apps/backend/pkg/websocket/actions.go`

```go
const (
    // ... existing actions

    // Git snapshot actions
    ActionSessionGitSnapshots    = "session.git.snapshots"
    ActionSessionGitCommits      = "session.git.commits"
    ActionSessionCumulativeDiff  = "session.git.cumulative_diff"
    ActionGitSnapshotCreated     = "session.git.snapshot.created"
    ActionGitCommitRecorded      = "session.git.commit.recorded"
)
```

---

## Frontend Implementation Plan

### Phase 1: State Management

#### 1.1 Add New State Slice

**File:** `apps/web/lib/state/slices/session-runtime/session-runtime-slice.ts`

**Add to state:**

```typescript
export interface SessionRuntimeState {
  // ... existing state

  // Git snapshots
  gitSnapshots: {
    bySessionId: Record<string, GitSnapshot[]>;
    loading: Record<string, boolean>;
  };

  // Session commits
  commits: {
    bySessionId: Record<string, SessionCommit[]>;
    loading: Record<string, boolean>;
  };

  // Cumulative diff
  cumulativeDiff: {
    bySessionId: Record<string, CumulativeDiff>;
    loading: Record<string, boolean>;
  };
}
```

**Add actions:**

```typescript
// Actions
setGitSnapshots: (sessionId: string, snapshots: GitSnapshot[]) =>
  set((draft) => {
    draft.gitSnapshots.bySessionId[sessionId] = snapshots;
    draft.gitSnapshots.loading[sessionId] = false;
  }),

setCommits: (sessionId: string, commits: SessionCommit[]) =>
  set((draft) => {
    draft.commits.bySessionId[sessionId] = commits;
    draft.commits.loading[sessionId] = false;
  }),

setCumulativeDiff: (sessionId: string, diff: CumulativeDiff) =>
  set((draft) => {
    draft.cumulativeDiff.bySessionId[sessionId] = diff;
    draft.cumulativeDiff.loading[sessionId] = false;
  }),
```

#### 1.2 Add Type Definitions

**File:** `apps/web/lib/state/slices/session-runtime/types.ts`

```typescript
export interface GitSnapshot {
  id: string;
  session_id: string;
  snapshot_type: 'status_update' | 'pre_commit' | 'post_commit' | 'pre_stage' | 'post_stage';
  branch: string;
  remote_branch: string;
  head_commit: string;
  base_commit: string;
  ahead: number;
  behind: number;
  files: Record<string, FileInfo>;
  triggered_by: string;
  metadata?: Record<string, unknown>;
  created_at: string;
}

export interface SessionCommit {
  id: string;
  session_id: string;
  commit_sha: string;
  parent_sha: string;
  author_name: string;
  author_email: string;
  commit_message: string;
  committed_at: string;
  pre_commit_snapshot_id: string;
  post_commit_snapshot_id: string;
  files_changed: number;
  insertions: number;
  deletions: number;
  created_at: string;
}

export interface CumulativeDiff {
  session_id: string;
  base_commit: string;
  head_commit: string;
  total_commits: number;
  files: Record<string, FileInfo>;
}
```

### Phase 2: WebSocket Handlers

#### 2.1 Update Git Status Handler

**File:** `apps/web/lib/ws/handlers/git-status.ts`

```typescript
export function registerGitStatusHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'session.git.status': (message) => {
      const payload = message.payload;
      if (!payload.session_id) return;

      // Still update runtime git status for immediate UI feedback
      store.getState().setGitStatus(payload.session_id, {
        branch: payload.branch,
        remote_branch: payload.remote_branch ?? null,
        modified: payload.modified,
        added: payload.added,
        deleted: payload.deleted,
        untracked: payload.untracked,
        renamed: payload.renamed,
        ahead: payload.ahead,
        behind: payload.behind,
        files: payload.files,
        timestamp: payload.timestamp,
      });
    },

    // New handlers
    'session.git.snapshot.created': (message) => {
      const payload = message.payload;
      if (!payload.session_id) return;

      // Fetch updated snapshots
      // This will be handled by the hook that subscribes to snapshots
    },

    'session.git.commit.recorded': (message) => {
      const payload = message.payload;
      if (!payload.session_id) return;

      // Fetch updated commits
      // This will be handled by the hook that subscribes to commits
    },
  };
}
```

### Phase 3: API Client

#### 3.1 Add Snapshot API Methods

**File:** `apps/web/lib/api/domains/session-api.ts`

```typescript
export async function fetchGitSnapshots(sessionId: string): Promise<GitSnapshot[]> {
  const response = await fetch(`/api/sessions/${sessionId}/git/snapshots`);
  if (!response.ok) throw new Error('Failed to fetch git snapshots');
  const data = await response.json();
  return data.snapshots;
}

export async function fetchSessionCommits(sessionId: string): Promise<SessionCommit[]> {
  const response = await fetch(`/api/sessions/${sessionId}/git/commits`);
  if (!response.ok) throw new Error('Failed to fetch session commits');
  const data = await response.json();
  return data.commits;
}

export async function fetchCumulativeDiff(sessionId: string): Promise<CumulativeDiff> {
  const response = await fetch(`/api/sessions/${sessionId}/git/cumulative-diff`);
  if (!response.ok) throw new Error('Failed to fetch cumulative diff');
  const data = await response.json();
  return data.cumulative_diff;
}
```

### Phase 4: Hooks

#### 4.1 Create Snapshot Hook

**File:** `apps/web/hooks/domains/session/use-git-snapshots.ts` (new file)

```typescript
import { useEffect } from 'react';
import { useStore } from '@/lib/state/store';
import { fetchGitSnapshots } from '@/lib/api/domains/session-api';

export function useGitSnapshots(sessionId: string | null) {
  const snapshots = useStore((state) =>
    sessionId ? state.gitSnapshots.bySessionId[sessionId] : undefined
  );
  const loading = useStore((state) =>
    sessionId ? state.gitSnapshots.loading[sessionId] : false
  );
  const setGitSnapshots = useStore((state) => state.setGitSnapshots);

  useEffect(() => {
    if (!sessionId) return;

    fetchGitSnapshots(sessionId)
      .then((data) => setGitSnapshots(sessionId, data))
      .catch((err) => console.error('Failed to fetch git snapshots:', err));
  }, [sessionId, setGitSnapshots]);

  return { snapshots, loading };
}
```

#### 4.2 Create Cumulative Diff Hook

**File:** `apps/web/hooks/domains/session/use-cumulative-diff.ts` (new file)

```typescript
import { useEffect } from 'react';
import { useStore } from '@/lib/state/store';
import { fetchCumulativeDiff } from '@/lib/api/domains/session-api';

export function useCumulativeDiff(sessionId: string | null) {
  const diff = useStore((state) =>
    sessionId ? state.cumulativeDiff.bySessionId[sessionId] : undefined
  );
  const loading = useStore((state) =>
    sessionId ? state.cumulativeDiff.loading[sessionId] : false
  );
  const setCumulativeDiff = useStore((state) => state.setCumulativeDiff);

  useEffect(() => {
    if (!sessionId) return;

    fetchCumulativeDiff(sessionId)
      .then((data) => setCumulativeDiff(sessionId, data))
      .catch((err) => console.error('Failed to fetch cumulative diff:', err));
  }, [sessionId, setCumulativeDiff]);

  return { diff, loading };
}
```

### Phase 5: UI Components

#### 5.1 Fix DiffView Component

**File:** `apps/web/components/task/task-changes-panel.tsx`

**Current Problem:**
```typescript
// WRONG: Passing entire diff as single hunk
<DiffView hunks={[file.diff]} />
```

**Solution 1: Parse Diff into Hunks**

```typescript
function parseDiffIntoHunks(diffContent: string): string[] {
  if (!diffContent) return [];

  const lines = diffContent.split('\n');
  const hunks: string[] = [];
  let currentHunk: string[] = [];
  let inHunk = false;

  for (const line of lines) {
    // Hunk header starts with @@
    if (line.startsWith('@@')) {
      if (currentHunk.length > 0) {
        hunks.push(currentHunk.join('\n'));
      }
      currentHunk = [line];
      inHunk = true;
    } else if (inHunk) {
      currentHunk.push(line);
    }
  }

  if (currentHunk.length > 0) {
    hunks.push(currentHunk.join('\n'));
  }

  return hunks;
}

// Usage
<DiffView hunks={parseDiffIntoHunks(file.diff)} />
```

**Solution 2: Use `data` Prop Instead**

The `@git-diff-view/react` library also accepts a `data` prop for complete diff:

```typescript
<DiffView
  data={file.diff}
  diffViewerMode="unified"
/>
```

#### 5.2 Add Cumulative Diff View

**File:** `apps/web/components/task/task-cumulative-diff-panel.tsx` (new file)

```typescript
'use client';

import { useCumulativeDiff } from '@/hooks/domains/session/use-cumulative-diff';
import { DiffView } from '@git-diff-view/react';

interface TaskCumulativeDiffPanelProps {
  sessionId: string;
}

export function TaskCumulativeDiffPanel({ sessionId }: TaskCumulativeDiffPanelProps) {
  const { diff, loading } = useCumulativeDiff(sessionId);

  if (loading) return <div>Loading cumulative diff...</div>;
  if (!diff) return <div>No cumulative diff available</div>;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-semibold">
          Total Changes from Base Branch
        </h3>
        <div className="text-sm text-muted-foreground">
          {diff.total_commits} commit{diff.total_commits !== 1 ? 's' : ''}
        </div>
      </div>

      <div className="space-y-2">
        {Object.entries(diff.files).map(([path, fileInfo]) => (
          <div key={path} className="border rounded-lg p-4">
            <div className="font-mono text-sm mb-2">{path}</div>
            <DiffView
              data={fileInfo.diff}
              diffViewerMode="unified"
            />
          </div>
        ))}
      </div>
    </div>
  );
}
```

#### 5.3 Add Session Timeline View

**File:** `apps/web/components/task/task-session-timeline.tsx` (new file)

```typescript
'use client';

import { useGitSnapshots } from '@/hooks/domains/session/use-git-snapshots';
import { useSessionCommits } from '@/hooks/domains/session/use-session-commits';

interface TaskSessionTimelineProps {
  sessionId: string;
}

export function TaskSessionTimeline({ sessionId }: TaskSessionTimelineProps) {
  const { snapshots } = useGitSnapshots(sessionId);
  const { commits } = useSessionCommits(sessionId);

  // Merge snapshots and commits into timeline
  const timeline = useMemo(() => {
    const items = [
      ...(snapshots?.map(s => ({ type: 'snapshot', data: s, timestamp: s.created_at })) ?? []),
      ...(commits?.map(c => ({ type: 'commit', data: c, timestamp: c.committed_at })) ?? []),
    ];
    return items.sort((a, b) =>
      new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime()
    );
  }, [snapshots, commits]);

  return (
    <div className="space-y-2">
      {timeline.map((item, idx) => (
        <div key={idx} className="border-l-2 pl-4 pb-4">
          {item.type === 'commit' ? (
            <div>
              <div className="font-semibold">Commit</div>
              <div className="text-sm text-muted-foreground">
                {item.data.commit_message}
              </div>
              <div className="text-xs text-muted-foreground">
                {item.data.files_changed} files,
                +{item.data.insertions} -{item.data.deletions}
              </div>
            </div>
          ) : (
            <div>
              <div className="font-semibold">Status Update</div>
              <div className="text-sm text-muted-foreground">
                {item.data.snapshot_type} - {item.data.triggered_by}
              </div>
            </div>
          )}
          <div className="text-xs text-muted-foreground mt-1">
            {new Date(item.timestamp).toLocaleString()}
          </div>
        </div>
      ))}
    </div>
  );
}
```

#### 5.4 Update Task Changes Panel

**File:** `apps/web/components/task/task-changes-panel.tsx`

**Add toggle between "Uncommitted Changes" and "Total Changes":**

```typescript
export function TaskChangesPanel({ sessionId }: TaskChangesPanelProps) {
  const [viewMode, setViewMode] = useState<'uncommitted' | 'cumulative'>('uncommitted');
  const gitStatus = useSessionGitStatus(sessionId);
  const { diff: cumulativeDiff } = useCumulativeDiff(sessionId);

  const filesToShow = viewMode === 'uncommitted'
    ? gitStatus?.files
    : cumulativeDiff?.files;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3>Changes</h3>
        <div className="flex gap-2">
          <button
            onClick={() => setViewMode('uncommitted')}
            className={viewMode === 'uncommitted' ? 'active' : ''}
          >
            Uncommitted
          </button>
          <button
            onClick={() => setViewMode('cumulative')}
            className={viewMode === 'cumulative' ? 'active' : ''}
          >
            Total from Base
          </button>
        </div>
      </div>

      {/* Render files */}
      {Object.entries(filesToShow ?? {}).map(([path, file]) => (
        <FileChangeItem key={path} path={path} file={file} />
      ))}
    </div>
  );
}
```

---

## Architecture Principles

### Abstraction Layer Boundaries

**Critical:** This refactor must respect existing abstraction layers in the Kandev architecture.

#### Backend Layers

1. **Repository Layer** (`internal/task/repository/`)
   - Owns all database access
   - Provides domain models and CRUD operations
   - **Never** bypass this layer to access SQLite directly

2. **Service/Orchestrator Layer** (`internal/orchestrator/`)
   - Business logic and coordination
   - Uses repository layer for persistence
   - Publishes events to event bus
   - **Never** accesses database directly

3. **Agent Lifecycle Layer** (`internal/agent/lifecycle/`)
   - Manages agent execution lifecycle
   - Publishes events for git status, file changes, etc.
   - **Never** accesses database directly

4. **Agentctl Layer** (`internal/agentctl/`)
   - Runs inside containers/standalone
   - Monitors workspace and git status
   - Communicates via WebSocket to backend
   - **No database access** (stateless)

#### Frontend Layers

1. **API Client Layer** (`lib/api/domains/`)
   - All HTTP/WebSocket communication
   - Returns typed data
   - **Never** access WebSocket directly from components

2. **State Management Layer** (`lib/state/`)
   - Zustand store with domain slices
   - Single source of truth
   - **Never** store server state in component state

3. **Hooks Layer** (`hooks/domains/`)
   - Encapsulates API calls + store subscriptions
   - Provides reactive data to components
   - **Never** call API directly from components

4. **Component Layer** (`components/`)
   - Pure presentation logic
   - Uses hooks for data
   - **Never** calls API or WebSocket directly

### Data Flow Constraints

**Backend:**
```
WorkspaceTracker → Event Bus → Orchestrator → Repository → Database
                                    ↓
                              WebSocket Gateway → Frontend
```

**Frontend:**
```
WebSocket → Handlers → Store → Hooks → Components
                ↓
         API Client → Store
```

**Rules:**
- Git snapshots created by `WorkspaceTracker` or `GitOperator`
- Snapshots persisted via `Repository` methods only
- Events published to event bus, not direct WebSocket
- Frontend receives via WebSocket handlers, updates store
- Components read from store via hooks

---

## Migration Strategy

**Note:** Since we're in dev phase, we can break compatibility. No gradual migration needed.

### Implementation Approach

1. **Database:** Drop and recreate with new schema
2. **Backend:** Remove `git_status` from `task_sessions.metadata` entirely
3. **Frontend:** Remove old metadata-based git status logic
4. **Clean break:** No backward compatibility, no fallback logic

---

## Data Retention & Cleanup

### Retention Policy

**Snapshots:**
- Keep all snapshots for active sessions
- For completed sessions:
  - Keep first snapshot (session start)
  - Keep all pre/post commit snapshots
  - Keep last snapshot (session end)
  - Delete intermediate `status_update` snapshots after 7 days

**Commits:**
- Keep all commit records permanently (small data size)

### Cleanup Job

**File:** `apps/backend/internal/task/repository/sqlite/cleanup.go`

```go
// CleanupOldGitSnapshots removes intermediate snapshots for completed sessions
func (r *Repository) CleanupOldGitSnapshots(ctx context.Context, retentionDays int) error {
    cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)

    _, err := r.db.ExecContext(ctx, `
        DELETE FROM task_session_git_snapshots
        WHERE snapshot_type = 'status_update'
        AND created_at < ?
        AND session_id IN (
            SELECT id FROM task_sessions
            WHERE state IN ('completed', 'failed', 'cancelled')
        )
        AND id NOT IN (
            -- Keep first and last snapshots
            SELECT id FROM (
                SELECT id, ROW_NUMBER() OVER (PARTITION BY session_id ORDER BY created_at ASC) as rn_asc,
                       ROW_NUMBER() OVER (PARTITION BY session_id ORDER BY created_at DESC) as rn_desc
                FROM task_session_git_snapshots
            ) WHERE rn_asc = 1 OR rn_desc = 1
        )
    `, cutoff)

    return err
}
```

---

## Testing Strategy

### Backend Tests

#### Unit Tests

**File:** `apps/backend/internal/task/repository/sqlite/git_snapshots_test.go`

```go
func TestCreateGitSnapshot(t *testing.T) {
    // Test snapshot creation
    // Test retrieval by session
    // Test retrieval by type
}

func TestGetCumulativeDiff(t *testing.T) {
    // Test diff calculation across multiple commits
    // Test with no commits
    // Test with staged but uncommitted changes
}

func TestCleanupOldGitSnapshots(t *testing.T) {
    // Test retention policy
    // Test that first/last snapshots are preserved
    // Test that commit snapshots are preserved
}
```

#### Integration Tests

**File:** `apps/backend/internal/agentctl/server/process/workspace_tracker_test.go`

```go
func TestWorkspaceTrackerSnapshotCreation(t *testing.T) {
    // Test that snapshots are created on file changes
    // Test that snapshots are created on git operations
    // Test debouncing behavior
}

func TestGitOperatorSnapshotTriggers(t *testing.T) {
    // Test pre/post stage snapshots
    // Test pre/post commit snapshots
    // Test commit record creation
}
```

### Frontend Tests

#### Component Tests

**File:** `apps/web/components/task/__tests__/task-changes-panel.test.tsx`

```typescript
describe('TaskChangesPanel', () => {
  it('should toggle between uncommitted and cumulative views', () => {
    // Test view mode switching
  });

  it('should parse diff into hunks correctly', () => {
    // Test diff parsing logic
  });

  it('should handle empty diffs gracefully', () => {
    // Test edge cases
  });
});
```

#### Hook Tests

**File:** `apps/web/hooks/domains/session/__tests__/use-cumulative-diff.test.ts`

```typescript
describe('useCumulativeDiff', () => {
  it('should fetch cumulative diff on mount', () => {
    // Test initial fetch
  });

  it('should update when session changes', () => {
    // Test session switching
  });
});
```

---

## Performance Considerations

### Database Indexing

**Critical Indexes:**
```sql
-- Fast session-based queries
CREATE INDEX idx_git_snapshots_session ON task_session_git_snapshots(session_id, created_at DESC);

-- Fast type-based queries
CREATE INDEX idx_git_snapshots_type ON task_session_git_snapshots(session_id, snapshot_type);

-- Fast commit lookups
CREATE INDEX idx_session_commits_session ON task_session_commits(session_id, committed_at DESC);
CREATE INDEX idx_session_commits_sha ON task_session_commits(commit_sha);
```

### Query Optimization

**Cumulative Diff Calculation:**
- Option 1: Store pre-computed cumulative diff in latest snapshot
- Option 2: Compute on-demand by running `git diff base_commit..HEAD`
- **Recommendation:** Option 2 (more accurate, less storage)

**Snapshot Retrieval:**
- Limit queries to last N snapshots (e.g., 100)
- Use pagination for timeline view
- Cache latest snapshot in memory

### WebSocket Optimization

**Avoid Snapshot Spam:**
- Don't broadcast every snapshot creation
- Only broadcast on significant events (commit, manual refresh)
- Use debouncing for rapid file changes



---

## Implementation Checklist

### Backend

- [ ] Create database migration for `task_session_git_snapshots` table
- [ ] Create database migration for `task_session_commits` table
- [ ] Add `GitSnapshot` and `SessionCommit` models
- [ ] Implement repository methods for snapshots
- [ ] Implement repository methods for commits
- [ ] Implement `GetCumulativeDiff` method
- [ ] Modify `WorkspaceTracker.updateGitStatus()` to create snapshots
- [ ] Modify `GitOperator.Stage()` to create pre/post snapshots
- [ ] Modify `GitOperator.Commit()` to create snapshots and commit records
- [ ] Update `handleGitStatusUpdated` event handler
- [ ] Add new WebSocket actions for snapshots/commits
- [ ] Add new API endpoints for snapshots/commits
- [ ] Implement cleanup job for old snapshots
- [ ] Write unit tests for repository methods
- [ ] Write integration tests for workspace tracker

### Frontend

- [ ] Add `gitSnapshots` state slice
- [ ] Add `commits` state slice
- [ ] Add `cumulativeDiff` state slice
- [ ] Add type definitions for new models
- [ ] Create `useGitSnapshots` hook
- [ ] Create `useSessionCommits` hook
- [ ] Create `useCumulativeDiff` hook
- [ ] Add API client methods for snapshots/commits
- [ ] Fix `DiffView` component crash (parse hunks or use `data` prop)
- [ ] Create `TaskCumulativeDiffPanel` component
- [ ] Create `TaskSessionTimeline` component
- [ ] Update `TaskChangesPanel` with view mode toggle
- [ ] Update WebSocket handlers for new events
- [ ] Write component tests
- [ ] Write hook tests

### Deployment

- [ ] Drop and recreate database with new schema
- [ ] Deploy backend with snapshot logic
- [ ] Deploy frontend with new components
- [ ] Test end-to-end locally
- [ ] Verify all git operations create snapshots correctly

---

## Success Metrics

### Functional Metrics

- ✅ No more "Invalid hunk header format" crashes
- ✅ Diff content updates immediately after stage/commit operations
- ✅ Can view cumulative diff from base branch
- ✅ Can view session timeline with commits and snapshots
- ✅ Git status persists across backend restarts

### Performance Metrics

- Snapshot creation latency: < 100ms
- Cumulative diff calculation: < 500ms
- Database query time: < 50ms
- WebSocket message size: < 100KB per snapshot

### Data Metrics

- Snapshot retention: 7 days for intermediate snapshots
- Storage growth: < 10MB per active session
- Cleanup job runs daily, removes > 90% of old snapshots

---

## Future Enhancements

### Phase 2 Features

1. **Diff Comparison Tool**
   - Compare any two snapshots
   - Show what changed between commits
   - Visual diff timeline

2. **Snapshot Annotations**
   - Add notes to snapshots
   - Tag important snapshots
   - Search snapshots by annotation

3. **Export Session History**
   - Export all snapshots as git patches
   - Generate session report with diffs
   - Share session history with team

4. **Smart Snapshot Triggers**
   - Create snapshot before risky operations
   - Auto-snapshot on test failures
   - Snapshot on agent permission requests

5. **Diff Analytics**
   - Track code churn per session
   - Identify frequently modified files
   - Measure session productivity

---

## Conclusion

This refactor addresses all current issues:

1. **Stale Diff:** Snapshots created after every git operation ensure fresh diff content
2. **DiffView Crash:** Fixed by properly parsing hunks or using `data` prop
3. **Transient Storage:** Persistent snapshots table replaces ephemeral metadata
4. **Missing Commits:** New `task_session_commits` table tracks all commits

The architecture is designed for:
- **Scalability:** Indexed queries, cleanup jobs, pagination
- **Abstraction Respect:** All layers maintain proper boundaries
  - Agentctl: Publishes events, no DB access
  - Orchestrator: Creates snapshots via repository layer
  - Repository: Owns all database operations
  - Frontend: API → Store → Hooks → Components
- **Extensibility:** Timeline view, cumulative diff, future analytics

**Estimated Implementation Time:**
- Backend: 3-4 days
  - Database schema + repository methods: 1 day
  - Event handlers + commit tracking: 1 day
  - API endpoints: 1 day
  - Testing: 1 day
- Frontend: 2-3 days
  - State management + hooks: 1 day
  - UI components: 1-2 days
  - Testing: 1 day
- **Total:** ~1 week

**Risk Level:** Low
- Clean break, no migration complexity
- Abstraction layers respected throughout
- Existing event flow reused

