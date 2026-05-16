---
status: draft
created: 2026-05-04
owner: cfl
needs-upgrade: [API surface, Permissions, Persistence guarantees, Scenarios]
---

# Office Agent Runtime — Error Handling Contract

This spec defines the runtime contract for how Office agent errors are observed, classified, and recovered. It covers what the lifecycle layer publishes on failure, how sessions and wakeups transition, how failures surface in the inbox and per-task chat, and how the user recovers — via Mark fixed, Resume session, or reassignment to a different agent.

## Why

When an agent run fails mid-turn (invalid model, auth failure, malformed response, transient upstream), today's UX is:

1. The chat shows a raw JSON-RPC blob in red: `{"code":-32603,"message":"Internal error","data":{"codex_error_info":"other","message":"…"}}`.
2. The session ends in `WAITING_FOR_INPUT`, which the chat reads as "live" - the topbar spinner and the "Agent working for Xs" header stay on even though the agent has stopped.
3. The wakeup scheduler queues a retry with the standard backoff table (~135 s). For a 400 `invalid_request_error` (e.g. unsupported model) the retry fails identically and burns CPU + tokens for nothing. A flaky provider can produce a tight retry loop that never surfaces to the user.
4. The agent detail Runs tab and the dashboard agent card show no sign of the failure. The user has to open the task to know.
5. Nothing surfaces in the office inbox even though the existing `office-inbox` spec already lists "Agent errors" as one of its computed sources.
6. With multiple agents using different providers, there's no path to recover by reassigning a task - the failed wakeup keeps firing on the original (broken) agent regardless of who's actually assigned.

Three classes of bug are mixed up here: a state-machine bug (`FAILED` modeled as `WAITING_FOR_INPUT`), a retry-policy bug (every error is treated as retryable), and a presentation bug (raw error blob shown verbatim, no inbox entry, no recovery path).

The target pattern: runs persist with explicit status (`queued | running | scheduled_retry | failed | timed_out | cancelled`); failed runs surface in the inbox as a computed view over the runs table with a `inbox_dismissals` row keyed by run id; the sidebar badge counts un-dismissed failures; the run-ledger card shows a humanised liveness reason rather than the raw payload. Scoped tightly for v1.

## What

### v1 classification policy: every agent error is terminal

The runtime does **not** classify errors in v1. Any error event from the agent adapter - Codex, Claude, Copilot, Amp, OpenCode, Auggie, Gemini, mock - is treated as terminal. The user fixes the cause (bad model, expired auth, network outage, rate limit) and re-runs the task by clicking **Resume session** in the chat or **Mark fixed** in the inbox.

This is a deliberate trade-off: the cost of a classifier per adapter is not worth paying yet. Treating every error as terminal means we may surface a transient failure that would have self-recovered on the next retry; the cost is one click. Treating retryable as terminal is always safe; treating terminal as retryable burns CPU and tokens. We prefer the safe direction.

A follow-up spec can introduce per-adapter classification once we have real usage data on which errors are common and worth auto-retrying. The existing `rate-limit-retry` spec is independent of this work - it parses provider rate-limit reset times. In v1 we treat rate limits as generic errors and let the user reassign or wait, rather than auto-detect.

### Retry policy

- No automatic retry. Every adapter error is terminal for the wakeup that produced it.
- The wakeup row is stamped with `status = failed` and `error_message`. No follow-up wakeup is queued from the failure path.
- Re-runs only happen via explicit user action: **Resume session** in chat, **Mark fixed** on an inbox entry, or task reassignment to a different agent.

### Inbox surfacing

Failed wakeups surface in the inbox, fulfilling the "Agent errors" bullet already in the `office-inbox` spec. Two kinds:

- **`agent_run_failed`** - one entry per failed (task, agent) wakeup while the agent is below threshold. Title: "<agent> failed on <task>". Action: **Mark fixed** -> dismiss + retry.
- **`agent_paused_after_failures`** - one entry per auto-paused agent. Title: "<agent> auto-paused after <N> failures (tasks A, B, C)". Action: **Mark fixed** -> unpause + retry the affected tasks.

When auto-pause triggers, the threshold-th failure does **not** create its own per-task entry. Instead, the prior `agent_run_failed` entries for that agent are auto-dismissed and replaced with a single `agent_paused_after_failures` entry that lists the affected tasks. The inbox shows one row per broken agent rather than N rows per failed task. (Per-task chat error entries are unaffected - they live in each task's chat thread.)

### Recovery hooks

#### Mark fixed = dismiss + retry

Both inbox actions converge on the same effect: clear the failure state and re-run.

For `agent_run_failed`:
1. Insert into `inbox_dismissals`.
2. Clear `FAILED` on the (task, agent) session -> `IDLE`.
3. Re-queue a wakeup for that (task, agent) with reason `manual_resume_after_failure`.

For `agent_paused_after_failures`:
1. Insert into `inbox_dismissals`.
2. Clear the agent's `pause_reason`, reset `consecutive_failures` to zero. (Counter reset is the user saying "I fixed the cause; start over".)
3. Re-queue wakeups for every (task, agent) listed on the entry with reason `manual_resume_after_failure`.

There is no separate "Dismiss without retry" affordance. If the user wants to silence the inbox without re-running the task, they reassign the task to a different agent - that's the explicit recovery path described below.

#### Reassignment as the multi-provider failover path

Multiple agents may run on different providers (e.g. CEO on Claude, Worker on Codex). When one provider hits a sustained outage or rate limit, the user can switch the assignee on a per-task basis to recover without waiting for the broken provider.

Reassignment is independent of the pause / dismiss flow:

- Changing `assignee_agent_instance_id` on a task fires the existing reactivity pipeline, which queues a fresh `task_assigned` wakeup for the new agent.
- The existing staleness check (`recovery-reliability` spec) cancels the prior wakeup for the (task, **old** agent) since the assignee has changed.
- Any per-task `agent_run_failed` inbox entry tied to the old (task, agent) auto-dismisses - the failure is no longer actionable on this task.
- The old agent's `consecutive_failures` counter is **not** reset by reassignment. The agent remains paused (or close to pause) since the root cause hasn't been fixed; reassigning is a workaround for the specific task, not an unpause.
- An `agent_paused_after_failures` entry persists until the user explicitly **Mark fixed**s it, regardless of how many of its affected tasks have been reassigned away. (The list of "affected tasks" on the entry is a snapshot at pause time; we don't recompute on reassign.)

Worked example with the rate-limit scenario:

1. Workspace has CEO (Claude) and Worker (Codex), threshold = 3.
2. Claude rate-limits. CEO fails on tasks A, B, then C.
3. After A: inbox row `agent_run_failed` for (A, CEO).
4. After B: inbox row `agent_run_failed` for (B, CEO).
5. After C: pause triggers. Rows for A and B auto-dismiss. New row `agent_paused_after_failures` for CEO covering {A, B, C}.
6. User reassigns task B to Worker (Codex). Codex's wakeup runs B successfully. The CEO entry's affected list still mentions B (we don't recompute) but the user can ignore that detail.
7. CEO stays paused. User waits for the rate limit window or fixes billing, then clicks **Mark fixed** on the CEO entry -> unpause + retry A and C (B is already done; the retry is a no-op for B because its current assignee is Worker).

### Chat surface (per-task)

The red banner that today shows the raw JSON-RPC blob is replaced by a structured **error entry** in the unified chat timeline (alongside comments, session entries, decisions). The entry contains:

- A short generic header: "The agent stopped with an error."
- A `Show details` collapsible revealing the raw error payload verbatim - what the user copy-pastes when filing a bug.
- The existing **Resume session** + **Start fresh** action buttons, unchanged. Resume session is equivalent to **Mark fixed** on the per-task inbox entry (same backend effect).

The entry sorts at the failure timestamp like any other comment, so the chat reads chronologically: `prompt -> tool calls -> error -> (next user comment if any) -> next turn`.

The runtime does **not** humanise the error message in v1. The raw payload is the most useful artefact when classification is absent.

### Sidebar agent indicator

The sidebar Agents list (`SidebarAgentsList`) currently renders, per row, either a `LiveAgentIndicator` (live count when > 0) or an `AgentStatusDot`. The runtime adds two cases:

- **Auto-paused agent** - a small red "paused" badge replaces the status dot, with a tooltip showing the pause reason. Takes priority over the live indicator (if a paused agent has somehow racing live count, the paused state is more important).
- **Agent with un-dismissed failures below pause threshold** - a red "<N> errors" badge alongside the live/status indicator. N is the count of `agent_run_failed` inbox entries for this agent.

The Inbox sidebar item already shows a total badge count; this is a per-agent surface for users who triage by agent rather than by inbox.

### Runs tab pill

The agent detail page Runs tab gains a status pill per row: `queued | running | failed | completed | cancelled`. The pill colour reuses the dashboard agent-card vocabulary. A failed run row links to the task that produced it.

### Dashboard agent card

When the agent's most recent wakeup is `failed` or the agent is auto-paused, the dashboard agent card subtitle changes:

- Most recent wakeup failed but agent below threshold: "Last run failed {ago}".
- Agent auto-paused: "Paused - <N> consecutive failures".

Same component, new branches in `pickSubtitle` and `StatusDot` (red dot in both cases).

## Data model

### `agent_instances` additions

- `consecutive_failures INT NOT NULL DEFAULT 0` - counter across any task assigned to this agent. Increments on every failure regardless of which task; resets to zero on any successful run for the agent.
- `failure_threshold INT NULL` - per-agent override. When NULL, the workspace default applies.
- `pause_reason TEXT NULL` - reused from manual pause. On auto-pause the runtime sets `"Auto-paused: <N> consecutive failures. Last error: <message>"`.

### `workspace_settings` additions

- `agent_failure_threshold INT NOT NULL DEFAULT 3` - workspace default threshold. Configurable from the workspace settings page. Per-agent override on `agent_instances.failure_threshold` wins when present. Rationale: twice is coincidence, three times is a pattern. Critical agents can be tuned down to 1, flaky-but-tolerable agents up to 5.

### `inbox_dismissals`

New table: `(user_id, item_kind, item_id, dismissed_at)` with a unique constraint on `(user_id, item_kind, item_id)`. `item_kind` is one of `agent_run_failed | agent_paused_after_failures` to start; the table is reusable for future inbox sources.

### Wakeup row

- `status` extended with `failed` (in addition to the existing values).
- `error_message TEXT NULL` - stamped on failure with the raw error payload.

## State machine

### Session state on agent error

Office sessions transition to a terminal `FAILED` state distinct from `WAITING_FOR_INPUT`. When the lifecycle layer publishes an error event:

- Session state -> `FAILED`.
- Task state -> `REVIEW` (already happens today; unchanged).
- Wakeup status -> `failed`, with `error_message` stamped, no retry queued.
- The `executors_running` row is preserved so the user can resume via the existing **Resume session** button. The agent process and agentctl instance are torn down.

This fixes today's bug where the topbar spinner stays on after the agent has actually stopped: a `FAILED` session is correctly read as non-live by `isLiveSession`.

### Run status lifecycle

`queued -> running -> {completed | failed | timed_out | cancelled}`. The `scheduled_retry` status from the target pattern is reserved for a future classifier; v1 produces only the listed terminal states.

### Consecutive-failure counter and auto-pause

Each agent carries `consecutive_failures` on the `agent_instances` row. It increments on every failure across any task assigned to this agent. Any successful run for the agent resets it to zero.

When the counter reaches the agent's threshold (per-agent override or workspace default), the agent is auto-paused: `pause_reason` is set to `"Auto-paused: <N> consecutive failures. Last error: <message>"`. The wakeup scheduler refuses to claim wakeups for a paused agent (existing behaviour); the counter is preserved across the pause so unpause-and-fail-immediately re-pauses without surprise.

## Failure modes

- **Adapter error event published**: session -> `FAILED`, wakeup -> `failed`, agent process and agentctl torn down, `executors_running` preserved, `consecutive_failures++`, inbox entry created or upgraded to `agent_paused_after_failures` on threshold.
- **Provider returns malformed response**: treated as terminal like any other adapter error. Raw payload preserved in the chat error entry and wakeup `error_message`.
- **Transient network failure**: treated as terminal in v1. User clicks Resume / Mark fixed to retry. Future classifier may auto-retry.
- **Auto-paused agent's wakeup fires**: scheduler refuses to claim it; behaviour identical to a manually-paused agent.
- **Reassignment during failure**: cancels the (task, old agent) wakeup via the existing staleness check; auto-dismisses `agent_run_failed` for (task, old agent). Does not unpause the old agent.

## Out of scope

- No per-adapter error classification, no rate-limit reset parsing in this spec. Generic terminal treatment for every error.
- No "Retry now" button. Resume session / Mark fixed cover manual retry.
- No automatic agent unpause. Pause only clears via explicit user action (Mark fixed on the inbox entry, or manual unpause from the agent detail page).
- No per-error-code action variant in the chat. Until we classify, every error gets the same Resume / Start fresh affordance.
- No notification provider integration (Local/OS/Apprise). The existing `office.inbox_item` event from the inbox spec covers it once that work lands.
- The runtime does not recompute the `affected_tasks` list on the `agent_paused_after_failures` entry when tasks are reassigned away. The list is a snapshot at pause time; the user resolves it as a whole.
