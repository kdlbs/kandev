---
status: draft
created: 2026-05-10
owner: cfl
---

# Office Heartbeat & Routine Rework

## Why

Today every office workspace gets one **standing "Workspace coordination" task** assigned to the coordinator/CEO agent. Heartbeat ticks, budget alerts, and agent-error escalations all enqueue runs against this single task. The task lives forever; the agent's session is resumed on every fire (`scheduler_integration.go:256-261` — `isResume=true` after the first run).

Two concrete problems:

1. **Serial bottleneck.** Runs claim a per-(agent, task) lock — the comment in `scheduler_integration.go:273-281` documents this explicitly:
   > "the run claim acts as the agent-is-busy lock that ClaimNextEligibleRun respects, so new runs (comments, status changes) for the same agent + task queue up rather than racing the active turn."
   With one task absorbing every heartbeat in the workspace, a single 30-minute coordinator turn stalls every other heartbeat-driven event behind it.

2. **Unbounded conversation growth.** A resumed session is the literal CLI conversation log. Six months in, that conversation contains every heartbeat ever fired plus every comment, every tool call, every retry. The model's context window is not designed for this. Concretely the agent will: hit auto-compaction with unpredictable quality loss; burn budget reading megabytes of stale context on every fire; start producing degraded answers because the recent context is buried under months of noise.

There is no concurrency policy, no catch-up cap, no compaction strategy. The system worked in early dev because no one ran it for long.

The target architecture for kandev's office tier: It does not change the workflow engine; the existing `on_heartbeat` trigger on workflow steps stays for the case where a long-lived task wants periodic ticks. What changes is the **default coordinator-wake mechanism** — it shifts from "queue a run against the standing task" to "queue a fresh, taskless run against the agent, with a continuation summary".

## What

### Core model shift

| Today | After |
|-------|-------|
| Heartbeat → run against standing task → resume session | Heartbeat → wakeup-request row → fresh run against agent (no task) → fresh session |
| Conversation grows forever | Conversation = one fire's worth, summary doc bridges fires |
| Implicit per-task lock = bottleneck | Per-routine concurrency policy = explicit, configurable |
| No catch-up control | Catch-up cap = 25 (configurable per routine), drop or compress beyond that |
| Routines: placeholder `task-<runid>`, no real task | Routines: lightweight (no task) or heavy (fresh task in routine workflow) per policy |

### New entities

#### `agent_continuation_summaries`
Per (agent_profile_id, scope) markdown blob bridging context across fires.

```
agent_profile_id  TEXT  NOT NULL  -- the agent the summary belongs to
scope             TEXT  NOT NULL  -- "heartbeat" | "routine:<id>"
content           TEXT  NOT NULL  DEFAULT ''  -- markdown body, capped at 8 KB
content_tokens    INT   NOT NULL  DEFAULT 0   -- approx token count for budget logging
updated_at        TIMESTAMP NOT NULL
updated_by_run_id TEXT NOT NULL DEFAULT ''
PRIMARY KEY (agent_profile_id, scope)
```

Writes are upsert (one current row per scope, no history table). Reads truncate to a configurable budget (default 1,500 chars in the prompt).

#### `agent_wakeup_requests`
The unifying queue for "this agent should wake up". Cron heartbeats, comment events, agent-error escalations, user mentions, agent self-requests all create rows here. The dispatcher coalesces / claims / drops them per the routine's policy and creates the actual `runs` row.

```
id                       TEXT PRIMARY KEY
agent_profile_id         TEXT NOT NULL
source                   TEXT NOT NULL  -- "heartbeat" | "comment" | "agent_error" | "routine" | "self" | "user"
reason                   TEXT NOT NULL  -- short label for telemetry
payload                  TEXT NOT NULL DEFAULT '{}'  -- JSON
status                   TEXT NOT NULL  -- "queued" | "claimed" | "coalesced" | "skipped" | "completed" | "failed" | "cancelled"
coalesced_count          INT  NOT NULL DEFAULT 1
idempotency_key          TEXT
run_id                   TEXT  -- the run this request fulfilled (when status terminal)
requested_at             TIMESTAMP NOT NULL
claimed_at               TIMESTAMP
finished_at              TIMESTAMP
INDEX (agent_profile_id, status)
INDEX (idempotency_key)
```

The runs table changes minimally — we add three columns to support taskless runs and the inspection surface (see *Run inspection surface* below):

```
result_json       TEXT  NOT NULL  DEFAULT '{}'  -- structured adapter output (see Continuation summary)
assembled_prompt  TEXT  NOT NULL  DEFAULT ''    -- final prompt as the agent saw it
summary_injected  TEXT  NOT NULL  DEFAULT ''    -- the summary that was prepended (snapshot for inspection)
```

`runs.payload.task_id` is empty for taskless runs; everything else (status machinery, claim/coalesce, cost rollup) carries over unchanged.

### Heartbeat flow (new)

1. `internal/scheduler/cron/agent_heartbeat.go` — new cron handler.
   1. List agents with `heartbeat.enabled=true` and `next_heartbeat_at <= now`.
   2. For each: gate (paused / cooldown / catch-up cap reached).
   3. Insert `agent_wakeup_requests` row with `source=heartbeat`, idempotency key `heartbeat:<agent>:<unix_minute>`.
   4. Advance `next_heartbeat_at`. If we missed > 25 ticks, **collapse them**: skip the missed runs but emit a single "you missed N ticks since X" line that lands in the next prompt's wake context.

2. The wakeup-dispatcher (existing engine, slightly extended):
   1. Claim the request.
   2. Apply the agent's heartbeat concurrency policy:
      - `skip_if_active` — if any heartbeat run is in flight for this agent, mark this request `skipped`.
      - `coalesce_if_active` — same, but increment `coalesced_count` on the in-flight request and drop this one.
      - `always_enqueue` — proceed regardless. (Default: `coalesce_if_active`.)
   3. Read `agent_continuation_summaries[agent, "heartbeat"]`.
   4. Insert a `runs` row with `agent_profile_id` set, `payload.task_id` empty, `reason=heartbeat`. Mark the wakeup request `claimed`, `run_id=<run>`.
   5. Hand to the existing `scheduler_integration.processRun` path.

3. Prompt assembly (modified `BuildAgentPrompt`):
   - Detect taskless runs (`taskID==""`).
   - For taskless runs, prepend the continuation summary (sliced to 1,500 chars) and skip the workflow-engine wake context.
   - For task-bound runs, behaviour is unchanged.

4. Session policy:
   - Taskless runs **always start a fresh session**. The existing `HasPriorSessionForAgent(taskID, agentID)` check returns false trivially when `taskID==""`. Add a defensive `taskID==""` short-circuit in the same function so we never resume across fires.

5. Run finish (existing AgentCompleted subscriber):
   - On success, call the **summary builder** (see *Continuation summary contract* below) with the run's `result_json`, the prior summary, and the workspace's recent state. Upsert the resulting markdown into `agent_continuation_summaries[agent, "heartbeat"]` (truncated to 8 KB).
   - On failure, leave the previous summary intact. Last-good wins.

### Routine flow (new)

Today routines write `LinkedTaskID = "task-<runID[:8]>"` (a placeholder, not a real task). Replace with two real shapes governed by routine config:

| Routine kind | What dispatch does |
|---|---|
| **lightweight** (`task_template` empty) | Same shape as heartbeat. Wakeup-request → fresh taskless run. Summary scoped to `routine:<id>`. Use case: a "check upstream PRs" routine that doesn't need a trackable artifact. |
| **heavy** (`task_template` set) | Create a fresh task in the routine's workflow (new template `routine.yml`, system-flagged). Run is a normal task-bound run. Use case: "daily review" where the agent's output should be a Linear-style trackable item. |

The new `routine.yml` workflow template is a single auto-completing step:

```yaml
id: routine
name: Routine
steps:
  - id: in_progress
    name: In Progress
    is_start_step: true
    events:
      on_enter:
        - type: auto_start_agent
      on_turn_complete:
        - type: move_to_step
          config: { step_id: done }
  - id: done
    name: Done
```

Add `routine` to `SystemWorkflowTemplateIDs` so heavy routine tasks inherit the system-task hide-by-default UX from the recent toggle work.

Routine concurrency policy: same enum as heartbeat, default `coalesce_if_active`, decided at dispatch by querying for an in-flight run for the same routine. Translate to: `findLiveRunForRoutine(routineID, fingerprint)` reading `agent_wakeup_requests` + `runs` joined.

Catch-up cap: per-routine `catch_up_policy ∈ {skip_missed, enqueue_missed_with_cap}`, cap = 25, dropped runs not recorded individually but summarized into the next prompt.

### Continuation summary contract

Markdown sections, max 8 KB total. The prompt slice (1,500 chars) is just the head — the builder keeps the most-important content first.

```markdown
## Active focus
2-3 lines. What the coordinator is currently watching/driving.

## Open blockers
Bullet list. Each: blocker + what's needed to unblock + when surfaced.

## Recent decisions
Bullet list. Last ~5 things the coordinator committed to. Date-stamped.

## Next action
One sentence. The single next thing to do on the next wake-up.
```

#### Generation: server-synthesised, not agent-written

The agent **does not write the summary**. A new server-side builder composes the markdown deterministically from structured inputs after each successful run. This avoids the failure modes of asking the agent to write prose summaries: rotting prompt instructions, fragile parsing, inconsistency across CLIs, summaries lost on agent error.

**Inputs the builder consumes** (`internal/office/summary/builder.go`, new file):

| Input | Source | Used for |
|---|---|---|
| `run.result_json` | New text column on `runs`. Adapters populate it with structured outputs naturally — fallback chain: `result_json.summary → .result → .message → .error`. | "Recent actions" + "Recent decisions" sections. |
| Workspace activity stats | Existing `office_activity_log` and `runs` tables — counts of completed/failed tasks since last summary, agent-error escalations, budget signals. | "Active focus" + opening blocker context. |
| Active blockers | Tasks in `BLOCKED` state assigned to agents the coordinator manages. | "Open blockers" section. |
| `previousSummaryBody` | The prior summary in `agent_continuation_summaries`. Pulled forward to keep continuity (e.g. a prior "Next action" stays visible if this run didn't supersede it). | All sections — used as fallback content. |
| Inferred next action | Decision table on `(workspace state, last run status)`. | "Next action" — falls back to "Continue monitoring." when nothing concrete. |

The agent contributes by populating `result_json` (mostly happens for free via the adapter contract — ACP/Codex/OpenCode all expose final-result data). It is **not asked** to write summary prose. If a future need emerges for the agent to record decisions explicitly, the right answer is a **dedicated `record_decision(text)` tool** that writes to a `result_json.decisions[]` array — but that is out of scope for v1.

#### Builder placement

Single Go package, ~150 lines:

```
internal/office/summary/
  builder.go      // BuildSummary(ctx, inputs) → string
  builder_test.go // table-driven tests for each section
  inputs.go       // BuildInputs struct + the queries that populate it
```

Called from the `AgentCompleted` event subscriber on successful taskless runs. Idempotent — re-running with the same inputs produces the same output.

### Configuration surface

Per agent (`agent_profiles` or a sibling `agent_runtime_config` row):

```
heartbeat_enabled            BOOL   default true for coordinator role, false otherwise
heartbeat_interval_seconds   INT    default 60
heartbeat_concurrency        TEXT   default "coalesce_if_active"
heartbeat_catch_up_policy    TEXT   default "enqueue_missed_with_cap"
heartbeat_catch_up_max       INT    default 25
```

Per routine: same shape but in `routines` table; today's `concurrency_policy` column is partially modeled — finish it.

### What goes away

- The `Workspace coordination` task created during onboarding (`maybeCreateCoordinationTask`).
- The `coordination` workflow's `on_heartbeat`/`on_budget_alert`/`on_agent_error` actions are no longer fired by the standing-task path. The workflow can be deleted from the YAML, or kept as documentation of "the workflow-engine heartbeat trigger contract" — recommend **delete** to avoid two heartbeat paths confusing readers.
- The `LinkedTaskID = "task-<runid>"` placeholder in routines.

### What stays

- The workflow-engine `on_heartbeat` trigger (`internal/scheduler/cron/heartbeat.go`). It's still useful for tasks that genuinely want periodic ticks while open — long investigations, watch-rooms. We just stop using it as the default coordinator-wake path.
- The runs queue / claim / coalesce machinery. All of it transfers; we're just inserting taskless runs through the same pipeline.
- `runs.output_summary` is kept as a free-form text catch-all (still useful for the run-detail UI), but the summary builder reads from `runs.result_json` as the primary structured source.

### Run inspection surface

Heartbeat and routine runs are normal `runs` rows, so they slot into the existing `/office/agents/[id]/runs/[runId]` detail route for free. That UI already shows: status, timing, cost rollup (input / output / cached tokens, `cost_cents`), per-run skill snapshots (content_hash, version, materialized_path), capabilities, `input_snapshot`, embedded session replay, and the `run.event.appended` event log. Backed by `GetRunDetail` returning a `RunDetail` aggregate, with WS live updates via `useRunLiveSync`.

For a developer debugging a misbehaving heartbeat, three small additions complete the picture. None of them require new components — they extend the existing `RunDetail` payload + `RuntimePanel` rendering:

1. **Expose stored snapshots in the detail response.** `runs.context_snapshot` and `runs.output_summary` are already persisted but `GetRunDetail` doesn't return them. Add both to the response struct. Bonus: also surface `result_json` (new column above) so the UI can render the structured output the summary builder consumed.

2. **Persist the assembled prompt** the agent received. New text column `runs.assembled_prompt`, written at run dispatch (the point where `BuildAgentPrompt` finishes assembling the final string). The detail UI gets a new "Prompt" tab that renders it as monospace text. Without this, the user sees `input_snapshot` (a JSON of context fields) and the skill snapshots, but never the actual rendered prompt — which is what they need when an agent does something weird.

3. **Surface the injected continuation summary.** New text column `runs.summary_injected`, captured at the moment the prompt is assembled (snapshot of `agent_continuation_summaries[agent, scope].content` as it was at that instant). The detail UI shows it next to the prompt. Critical for "the summary said X but the agent acted as if it said Y" debugging — the summary mutates between fires, so without a per-run snapshot the post-hoc DB read is the wrong answer.

These three additions also enable a future "replay this run" button (the underlying inputs are now all captured) — out of scope for v1 but cheap once the data is there.

The cost-tracking story is unchanged: `office_cost_events` already keys events by `agent_profile_id` so heartbeat runs (taskless or not) have their tokens / cost attributed correctly. Per-message granularity is not provided today and is out of scope for this spec.

## Out of scope

- A full agent-memory subsystem (vector store, semantic recall, etc.). The summary doc is deliberately the *minimum* viable memory layer.
- Web UI for editing summaries directly. Read-only display in the agent's overview tab is enough for v1.
- Backfilling historical heartbeat conversations into summaries. The existing standing tasks die at the cutover; the summary starts blank.

## Risks

- **Quality loss without conversation memory.** A resumed session lets the agent reference "the comment two heartbeats ago"; under the new model that information must be in the summary or it's gone. The server-synthesised builder mitigates this for objective state (workspace activity, blockers, recent run results) since those come from the DB, not from the agent's memory. The lossy slice is "what the agent was *thinking* between fires" — caught only via `result_json` populated by the adapter, or eventually by an explicit `record_decision` tool. Audit the first week of agent behaviour after rollout.
- **Heartbeat run cost.** Each fire pays the full prompt-and-response cost. The continuation summary is small (1,500 chars ≈ 400 tokens) but it's still real. Idle-skip (`docs/specs/idle-wakeup-skip/spec.md`) covers the no-actionable-work case; combined with that, taskless heartbeats should be cheap.
- **Frontend assumptions.** The agent overview tab, recent activity, and inbox may all assume "run → task". Need an audit pass — the `system tasks` toggle work proved this assumption is shallow but not eliminated.
- **Migration.** User confirmed early-dev DB recreate is fine, no migration needed.

## Resolved design choices

The two open questions from earlier drafts are now settled:

### Coalescing model — three layers, one job each

Coalescing happens entirely at the **wakeup-request layer**, with the runs table acting as the execution record.

- **`agent_wakeup_requests.idempotency_key`** — source-level dedup. UNIQUE column. Format `heartbeat:<agent>:<unix_minute>` for cron heartbeats; `comment:<id>` for comments; `routine:<routine_id>:<trigger_id>:<unix_minute>` for routines. Duplicate inserts in the same window are rejected silently — that's the cron-tick-fired-twice case.
- **Claim-time merge** — when the dispatcher processes a wakeup-request, it looks for an in-flight run for the same agent (queued → scheduled-retry → running, in that order). If one exists: insert the new request with `status="coalesced"`, `run_id=<existing>`; **merge the new request's payload into the existing run's `context_snapshot`**; increment `coalesced_count`. The agent will see the merged context when it actually runs. If none exists: insert the wakeup-request with `status="queued"` and create the corresponding `runs` row.
- **`runs.idempotency_key`** — kept as a defensive secondary key. Rarely tripped now (the wakeup-request layer handles the common case), but useful for the rare "two cron processes fired the same tick from different leaders" scenario during a leadership change.

Three layers: source dedupes (idempotency_key), claim coalesces (merge into in-flight), run is the execution.

### Payload schema — free-form JSON in DB, typed structs in code

`agent_wakeup_requests.payload` is `TEXT DEFAULT '{}'`. The DB column is intentionally free-form so adding a new wakeup source doesn't require a migration. Type safety lives in code: `internal/office/wakeup/payloads.go` defines a Go struct per `source` enum value, and the dispatcher unmarshals per-source on claim:

```go
type HeartbeatPayload struct {
    MissedTicks int `json:"missed_ticks,omitempty"` // when catch-up cap collapsed N fires
}
type CommentPayload struct {
    TaskID    string `json:"task_id"`
    CommentID string `json:"comment_id"`
}
type AgentErrorPayload struct {
    AgentProfileID string `json:"agent_profile_id"` // the failing agent
    RunID          string `json:"run_id"`            // the run that failed
    Error          string `json:"error"`
}
type RoutinePayload struct {
    RoutineID string         `json:"routine_id"`
    Variables map[string]any `json:"variables,omitempty"`
}
// ...one per source. Switched on agent_wakeup_requests.source.
```

Adding a new source = add a struct + a switch case; no schema dance, but typing errors caught at the boundary not by reading raw JSON in 12 different consumers. The discipline of typed Go structs versus free-form `jsonb` with ad-hoc parsing per source.

## Implementation plan

Three PRs; each is independently shippable behind a per-workspace flag `office.agent_heartbeat_enabled` (default off until the third PR lands).

### PR 1 — Summary infra + run inspection extensions (no behaviour change)
- Add `agent_continuation_summaries` table + repo (CRUD with 8 KB cap on write).
- Add `runs.result_json`, `runs.assembled_prompt`, `runs.summary_injected` columns (default empty so existing rows stay valid).
- Add the `internal/office/summary` package (`builder.go` + `inputs.go` + tests).
- Modify `BuildAgentPrompt` to prepend the summary and persist `assembled_prompt` + `summary_injected` on the run row when `taskID==""`. Today no caller passes empty taskID, so the prepend branch is a no-op until PR 2; the persistence runs for every dispatch and is useful immediately for the existing run-detail UI.
- Extend `GetRunDetail` to return the new columns plus the existing `context_snapshot` / `output_summary` (small response-struct change). Add a "Prompt" panel to `RunDetailView` rendering `assembled_prompt` + `summary_injected`.
- Hook AgentCompleted: when a run has `taskID==""`, call the summary builder and upsert. Also no-op until PR 2.
- Tests: repo CRUD, cap behaviour, builder table-driven for each markdown section, prompt prepend with the new branch, run-detail response includes the new fields.

### PR 2 — Agent-level heartbeat path + flag
- Add `agent_wakeup_requests` table + repo + dispatcher (with the three-layer coalesce model from *Resolved design choices*).
- Add `internal/office/wakeup/payloads.go` with the per-source typed structs.
- Add `internal/scheduler/cron/agent_heartbeat.go` with the cron loop, gates, and catch-up cap.
- Add `agent_profiles.heartbeat_*` columns + onboarding defaults (heartbeat enabled for coordinator role only).
- Add the flag check in `maybeCreateCoordinationTask` — when flag is on, skip creating the standing task; archive any existing one.
- Update `HasPriorSessionForAgent` to return false for taskID=="" (defensive).
- Tests: heartbeat cron with `synctest`, catch-up-cap behaviour, concurrency policies, fresh-session-per-fire, claim-time coalesce merging payloads into `context_snapshot`.
- Manual smoke: enable the flag in dev, watch three heartbeats fire, confirm fresh sessions, summary updates between fires, run-detail UI shows the prompt + injected summary for each.

### PR 3 — Routine task materialisation + concurrency policy
- Add `config/workflows/routine.yml` template; register `routine` in `SystemWorkflowTemplateIDs`.
- Replace `LinkedTaskID = "task-<runid>"` with real task creation (heavy) or empty (lightweight), driven by routine config.
- Plumb `concurrency_policy` and `catch_up_policy` through the routine dispatcher.
- Tests: heavy routine creates a real task; lightweight does not; coalesce vs always_enqueue vs skip_missed; catch-up cap.
- Frontend audit: ensure agent overview tab + inbox handle taskless runs and routine-task tasks correctly. Likely a small TaskRow / ActivityRow update.

After PR 3, flip the default to on in a follow-up; remove the legacy `Workspace coordination` task creation entirely.

## Telemetry & observability

- Per heartbeat fire: log `(agent, scope, summary_chars, summary_tokens, prompt_total_tokens, dropped_catch_up, queue_lag_ms)`.
- Per skipped/coalesced wakeup-request: log source + reason + the active wakeup it deferred to.
- Add a `kandev_heartbeat_runs_total` / `_skipped_total` / `_dropped_catch_up_total` counter set if Prometheus is wired in this project (check before adding).

## Decisions to lock before coding

1. **Summary scope key**: `"heartbeat"` (per agent) vs `"agent:<id>"` vs `"agent:<id>:heartbeat"`. Recommend `"heartbeat"` — `agent_profile_id` is already half the row key. Composite values become useful only once an agent has multiple summary kinds (e.g. one per routine), at which point `"routine:<id>"` slots in alongside `"heartbeat"`.
2. **Coordinator default**: should every coordinator agent have heartbeat on by default? Recommend yes — that's the user-visible promise of "the coordinator wakes up periodically". Workers default to off.
3. **In-flight standing-task runs at flag flip**: recommend letting them complete naturally — new wakeups go through the new path, the standing task is archived only after the last in-flight run finishes. Avoids the cancel-mid-turn ugly path.
4. **Sub-minute heartbeats** (deferred): the current idempotency-key format `heartbeat:<agent>:<unix_minute>` assumes cadence ≥ 1 minute. If we ever offer faster cadences, the format becomes `heartbeat:<agent>:<unix_seconds>` and the cadence floor in the cron handler needs revisiting. Not in scope today; flagged so the format isn't accidentally locked in elsewhere.
