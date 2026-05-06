# Office Review Gaps — Implementation Plan

5 blockers and 3 suggestions identified by external code review. Ordered by impact.

---

## Blocker 1: Task comments don't wake assigned agents

**Problem:** Comments are inserted into the DB but no `task_comment` wakeup is queued. When a reviewer posts feedback, the builder agent never wakes up.

**Files to modify:**
- `apps/backend/internal/office/dashboard/service.go` — `CreateComment` method
- `apps/backend/internal/office/service/event_subscribers.go` — `handleCommentCreated`
- `apps/backend/internal/office/channels/comments.go` — `publishCommentCreated`

**Implementation:**

1. In `dashboard/service.go`, after `CreateTaskComment` succeeds, publish an `OfficeCommentCreated` event via the event bus:
```go
func (s *DashboardService) CreateComment(ctx context.Context, taskID, authorType, authorID, body string) error {
    // ... existing create logic ...

    // Publish event for wakeup processing
    s.publishEvent(events.OfficeCommentCreated, map[string]string{
        "task_id": taskID,
        "author_type": authorType,
        "author_id": authorID,
    })
    return nil
}
```

2. In `service/event_subscribers.go`, the `handleCommentCreated` handler must:
   - Look up the task's `assignee_agent_instance_id`
   - Skip if the comment author IS the assigned agent (no self-wake)
   - Queue a `WakeupReasonTaskComment` wakeup with `{"task_id": ..., "comment_id": ...}` payload
```go
func (s *Service) handleCommentCreated(ctx context.Context, data map[string]string) {
    taskID := data["task_id"]
    authorID := data["author_id"]

    fields, err := s.repo.GetTaskExecutionFields(ctx, taskID)
    if err != nil || fields.AssigneeAgentInstanceID == "" {
        return // no assignee, nothing to wake
    }

    // Don't wake the agent for its own comments
    if fields.AssigneeAgentInstanceID == authorID {
        return
    }

    s.QueueWakeup(ctx, fields.AssigneeAgentInstanceID, WakeupReasonTaskComment,
        fmt.Sprintf(`{"task_id":%q,"comment_author":%q}`, taskID, authorID),
        fmt.Sprintf("comment:%s:%s", taskID, authorID))
}
```

3. Ensure the channel relay path (`channels/comments.go`) also publishes the same event when an inbound channel message creates a comment.

**Tests:**
- `TestCommentCreated_WakesAssignee` — create task with assignee, post comment from different author, verify wakeup queued
- `TestCommentCreated_SkipsSelfComment` — post comment from the assigned agent, verify no wakeup
- `TestCommentCreated_NoAssignee` — post comment on unassigned task, verify no wakeup
- `TestChannelInbound_WakesAssignee` — inbound channel message creates comment, verify wakeup

---

## Blocker 2: Wakeup lifecycle — finish after session ends, not after launch

**Problem:** `processWakeup` calls `StartTask` then immediately `finishWakeup`. The wakeup should stay `claimed` until the agent session completes or fails. This breaks retry (crashed agents appear "finished") and concurrency (checkout released too early).

**Files to modify:**
- `apps/backend/internal/office/service/scheduler_integration.go` — `processWakeup`, `launchOrLog`
- `apps/backend/internal/office/repository/sqlite/wakeups.go` — add `GetClaimedWakeupByTaskID`
- `apps/backend/internal/office/service/event_subscribers.go` — add session completion handler
- `apps/backend/internal/orchestrator/event_handlers_agent.go` — where session complete/fail events are emitted

**Implementation:**

1. **Remove the immediate `finishWakeup` call** from `processWakeup` / `launchOrLog`. After `StartTask` succeeds, leave the wakeup in `claimed` status.

2. **Store wakeup_id on the session/execution.** When `StartTask` creates a session, pass the wakeup ID so it can be retrieved later. Options:
   - Add `wakeup_id` to session metadata
   - Or store a mapping in the office repo: `office_wakeup_sessions(wakeup_id, session_id, task_id)`

3. **Finish the wakeup on session completion.** Subscribe to `session.completed` and `session.failed` events in the office event subscribers:
```go
func (s *Service) handleSessionCompleted(ctx context.Context, sessionID string) {
    wakeupID, err := s.repo.GetWakeupIDBySessionID(ctx, sessionID)
    if err != nil || wakeupID == "" {
        return // not an office-managed session
    }
    s.FinishWakeup(ctx, wakeupID)
}

func (s *Service) handleSessionFailed(ctx context.Context, sessionID, errorMsg string) {
    wakeupID, err := s.repo.GetWakeupIDBySessionID(ctx, sessionID)
    if err != nil || wakeupID == "" {
        return
    }
    s.HandleWakeupFailure(ctx, wakeupID, errorMsg)
}
```

4. **Add a stale wakeup cleanup.** Wakeups stuck in `claimed` for more than 30 minutes (session crashed without reporting) should be failed by the GC sweep or a periodic check in the tick loop.

**Tests:**
- `TestWakeup_StaysClaimedUntilSessionCompletes` — launch task, verify wakeup stays `claimed`, complete session, verify `finished`
- `TestWakeup_FailsOnSessionError` — launch task, fail session, verify wakeup enters retry
- `TestWakeup_StaleClaimedCleanup` — wakeup claimed 60 min ago with no session update, verify cleanup

---

## Blocker 3: Cost tracking not wired to real agent usage

**Problem:** `RecordCostEvent` has no production callers. The ACP protocol sends `context_window` events with token usage, but nobody records them as cost events.

**Files to modify:**
- `apps/backend/internal/orchestrator/event_handlers_agent.go` or `executor/executor_execute.go` — where ACP session events are processed
- `apps/backend/internal/office/service/event_subscribers.go` — subscribe to cost-relevant events
- `apps/backend/internal/events/types.go` — add cost event type if needed

**Implementation:**

1. **Identify where token usage is available.** In the ACP protocol, `session/update` notifications with type `context_window` contain token counts. These are processed in the orchestrator/executor layer. Read:
   - `apps/backend/internal/orchestrator/executor/executor_execute.go` — look for `context_window` handling
   - `apps/backend/internal/agentctl/server/adapter/transport/` — where ACP events are parsed

2. **Emit a cost event** when a session turn completes with token usage:
```go
// In the orchestrator's session completion handler:
if usage := session.TokenUsage; usage != nil {
    eventBus.Publish(events.OfficeSessionCostReported, map[string]interface{}{
        "session_id": session.ID,
        "task_id": session.TaskID,
        "agent_instance_id": agentInstanceID,
        "model": usage.Model,
        "tokens_in": usage.InputTokens,
        "tokens_out": usage.OutputTokens,
        "tokens_cached_in": usage.CachedInputTokens,
    })
}
```

3. **Subscribe in office event subscribers** to `OfficeSessionCostReported` and call `RecordCostEvent`:
```go
func (s *Service) handleSessionCostReported(ctx context.Context, data CostData) {
    costCents := s.CalculateCostCents(data.Model, data.TokensIn, data.TokensOut, data.TokensCachedIn)
    s.RecordCostEvent(ctx, CostEvent{
        SessionID: data.SessionID,
        TaskID: data.TaskID,
        AgentInstanceID: data.AgentInstanceID,
        Model: data.Model,
        TokensIn: data.TokensIn,
        TokensOut: data.TokensOut,
        TokensCachedIn: data.TokensCachedIn,
        CostCents: costCents,
    })
}
```

4. **Replace static pricing** with LiteLLM community pricing (optional, can be a follow-up):
   - Fetch `https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json` on startup
   - Cache in memory with 24h TTL
   - Fall back to the existing static table on fetch failure

**Tests:**
- `TestCostEvent_RecordedOnSessionComplete` — complete a session with token usage, verify cost event in DB
- `TestBudget_EnforcedWithRealCosts` — record costs up to budget limit, verify next wakeup is blocked

---

## Blocker 4: Channel webhook authentication

**Problem:** Inbound webhook only has body size limit and TODO for signature verification. No per-platform auth.

**Files to modify:**
- `apps/backend/internal/office/channels/handler.go` — `channelInbound`
- `apps/backend/internal/office/models/models.go` — `Channel` struct (add `WebhookSecret` field)
- `apps/backend/internal/office/repository/sqlite/channels.go` — persist webhook secret

**Implementation:**

1. **Add `webhook_secret` to Channel model** — generated on channel creation, stored in DB.

2. **Add generic HMAC verification** to the inbound handler:
```go
func (h *Handler) channelInbound(c *gin.Context) {
    channelID := c.Param("channelId")
    channel, err := h.svc.GetChannelByID(ctx, channelID)
    if err != nil {
        c.JSON(404, gin.H{"error": "channel not found"})
        return
    }

    // Verify webhook signature if secret is configured
    if channel.WebhookSecret != "" {
        sig := c.GetHeader("X-Webhook-Signature")
        if sig == "" {
            sig = c.GetHeader("X-Telegram-Bot-Api-Secret-Token")
        }
        if sig == "" {
            sig = c.GetHeader("X-Hub-Signature-256") // GitHub-style
        }
        if !verifyHMAC(channel.WebhookSecret, bodyBytes, sig) {
            c.JSON(401, gin.H{"error": "invalid signature"})
            return
        }
    }
    // ... rest of handler
}
```

3. **Add per-channel rate limiting** using a simple in-memory token bucket (or skip for v1 and add later).

4. **Return the webhook secret** on channel creation so the user can configure it in their platform.

**Tests:**
- `TestChannelInbound_ValidSignature` — post with correct HMAC, verify accepted
- `TestChannelInbound_InvalidSignature` — post with wrong HMAC, verify 401
- `TestChannelInbound_NoSecret` — channel with no secret configured, verify accepted (backward compat)

---

## Blocker 5: Git-sourced skills unsupported

**Problem:** `MaterializeSkills` returns "not yet implemented" for git sources.

**Files to modify:**
- `apps/backend/internal/office/skills/materialization.go` — implement git clone
- `apps/backend/internal/office/skills/import.go` — git URL parsing (may already exist partially)

**Implementation:**

1. **Clone and cache git repos.** When a skill has `source_type = "git"` and `source_locator = "https://github.com/user/repo"`:
```go
func materializeGitSkill(locator, cacheDir string) (string, error) {
    repoDir := filepath.Join(cacheDir, hashLocator(locator))
    if dirExists(repoDir) {
        // Pull latest
        cmd := exec.Command("git", "-C", repoDir, "pull", "--ff-only")
        cmd.Run()
    } else {
        // Clone
        cmd := exec.Command("git", "clone", "--depth=1", locator, repoDir)
        if err := cmd.Run(); err != nil {
            return "", fmt.Errorf("git clone %s: %w", locator, err)
        }
    }
    return repoDir, nil
}
```

2. **Extract SKILL.md** from the cloned repo at the expected path (root or `skills/<name>/SKILL.md`).

3. **List file inventory** — scan the cloned directory for `references/`, `scripts/`, `assets/` subdirs.

4. **Safe path validation** — ensure the locator doesn't contain path traversal (`../`), and the clone dir is under the cache directory.

5. **Cache invalidation** — on skill update, delete the cached clone and re-clone.

**Tests:**
- `TestMaterializeGitSkill_ClonesRepo` — mock git clone, verify SKILL.md extracted
- `TestMaterializeGitSkill_PullsOnUpdate` — cached repo exists, verify pull instead of clone
- `TestMaterializeGitSkill_InvalidURL` — bad URL, verify error
- `TestMaterializeGitSkill_PathTraversal` — locator with `../`, verify rejected

---

## Suggestion 1: Issue detail activity + sessions

**File:** `apps/web/app/office/issues/[id]/page.tsx:139`

**Problem:** Activity and sessions are hardcoded empty arrays:
```typescript
const activity: IssueActivityEntry[] = [];
const sessions: TaskSession[] = [];
```

**Fix:** Fetch from the API:
- Activity: `GET /api/v1/office/workspaces/:wsId/activity?task_id=:taskId` (or add a per-task activity endpoint)
- Sessions: `GET /api/v1/tasks/:taskId/sessions` (existing kanban endpoint)

Wire them into `useIssueData`:
```typescript
const [activity, setActivity] = useState<IssueActivityEntry[]>([]);
const [sessions, setSessions] = useState<TaskSession[]>([]);

useEffect(() => {
    fetchActivity(taskId).then(setActivity);
    fetchSessions(taskId).then(setSessions);
}, [taskId]);
```

---

## Suggestion 2: Spec index is stale

**File:** `docs/specs/INDEX.md`

**Fix:** Add missing entries and an implementation status column:
```markdown
| Slug | Status | Implemented? | Created | PR |
|------|--------|-------------|---------|-----|
| [office-overview](office-overview/spec.md) | draft | partial | 2026-04-25 | — |
| [office-agent-context](office-agent-context/spec.md) | draft | yes | 2026-04-27 | — |
| [office-onboarding](office-onboarding/spec.md) | draft | yes | 2026-04-27 | — |
```

---

## Suggestion 3: Client pages refetch after SSR

**Files:** Various office pages (costs, settings, etc.)

**Fix:** Hydrate complete page data into the office store during SSR, then read from the store in client components instead of re-fetching. Follow the existing `SSR fetch → hydrate → read store` pattern from `lib/state/hydration/`.

This is a broader refactor — defer to a follow-up PR.

---

## Priority order

1. **Comment → wakeup** (Blocker 1) — ~2 hours. Breaks the review feedback loop.
2. **Wakeup lifecycle** (Blocker 2) — ~4 hours. Breaks retry and concurrency.
3. **Cost ingestion** (Blocker 3) — ~4 hours. Makes budgets decorative.
4. **Channel auth** (Blocker 4) — ~2 hours. Security gap.
5. **Git skills** (Blocker 5) — ~4 hours. Feature gap.
6. **Issue detail UX** (Suggestion 1) — ~1 hour. Easy win.
7. **Spec index** (Suggestion 2) — ~15 min. Bookkeeping.
8. **SSR refetch** (Suggestion 3) — defer to follow-up.

## Files summary

| Blocker | Backend files | Frontend files |
|---------|--------------|---------------|
| 1. Comment wakeups | `dashboard/service.go`, `service/event_subscribers.go`, `channels/comments.go` | — |
| 2. Wakeup lifecycle | `service/scheduler_integration.go`, `repository/sqlite/wakeups.go`, `service/event_subscribers.go` | — |
| 3. Cost ingestion | `orchestrator/executor/executor_execute.go` or `event_handlers_agent.go`, `service/event_subscribers.go` | — |
| 4. Channel auth | `channels/handler.go`, `models/models.go`, `repository/sqlite/channels.go` | — |
| 5. Git skills | `skills/materialization.go`, `skills/import.go` | — |
| S1. Issue detail | — | `app/office/issues/[id]/page.tsx` |
| S2. Spec index | `docs/specs/INDEX.md` | — |
