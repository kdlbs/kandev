---
status: draft
created: 2026-05-10
owner: cfl
---

# Heartbeat-as-Routine: convert to routines-driven wakeups

## Why

The previous spec (`docs/specs/office-heartbeat-rework`) shipped the agent-level heartbeat path: a system-level cron in `internal/scheduler/cron/agent_heartbeat.go` ticks every minute, finds coordinators with `agent_profiles.heartbeat_enabled=1`, and dispatches a fresh taskless run via the wakeup-request infrastructure. The settings — interval, concurrency policy, catch-up policy, the enabled flag — live as columns on `agent_profiles` and are **invisible to the user**. There is no UI to inspect, disable, or tune the cadence; you'd `UPDATE agent_profiles SET heartbeat_interval_seconds=300 …` directly.

The target design: there is **no agent-level cron at all**. Every periodic wake-up is a row in the `routines` table with one or more `routineTriggers` (`schedule | webhook | api`). The coordinator ships with no automatic heartbeat — it sits idle until the user (or a plugin) installs a routine. The benefit is total transparency: a single screen lists every periodic job, the user can edit cron / concurrency / catch-up, disable, or delete any of them, and there's no second mechanism doing wake-ups behind their back.

Hidden background work erodes trust; "the coordinator wakes up every minute and we silently spend tokens for it" is exactly the kind of thing a user wants to know about and turn off when they're not actively working. A routine can do everything our agent-level cron does plus support webhook and manual triggers for free, and we already have the routines infrastructure (table, dispatcher integration, workflow template — all from PR 3 of the prior spec).

Desired properties:

- **No system-level agent heartbeat cron.** Only `routines.tickScheduledTriggers` drives periodic wakes. No `agents.heartbeat_*` columns.
- **No default routine is auto-installed at onboarding.** The coordinator gets instruction files and that's it. To get periodic wake-ups, the user installs a routine — or a plugin manifest declares one.
- **`HEARTBEAT.md` is just instruction text** — read on every wake-up regardless of source. It's not wired to a specific routine name. We can keep our coordinator HEARTBEAT.md verbatim; nothing about the migration touches the instructions.
- **Routines UI is rich**: cron expression + timezone, concurrency policy enum (`coalesce_if_active | always_enqueue | skip_if_active`), catch-up policy enum (`skip_missed | enqueue_missed_with_cap`), trigger kinds (`schedule | webhook | api`), variables array, status (`active | paused | archived`), priority. Plugin-managed routines marked with a badge. Status is editable on plugin routines; metadata is locked.
- **We already have most of the routine infrastructure** — `office_routines` table, dispatcher, lightweight + heavy split, system-flagged via `templateIDRoutine` for tasks. The gaps are in the UI (incomplete edit form, no catch-up controls, no enable/disable toggle, no detail page) and the missing pre-install step.

The migration is mostly subtraction. Drop the agent-level cron and its schema; have onboarding install one regular routine; finish the routines UI.

## What

### Core model shift

| Today | After |
|---|---|
| Agent-level cron (`internal/scheduler/cron/agent_heartbeat.go`) ticks every minute, dispatches per agent | No agent-level cron. The existing routines cron tick (`RoutineService.TickScheduledTriggers`) is the single periodic-wake driver. |
| `agent_profiles.heartbeat_*` columns hold cadence + policy | Settings live on the routine row, editable in the UI. |
| Coordinators wake automatically without configuration | Onboarding pre-installs one regular routine ("Coordinator heartbeat"). User can edit / disable / delete it. |
| `wakeup_requests.source = "heartbeat"` is a distinct source | Source becomes `routine` for periodic wakes. The `heartbeat` source value is retired. |
| No routines edit UI / detail page | Full CRUD: list page (have it), create dialog (have most of it, missing fields), detail/edit page (missing), enabled toggle on the row. |

### What goes away

- `internal/scheduler/cron/agent_heartbeat.go` and its tests.
- `internal/office/repository/sqlite/heartbeat.go` (`ListDueHeartbeatAgents`, `AdvanceNextHeartbeatAt`, `SetAgentHeartbeatEnabled`).
- The cron handler's registration in `cmd/kandev/cron.go`.
- `agent_profiles.heartbeat_*` columns (six of them) plus the matching fields on `AgentProfile`. Idempotent `ALTER TABLE … DROP COLUMN` is awkward in SQLite; the cleanest path is the user's "recreate the DB" plan.
- `HeartbeatEnabled=true, HeartbeatIntervalSeconds=60, …` defaults in `createOnboardingAgent`.
- The `SourceHeartbeat = "heartbeat"` wakeup-source enum value — every existing call site that uses it (the agent-level cron) is going away. All other wakeup sources keep their values.
- The `wakeup.HeartbeatPayload` struct — replaced by `RoutinePayload` (which already carries `MissedTicks`).

### What stays

- `agent_wakeup_requests` table + repo + dispatcher. Comments / agent-error / user-mention / self-wake all still flow through it.
- `wakeup.RoutinePayload` and the routine source. The routines path is the survivor.
- `wakeup.Dispatcher` — its `RoutineLookup`-driven concurrency policy already does everything we need. The lookup reads the routine's policy at claim time; nothing changes here.
- `office.summary` package (continuation summary builder) and the AgentCompleted hook — fired on taskless runs regardless of whether the wakeup came from a system cron or a routine.
- The continuation-summary scope key. Today we hardcode `scope="heartbeat"` in `handleTasklessAgentCompleted`. With the migration the scope changes to `scope="routine:<routineID>"` for routine-fired runs (each routine gets its own summary chain). A routine is the closest analogue to a "thread of work that wakes up periodically".

### The pre-installed coordinator routine

At `createOnboardingAgent` time, when the new agent's role is CEO/coordinator, also create one routine:

```
name:                "Coordinator heartbeat"
description:         "Wakes the coordinator every 5 minutes to check workspace activity, react to errors and budget signals, and decide what to do next."
assignee_agent_id:   <new coordinator agent id>
status:              active
concurrency_policy:  coalesce_if_active
catch_up_policy:     enqueue_missed_with_cap
catch_up_max:        25
task_template:       ""    -- lightweight (taskless run per fire)
variables:           []
trigger:
  kind:              schedule
  cron_expression:   "*/5 * * * *"
  timezone:          (workspace TZ, fall back to UTC)
  enabled:           true
```

This is a **regular routine**. No `is_system` flag, no lock, no badge. The user sees it in the routines list, can change the cron to `* * * * *` if they want minute-level cadence, can disable it, can delete it. If they delete it, the coordinator only wakes via reactive sources (comments, agent errors, manual fires, user mentions). No magic.

**Default cadence is `*/5 * * * *` (every five minutes), not every minute** — the prior 60s default was too aggressive for the cost it imposed. Five minutes is a reasonable balance for an idle workspace; users can crank it up when they're actively driving the coordinator. (The catch-up cap of 25 means the worst case is "five hours of cron ticks were missed" before we start dropping fires.)

Subsequent coordinator agents created via API or UI (not just onboarding) get the same routine pre-installed. The check is on agent role at creation time.

### Routines UI gaps

The existing UI at `apps/web/app/office/routines/` has a list page and a create dialog with concurrency policy + trigger kind (`cron | webhook | api`) + cron expression. The dialog is missing a handful of fields, and there's no edit form — once a routine is created you can't change its cadence without going to the DB.

What needs to land in the routines UI for the migration to make sense:

1. **Detail / edit page** at `/office/routines/[id]`. Same fields as the create dialog, plus:
   - Status: `active` / `paused` / `archived` (radio).
   - Catch-up policy: `enqueue_missed_with_cap` / `skip_missed`. Catch-up max (number, default 25) when policy is `enqueue_missed_with_cap`.
   - Last fired at + last run id (link). Read-only, surfaced for at-a-glance health.
   - "Run now" button — fires manually via existing `FireManual` API, useful for testing.

2. **Create dialog gaps**: add catch-up policy + max fields. Match the detail page form. Remove the `triggerKind="api"` option until we wire API triggers (today only `cron` and `webhook` are useful; api is dead for v1).

3. **List page row**: add an enabled toggle (clicking it flips status between `active` and `paused`). Show the cron expression and "next fire" timestamp inline so users can see at a glance when the coordinator will next wake up.

4. **Empty state**: if a coordinator agent exists but has no enabled routines, show a small banner on the agent detail page: "This coordinator has no scheduled wake-ups. It will only fire on comments, errors, or manual triggers." Linkable to the routines page.

Webhook trigger CRUD is half-built today (UI shows the kind option; backend has the table). It can stay half-built for the migration — it's parallel work and not required for the heartbeat-as-routine story. Defer the webhook polish to a separate spec.

### Wakeup-source consolidation

Today the `agent_wakeup_requests.source` enum is:

```
heartbeat | comment | agent_error | routine | self | user
```

`heartbeat` is going away. After the migration the enum is:

```
routine | comment | agent_error | self | user
```

Three concrete sub-tasks:
- `wakeup/payloads.go`: drop `SourceHeartbeat` constant; drop `HeartbeatPayload` struct.
- `wakeup/dispatcher.go resolvePolicy`: drop the heartbeat-source switch arm; the routine arm covers it.
- Anywhere reason="heartbeat" is set on the underlying `runs` row, change to `reason="routine"` (the user-facing display already comes from the routine name, not the reason string).

Existing data: `recreate the DB` covers it. Production rollout (when there is a production) would need a one-shot UPDATE to flip rows from `heartbeat` → `routine` with a synthetic routine id, but we don't have that problem today.

## Out of scope

- Webhook trigger UI polish — separate spec.
- Plugin-managed routines (`pluginManagedResources` pattern). We don't have plugins yet; when we do, this can layer on.
- Routine revisions / rollback (`routineRevisions` table). Useful but not load-bearing for the migration.
- A "Run now" UI for manually firing any routine on demand. Out of scope unless trivial — the FireManual API already exists, so the button is small but the spec doesn't mandate it.
- Removing the `wakeup.HeartbeatPayload` JSON tag for `missed_ticks` — the field already lives on `RoutinePayload` (added when implementing the routine catch-up cap), so the payload shape carries over.

## Risks

- **Empty-coordinator regression.** Today every workspace gets a coordinator that wakes up. Under the migration, that's only true if the onboarding routine install succeeds. A failure mode that leaves a coordinator with no routine = a silent zombie. Mitigation: the onboarding routine installer logs + warns, and the coordinator's agent-detail UI surfaces the "no scheduled wake-ups" empty state described above. Worst case the user notices and can install a routine themselves.
- **Default cadence change.** Going from 60s to 5 minutes means coordinators react to events ~5x slower on average. This is a quality regression for tightly-monitored workspaces; mitigation is "the user can change the routine to `* * * * *` in two clicks".
- **Source enum drop.** Anywhere that filters `agent_wakeup_requests` by `source = "heartbeat"` for analytics / debug needs updating. Search for it before merge.
- **Catch-up math is now centralised in routines.** The agent_heartbeat catch-up logic was independent of the routines path. Centralising means one bug surface instead of two — net win.

## Implementation plan

Three PRs, each independently shippable. PR 1 is the destructive cleanup; PR 2 is the install path; PR 3 is the UI completion. They can be reviewed in order; the user-facing change is only complete after PR 3 lands.

### PR 1 — Drop the agent-level heartbeat cron

- Delete `internal/scheduler/cron/agent_heartbeat.go` and its test.
- Delete `internal/office/repository/sqlite/heartbeat.go` (three functions only used by the cron handler).
- Drop the cron handler registration from `cmd/kandev/cron.go`.
- Drop the six `heartbeat_*` columns from `agent_profiles` (idempotent ALTER TABLE block in `base_migrations.go`; SQLite doesn't support DROP COLUMN cleanly so the migration leaves them as dead columns until the next DB recreate, which is fine).
- Remove the corresponding `Heartbeat*` fields from `AgentProfile` in `agent/settings/models/models.go`.
- Remove `SourceHeartbeat` from `wakeup/payloads.go`; drop `HeartbeatPayload` struct.
- Drop the heartbeat-source arm from `wakeup.Dispatcher.resolvePolicy`.
- Delete the heartbeat default-on assignment in `createOnboardingAgent`.
- Tests: delete the heartbeat cron tests; the wakeup dispatcher's heartbeat-source test cases get removed; the heartbeat E2E smoke test in `wakeup/heartbeat_e2e_test.go` is renamed/repurposed for routine-fired wakes (PR 2 picks this up).

After PR 1 the build is green, but coordinators no longer wake on a system cron — they only wake via comments / agent-errors / manual / self / user. **PR 1 is intentionally a regression** until PR 2 lands.

### PR 2 — Pre-install the coordinator-heartbeat routine

- Add `RoutineService.CreateDefaultCoordinatorRoutine(ctx, workspaceID, agentID)` that materialises the routine + schedule trigger described above. Idempotent (skip if a routine with that fingerprint already exists for the agent).
- Hook it into `createOnboardingAgent` after the agent is created, gated on role = CEO/coordinator.
- Hook it into the agent-create API path (`internal/office/agents/handler.go` or wherever `CreateAgentInstance` is called from the UI) so subsequent coordinator agents also get one.
- Update the heartbeat E2E smoke test to drive a coordinator-routine fire end-to-end: enable the routine, advance synthetic time five minutes, assert a fresh taskless run appears with `runs.reason="routine"` and `payload.routine_id` populated.
- Continuation summary scope changes from `"heartbeat"` to `"routine:<routineID>"` in `handleTasklessAgentCompleted` for routine-source runs. The summary builder loads the prior summary keyed by routine id, so a coordinator with multiple routines (one per concern, e.g. "PR review" + "budget watch") gets a per-routine summary chain.

After PR 2 the system has the same end-to-end behaviour as before, just driven by a routine the user can see.

### PR 3 — Routines UI completion

- Detail / edit page at `/office/routines/[id]`. Same field surface as the create dialog plus catch-up policy + max, status, last-fired indicator, "Run now" button (calls existing FireManual API).
- Add catch-up policy + max to the create dialog.
- Drop the `api` trigger kind option (defer with the webhook polish).
- Add an enabled toggle to each row in the list. Show cron + next-fire-at inline.
- "No scheduled wake-ups" banner on the agent detail page when role = CEO/coordinator and the agent has no enabled routines pointed at it.

After PR 3 the user can configure everything previously hardcoded in `createOnboardingAgent` directly from the UI.

## Decisions to lock before coding

1. **Default cron**: `*/5 * * * *` (every five minutes) for the pre-installed coordinator routine. Deliberately less aggressive than today's 60s. Reasonable for an idle workspace; the user can crank it up.
2. **Routine name**: "Coordinator heartbeat" — descriptive, the user can rename it. We don't lock the name.
3. **No special "system" flag** on the pre-installed routine. Total user control: edit, disable, delete. If the user wants to recreate it after deletion, they re-run onboarding (or we add a "Reset to defaults" button later).
4. **Continuation summary scope** moves from `"heartbeat"` to `"routine:<routineID>"`. Each routine gets its own summary chain. A coordinator running three routines gets three independent summary buckets — matches the way the agent's actual context drifts apart per concern.
5. **What if the coordinator is reassigned to a routine the user installed manually?** The pre-installed routine is just a row; if the user reassigns, deletes, or replaces it with their own, that's fine. No reconciliation logic.
6. **Hard-deprecate or soft-deprecate the heartbeat enum value?** Soft (delete, no migration shim). The enum is internal — no published payload contract today. Touching every reference in one PR is cheaper than carrying two valid values.
