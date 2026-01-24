# Manual Git Commit Sync on Page Refresh - Feasibility Analysis

## Problem Statement

When users execute git commands directly in the shell/console (e.g., `git commit`, `git reset`, `git rebase`), the UI and data model don't reflect these changes. The user has accepted that manual git operations will only sync on page refresh. This document analyzes whether we can **fully synchronize** our data model with the actual git state when the UI first requests the data.

**Full sync means:**
- **Add** commits that exist in git but not in our model (manual `git commit`)
- **Remove** commits that exist in our model but not in git (manual `git reset`, `git rebase`)

## Current Data Model

### Key Entities

1. **SessionCommit** (`task_session_commits` table)
   - Stores commits made during a session
   - Fields: `commit_sha`, `parent_sha`, `author_name`, `author_email`, `commit_message`, `committed_at`, `files_changed`, `insertions`, `deletions`
   - Currently populated only by API-initiated commits via `GitOperator`

2. **GitSnapshot** (`task_session_git_snapshots` table)
   - Point-in-time snapshots of git state
   - Fields: `head_commit`, `base_commit`, `branch`, `ahead`, `behind`, `files`
   - First snapshot contains `base_commit` (the commit SHA the session started from)

3. **Worktree** (`task_session_worktrees` table)
   - Associates sessions with worktrees
   - Fields: `worktree_path`, `worktree_branch`, `repository_id`
   - `BaseBranch` stored in `Worktree` model (the branch the worktree was created from)

4. **TaskSession** (`task_sessions` table)
   - Has `repository_id`, `base_branch` fields
   - Associated with worktrees via `TaskSessionWorktree`

### Key Relationships

```
TaskSession
    └── TaskSessionWorktree (worktree_path, worktree_branch)
    └── GitSnapshot (first has base_commit = starting point)
    └── SessionCommit (commits we track)
```

## Detection Strategy

### Data Available for Commit Range

1. **Starting Point**: First `GitSnapshot.base_commit` OR resolve `origin/{baseBranch}` in worktree
2. **Current HEAD**: Run `git rev-parse HEAD` in worktree path
3. **Worktree Path**: From `TaskSessionWorktree.worktree_path`

### Sync Algorithm

```
1. Get worktree path for session (from TaskSessionWorktree)
2. Get base_commit from first GitSnapshot (or derive from BaseBranch)
3. Run: git log --format="%H|%P|%an|%ae|%s|%aI" base_commit..HEAD
4. Parse commits from git output → actualCommits (set of SHAs)
5. Get existing SessionCommit records → storedCommits (set of SHAs)
6. Compute diff:
   - toAdd = actualCommits - storedCommits (commits in git, not in DB)
   - toRemove = storedCommits - actualCommits (commits in DB, not in git)
7. Insert toAdd commits
8. Delete toRemove commits
```

This handles:
- `git commit` → new commit appears in actualCommits → added to DB
- `git reset --hard HEAD~1` → commit disappears from actualCommits → removed from DB
- `git rebase` → old commits gone, new commits appear → DB updated accordingly
- `git commit --amend` → old SHA gone, new SHA appears → old removed, new added

### Git Command to List Commits

```bash
git log --format="%H|%P|%an|%ae|%s|%aI" --numstat base_commit..HEAD
```

This provides:
- `%H` - commit SHA
- `%P` - parent SHA(s)
- `%an` - author name
- `%ae` - author email
- `%s` - subject (commit message)
- `%aI` - author date (ISO 8601)
- `--numstat` - files changed, insertions, deletions

## Critical Constraint: Remote Execution

**The backend cannot directly execute git commands in the worktree because:**

1. **Docker containers** - The worktree lives inside a container, not accessible from the host filesystem
2. **Remote execution** - The agent might run on a remote VPS or Kubernetes pod
3. **Isolation** - agentctl is the **only** component with filesystem access to the workspace

**Therefore: All git operations must go through agentctl.**

## Architecture Options

### Option A: Handler Layer via Lifecycle Manager (Recommended)

**Location**: `task/handlers/TaskHandlers.wsGetSessionCommits()` or `agent/handlers/GitHandlers`

**Flow**:
```
Frontend (page refresh)
    → WS: session.git.commits
    → Handler: wsGetSessionCommits
        → Check if session has running agentctl (via lifecycle manager)
        → If running: call agentctl API to get git log
        → Sync commits in DB (add missing, remove orphaned)
        → Return synchronized commits
        → If not running: return stored commits only (no sync possible)
```

**Changes Required**:
1. Add new agentctl endpoint: `GET /api/v1/git/log` (returns commits in range)
2. Add agentctl client method: `GitLog(ctx, baseCommit) ([]*CommitInfo, error)`
3. Modify handler to call agentctl and sync before returning
4. Add `DeleteSessionCommit` to repository

**Pros**:
- Works with Docker, remote, and Kubernetes execution
- Leverages existing agentctl infrastructure
- Handler already has access to lifecycle manager

**Cons**:
- Sync only works when session is running (agentctl available)
- Adds latency to the request (git command execution)

### Option B: Sync on Session Start/Resume

**Location**: `agent/lifecycle/SessionManager.InitializeSession()`

**Flow**:
```
Session start/resume
    → agentctl becomes available
    → Backend calls agentctl to get git log
    → Sync commits in DB
    → (Later) Frontend requests commits → returns already-synced data
```

**Pros**:
- Sync happens once at session start, not on every request
- Commits are ready when UI requests them

**Cons**:
- Doesn't sync if user does manual git after session starts
- Still need Option A for mid-session sync

### Option C: Hybrid (Recommended)

Combine both:
1. **On session start**: Sync commits via agentctl
2. **On page refresh (if session running)**: Re-sync via agentctl
3. **If session not running**: Return stored commits (stale but acceptable)

This provides:
- Fresh data when session is active
- Best-effort data when session is stopped
- User understands "refresh while running" = fresh data

## Recommended Implementation (Option C - Hybrid)

### 1. New agentctl Endpoint

```go
// In agentctl/server/api/git.go

// GET /api/v1/git/log?base_commit=<sha>
// Returns all commits from base_commit to HEAD
func (s *Server) handleGitLog(c *gin.Context) {
    baseCommit := c.Query("base_commit")
    if baseCommit == "" {
        c.JSON(400, gin.H{"error": "base_commit is required"})
        return
    }

    commits, err := s.procMgr.GetGitOperator().Log(c.Request.Context(), baseCommit)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }

    c.JSON(200, gin.H{"commits": commits})
}
```

### 2. GitOperator.Log Method

```go
// In agentctl/server/process/git.go

type CommitInfo struct {
    SHA          string    `json:"sha"`
    ParentSHA    string    `json:"parent_sha"`
    AuthorName   string    `json:"author_name"`
    AuthorEmail  string    `json:"author_email"`
    Message      string    `json:"message"`
    CommittedAt  time.Time `json:"committed_at"`
    FilesChanged int       `json:"files_changed"`
    Insertions   int       `json:"insertions"`
    Deletions    int       `json:"deletions"`
}

func (g *GitOperator) Log(ctx context.Context, baseCommit string) ([]*CommitInfo, error) {
    // git log --format="%H|%P|%an|%ae|%s|%aI" --numstat base_commit..HEAD
    cmd := exec.CommandContext(ctx, "git", "log",
        "--format=%H|%P|%an|%ae|%s|%aI",
        "--numstat",
        baseCommit+"..HEAD")
    cmd.Dir = g.workDir
    output, err := cmd.Output()
    if err != nil {
        return nil, err
    }
    return parseGitLog(string(output))
}
```

### 3. agentctl Client Method

```go
// In agentctl/client/client.go

func (c *Client) GitLog(ctx context.Context, baseCommit string) ([]*CommitInfo, error) {
    resp, err := c.get(ctx, "/api/v1/git/log?base_commit="+url.QueryEscape(baseCommit))
    if err != nil {
        return nil, err
    }
    var result struct {
        Commits []*CommitInfo `json:"commits"`
    }
    if err := json.Unmarshal(resp, &result); err != nil {
        return nil, err
    }
    return result.Commits, nil
}
```

### 4. Handler with Sync Logic

```go
// In task/handlers/task_handlers.go

func (h *TaskHandlers) wsGetSessionCommits(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
    var req wsGetSessionCommitsRequest
    if err := msg.ParsePayload(&req); err != nil {
        return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
    }
    if req.SessionID == "" {
        return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
    }

    // Try to sync if session is running
    if err := h.syncCommitsIfRunning(ctx, req.SessionID); err != nil {
        h.logger.Warn("commit sync failed", zap.Error(err), zap.String("session_id", req.SessionID))
        // Non-fatal - continue with stored commits
    }

    commits, err := h.controller.GetSessionCommits(ctx, req.SessionID)
    if err != nil {
        h.logger.Error("failed to get session commits", zap.Error(err), zap.String("session_id", req.SessionID))
        return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get session commits", nil)
    }

    return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
        "session_id": req.SessionID,
        "commits":    commits,
    })
}

func (h *TaskHandlers) syncCommitsIfRunning(ctx context.Context, sessionID string) error {
    // 1. Check if session has running agentctl
    execution, found := h.lifecycle.GetExecutionBySessionID(sessionID)
    if !found {
        return nil // Not running, skip sync
    }
    client := execution.GetAgentCtlClient()
    if client == nil {
        return nil // No client, skip sync
    }

    // 2. Get base commit from first snapshot
    baseCommit, err := h.controller.GetSessionBaseCommit(ctx, sessionID)
    if err != nil || baseCommit == "" {
        return nil // No base commit, skip sync
    }

    // 3. Get actual commits from agentctl
    actualCommits, err := client.GitLog(ctx, baseCommit)
    if err != nil {
        return fmt.Errorf("git log failed: %w", err)
    }

    // 4. Sync with DB
    return h.controller.SyncSessionCommits(ctx, sessionID, actualCommits)
}
```

### 5. Controller Sync Method

```go
// In task/controller/task.go

func (c *TaskController) SyncSessionCommits(ctx context.Context, sessionID string, actual []*CommitInfo) error {
    return c.service.SyncSessionCommits(ctx, sessionID, actual)
}

func (c *TaskController) GetSessionBaseCommit(ctx context.Context, sessionID string) (string, error) {
    snapshot, err := c.service.GetFirstGitSnapshot(ctx, sessionID)
    if err != nil {
        return "", err
    }
    return snapshot.BaseCommit, nil
}
```

### 6. Service Sync Method

```go
// In task/service/service.go

func (s *Service) SyncSessionCommits(ctx context.Context, sessionID string, actual []*CommitInfo) error {
    stored, err := s.repo.GetSessionCommits(ctx, sessionID)
    if err != nil {
        return err
    }

    storedSHAs := make(map[string]*models.SessionCommit)
    for _, c := range stored {
        storedSHAs[c.CommitSHA] = c
    }

    actualSHAs := make(map[string]bool)
    for _, c := range actual {
        actualSHAs[c.SHA] = true
    }

    // Add commits in git but not in DB
    for _, c := range actual {
        if _, exists := storedSHAs[c.SHA]; !exists {
            commit := &models.SessionCommit{
                SessionID:     sessionID,
                CommitSHA:     c.SHA,
                ParentSHA:     c.ParentSHA,
                AuthorName:    c.AuthorName,
                AuthorEmail:   c.AuthorEmail,
                CommitMessage: c.Message,
                CommittedAt:   c.CommittedAt,
                FilesChanged:  c.FilesChanged,
                Insertions:    c.Insertions,
                Deletions:     c.Deletions,
            }
            if err := s.repo.CreateSessionCommit(ctx, commit); err != nil {
                return fmt.Errorf("failed to add commit %s: %w", c.SHA, err)
            }
        }
    }

    // Remove commits in DB but not in git
    for sha, commit := range storedSHAs {
        if !actualSHAs[sha] {
            if err := s.repo.DeleteSessionCommit(ctx, commit.ID); err != nil {
                return fmt.Errorf("failed to remove commit %s: %w", sha, err)
            }
        }
    }

    return nil
}
```

### Abstraction Layer Assessment

| Layer | Current Responsibility | Added Responsibility | Breaks Abstraction? |
|-------|------------------------|----------------------|---------------------|
| agentctl | Workspace operations | GitLog endpoint | No - fits existing pattern |
| Repository | Data access (CRUD) | DeleteSessionCommit | No - standard CRUD |
| Service | Business logic | SyncSessionCommits | No - business logic |
| Controller | Request/response mapping | GetSessionBaseCommit, SyncSessionCommits | No - orchestration |
| Handler | WS/HTTP routing | syncCommitsIfRunning | Minor - but has lifecycle access |

**Verdict**: This follows existing patterns where handlers access lifecycle manager for agentctl operations (see `agent/handlers/git_handlers.go`). The sync logic is appropriately distributed across layers.

## Edge Cases

1. **Session not running** - No agentctl available; return stored commits (stale but acceptable)
2. **No first snapshot** - Cannot determine base commit; skip sync
3. **Git command fails** - Log warning, return existing commits
4. **Concurrent requests** - Use session-level mutex or idempotent operations
5. **Rewritten history (reset, rebase, amend)** - Handled by removing orphaned commits
6. **Force push from remote** - If user pulls rewritten history, sync will update accordingly
7. **Merge commits** - Handled normally; parent_sha may have multiple parents
8. **Container restarted** - agentctl restarts, sync works on next request

## Required Repository Changes

A new method is needed to delete commits:

```go
// In task/repository/interface.go
DeleteSessionCommit(ctx context.Context, id string) error

// In task/repository/sqlite/git_snapshots.go
func (r *Repository) DeleteSessionCommit(ctx context.Context, id string) error {
    _, err := r.db.ExecContext(ctx, `DELETE FROM task_session_commits WHERE id = ?`, id)
    return err
}
```

## Implementation Summary

| Component | Changes |
|-----------|---------|
| `agentctl/server/api` | Add `GET /api/v1/git/log` endpoint |
| `agentctl/server/process/git.go` | Add `GitOperator.Log()` method |
| `agentctl/client/client.go` | Add `GitLog()` client method |
| `task/repository/interface.go` | Add `DeleteSessionCommit()` |
| `task/repository/sqlite` | Implement `DeleteSessionCommit()` |
| `task/service/service.go` | Add `SyncSessionCommits()` |
| `task/controller/task.go` | Add `SyncSessionCommits()`, `GetSessionBaseCommit()` |
| `task/handlers/task_handlers.go` | Add `syncCommitsIfRunning()`, modify `wsGetSessionCommits` |

## Conclusion

**Yes, this is feasible without breaking abstraction layers.**

The implementation requires routing through agentctl since the workspace may be in a container or remote environment. The sync happens:
1. **On page refresh** - If session is running, sync via agentctl before returning commits
2. **If session stopped** - Return stored commits (user understands they may be stale)

This is a "lazy sync" pattern - we only sync when data is requested AND the session is running. The sync is **bidirectional**:
- Commits added manually → appear in UI after refresh (if session running)
- Commits removed (reset/rebase/amend) → disappear from UI after refresh (if session running)

**Limitation**: If the session is stopped, manual git operations won't sync until the session is started again. This is acceptable given the constraint that agentctl must be running to access the workspace.

