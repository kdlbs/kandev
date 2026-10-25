---
spec: docs/specs/office/requirements/automation-continuity.md
decision: docs/decisions/2026-08-22-user-configured-automation-continuity.md
created: 2026-08-22
status: completed
---

# Implementation Plan: Automation Continuation and Coordinator Access

## Overview

Add one explicit automation setting: create an isolated task for each firing or continue one task,
primary session, task environment, and worktree. Every firing remains a distinct `AutomationRun`
bound to an exact turn. Automation sessions receive a fixed, workspace-scoped coordinator MCP
surface whose trusted principal is resolved once before handler dispatch.

Agent-side compaction remains responsible for live provider context. Kandev bounds only its
non-native fallback resume prompt to the newest 50 user or assistant text messages. Tool calls and
tool results do not appear and do not consume the limit. The design also
closes the reviewed lifecycle gaps: self-targeting, stale open runs, automation deletion, run-title
snapshots, registration decomposition, and shared-worktree concurrency.

## Backend

### Persistence and run contracts

Extend `apps/backend/internal/automation/models.go` and `store.go` with
`continuation_policy`, `continuation_task_id`, exact `session_id`/`turn_id` bindings,
`thread_action`, `thread_reason`, and `display_title`. Reuse the existing `triggered` status for an
admitted but unbound run. The shared open-run predicate counts both `triggered` and `task_created`;
legacy unbound `triggered` rows reconcile to failed instead of remaining open.

Render `display_title` before admission. A newly created or replacement task receives that title;
a resumed shared task is not renamed. YAML exports only `continuation_policy` and excludes runtime
pointers, bindings, titles, and the fixed MCP profile.

### Reusable dispatch and recovery

Update `apps/backend/internal/automation/service.go` and
`apps/backend/internal/orchestrator/event_handlers_automation.go` so admission writes the run before
event publication and the event carries the run ID. Dispatch returns the exact
`{task_id, session_id, turn_id}` and records `created`, `resumed`, or `replaced`.

Serialize admission and continuation binding per automation. Reconcile an open bound run to failed
when its exact turn has no live execution, open turn, or pending blocker. Add a backend action that
stops the exact run/session/turn, marks only that run failed, and releases its slot. A genuinely
live or blocked turn remains open until it completes or a person stops it.

### Bounded fallback resume history

Change `apps/backend/internal/agent/runtime/lifecycle/session_history.go` so
`GenerateResumeContext` selects the newest 50 non-empty `user_message` and `agent_message` entries.
It excludes tool calls, tool results, status events, and unknown types before selection. It restores
message order and applies the current per-message truncation. The new prompt stays outside the
window. Native session continuation and agent compaction remain unchanged. Add boundary tests for
49, 50, and 51+ messages, heavy intervening tool activity, ordering, and unchanged history files.

### Automation MCP principal and tool catalog

Add `SurfaceAutomation` to `apps/backend/internal/mcp/profile/profile.go`. Decompose
`registerKanbanTools` and configuration registration into small reusable groups so the automation
catalog can include the named tools without `delete_task_kandev`, configuration mutation,
task-local authoring, provider PR/MR, diagnostics, or plugin tools. Pin all base-surface catalogs to
prevent regressions.

Extend `apps/backend/internal/mcp/scope` and the lifecycle stream wrapper to resolve one trusted
automation principal before MCP dispatch. It carries automation/workspace/caller task/caller
session/surface identity and is the single workspace boundary used by the action dispatcher and
handlers. Owner identity remains necessary but is not sufficient for automation calls.

Reject the automation's own hidden task and every session on it for mutations, messaging, stopping,
spawning, and blocker discovery or resolution. Cross-task spawning uses the target task's normal
profile; it never inherits `SurfaceAutomation` from the caller. Permission audit derives the
automation actor and `automation_mcp` source from the trusted principal.

### Shared-run lifecycle and cleanup

Project summaries by exact turn and preserve the legacy task-based fallback only for rows without a
binding. Retention counts distinct task IDs and protects the current continuation. Run deletion and
delete-all remain reference-aware.

Replace row-only automation deletion with service-owned lifecycle cleanup: prevent new admission,
capture distinct referenced task IDs plus `continuation_task_id`, stop live sessions, delete the
automation/run records, and pass each now-unreferenced hidden task through normal task/worktree
cleanup. Insert one durable `automation_task_cleanup_jobs` row per distinct task in the deletion
transaction; delete the job only after task/worktree cleanup succeeds. Startup and normal orphan
reconciliation retry remaining jobs. The workspace reset path reuses the same ownership logic.

Kandev does not reset or rebase a reused checkout. Replacement creates a fresh task environment
from the repository's current configured base branch.

## Frontend

### Automation editor

The create and edit forms add a visible Context between runs radio group. `new_task` is selected by
default; `reuse_thread` locks concurrency at 1. The section, both options, and the concurrency lock
use the exact visible descriptions from the spec. Descriptions are linked to the applicable heading
or radio control and do not require hover or focus. There is no MCP capability UI.

### Automation detail and run transcript

Thread `display_title`, exact turn identity, and the `triggered` Running state through API types and
run components. Selecting two runs that share a session still focuses different turns. Both
`triggered` and `task_created` appear in the Running group/filter.

Expose Stop current run for a selected open run. It calls the exact-run cancellation action and
keeps its busy/error state local to that run.

## Mobile design contract

- Desktop outcome: the selected open run has a visible Stop current run action beside its live
  status. Mobile entry point: the same action appears in the selected-run drawer/content header.
- Nearest exemplar: the existing automation detail mobile drawer for run selection; retain its
  hierarchy and use its selected-run content header for the action.
- Presentation: keep run navigation in the existing inset bottom drawer and transcript as the
  primary surface. Do not introduce a second confirmation drawer; use the existing destructive
  confirmation primitive if cancellation requires confirmation.
- Scrolling and touch: the detail page remains the single main scroll owner, drawer content keeps
  its internal scroll, the action is at least 44 px, and document horizontal overflow remains zero.
- Shared logic: desktop and mobile use the same selected-run identity, cancellation mutation, busy
  state, and error mapping.
- Mobile proof: open a running reusable automation, stop its exact run, observe Failed and an
  enabled next firing, and assert viewport containment and no horizontal overflow.

## Tests

- Automation store/service tests: policy defaults, `triggered` admission, exact bindings,
  `display_title`, open predicates, reconciliation, deletion ownership, and export exclusions.
- Orchestrator tests: first creation, resume, fallback replacement, exact-turn completion, stale
  bound-run repair, exact-run stop, and no shared-task rename.
- Lifecycle tests: newest-50 user/assistant messages, exclusion of tool events, chronological order,
  message truncation, current-request placement, and source-history preservation.
- MCP profile/server/handler tests: exact catalogs, split-registration regressions, principal
  derivation, same/foreign/self target matrices, non-forgeable audit, and target-profile spawning.
- Frontend tests: create/edit policy payloads, visible and accessible descriptions, concurrency
  lock, run-title rendering, `triggered` filtering, shared-session turn selection, and exact-run
  stop terminal paths.

## E2E tests

- Desktop: read both continuity descriptions, create a reusable automation, fire it twice, verify
  one task/session with distinct turns and titles, then stop an open exact run.
- Mobile: read both continuity descriptions, create/edit continuity, select a shared-session run,
  stop it through the visible touch action, and verify no horizontal overflow.
- Backend integration: fixed catalog, self-target denial, foreign-workspace denial, target-profile
  spawning, stale-run reconciliation, and automation deletion cleanup.

## Public documentation

Implementation updates `docs/public/automation-and-mcp.md`,
`docs/public/agents-and-profiles.md`, `docs/public/websocket-api.md`, and
`docs/public/feature-status.md`. Document continuity choices, agent-managed compaction, the
newest-50 message bound, tool-event exclusion, workspace/self boundaries, exact-run recovery, fixed
MCP authority, and non-destructive reused-worktree behavior.

## Implementation waves and parallel candidates

Wave 1 (parallel candidates; user authorization required):

- [x] [Task 01: Persist continuation and run contracts](task-01-persist-continuation-policy.md)
- [x] [Task 03: Bound fallback resume history](task-03-bound-fallback-resume-history.md)

Wave 2 (parallel candidates after Task 01; user authorization required):

- [x] [Task 02: Dispatch reusable automation turns](task-02-dispatch-reusable-turns.md)
- [x] [Task 04: Define automation MCP surface](task-04-define-automation-mcp-surface.md)
- [x] [Task 06: Add continuation control](task-06-add-continuation-control.md)

Wave 3:

- [x] [Task 05: Maintain shared run lifecycle](task-05-maintain-shared-run-lifecycle.md)
- [x] [Task 07: Anchor and control shared runs](task-07-anchor-and-control-shared-runs.md)

Wave 4:

- [x] [Task 08: Validate coordinator automations](task-08-prove-and-document-coordinators.md)

The primary conversation executes sequentially unless the user explicitly authorizes subagents.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/automation ./internal/orchestrator
cd apps/backend && go test ./internal/agent/runtime/lifecycle
cd apps/backend && go test -tags fts5 ./internal/mcp/profile ./internal/mcp/scope ./internal/mcp/server ./internal/mcp/handlers
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- --run components/automations components/runs lib/api/domains/automation-runs-api.test.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm run i18n:ratchet
cd apps/web && pnpm e2e:run tests/automations-settings.spec.ts tests/automations-run-detail.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome --no-build tests/mobile-automations-scroll.spec.ts tests/mobile-automation-detail.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Risks

- A completion or stop action without exact run/turn identity can settle the wrong shared run.
- A missing dispatch-level principal can fall back to owner-wide workspace reach.
- Allowing self-target mutations can bypass permission authority or create concurrent sessions in
  the reused checkout.
- Fallback history filtering in the wrong order can return the oldest messages or let tool events
  displace conversation from the 50-message window.
- Deleting database references before capturing task ownership can orphan hidden worktrees.
- Automatically rewriting a reused checkout can destroy work that continuity promises to retain.

## Out of scope

- Concurrent scheduled turns or additional sessions on the reusable automation task.
- Age/turn-based rotation of a healthy compacting agent session.
- Per-automation MCP capability settings, arbitrary tool allowlists, or the full external surface.
- Cross-workspace coordinator authority or self-approval.
- Automatic reset/rebase of reused worktrees.
- Automation-specific transcript pruning; transcript retention remains a product-wide task policy.
- Reconstructing live provider permission prompts after runtime loss.
- Making `AutomationRun` the direct owner of sessions or worktrees.

## Verification results

- Backend: automation/orchestrator 2,452 tests; lifecycle 1,947 tests; MCP/profile/scope/server/handlers 877 tests; backendapp 466 tests.
- Frontend: focused automation/run/API tests 308 tests; transcript/detail tests 32 tests; typecheck passed.
- Internationalization and public docs checks passed, including the new-code ratchet.
- Desktop E2E passed 24 tests; mobile E2E passed 10 tests.
- `pnpm install --frozen-lockfile`, `git diff --check`, and public-doc validation passed.
