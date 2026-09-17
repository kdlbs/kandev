---
id: "06-attention"
title: "Attention reconciliation and targeted wakeups"
status: done
wave: 3
depends_on: ["01-durable-intake","02-objectives-routing"]
plan: "plan.md"
requirements:
  - REQ-ORCHESTRATION-ASSISTANT-005
acceptance_criteria:
  - AC-ORCHESTRATION-ASSISTANT-005.1
  - AC-ORCHESTRATION-ASSISTANT-005.2
  - AC-ORCHESTRATION-ASSISTANT-005.3
  - AC-ORCHESTRATION-ASSISTANT-005.4
system_design:
  - ../../specs/orchestration/system-design/personal-assistant.md
---

# Task 06: Attention reconciliation and targeted wakeups

## Inputs

Read the [requirements](../../specs/orchestration/requirements/personal-assistant.md) and [design](../../specs/orchestration/system-design/personal-assistant.md); legacy scenarios S09, S10, S13, S19, and [plan](plan.md), Backend 5. Read applicable AGENTS.md and implementation skills before editing. The [baseline experiments](experiments.md) are continuation evidence, not completed implementation.

## Acceptance

1. Canonical questions, permissions, auth failures, reviews and results across all linked sessions produce correct attention without a task-board movement.
2. Duplicate/out-of-order/restart events converge on one current record; idle-with-no-request and unchanged scans spend no model turns.
3. Paused/disabled assistants do not dispatch; bounded periodic recovery repairs missed managed-work events without replaying actions.

## Likely files

- apps/backend/internal/orchestration/models/attention.go (new)
- apps/backend/internal/orchestration/repository/sqlite/attention.go (new)
- apps/backend/internal/orchestration/runtime/events.go, recovery.go; attention.go, attention_test.go (new)
- apps/backend/internal/backendapp/adapters_assistant_attention.go (new)
- apps/backend/internal/task/statussummary; internal/events/types.go; canonical pending-input event producers (reuse/extend bounded identities)

## Implementation sequence

Implement a deterministic attention projector first using typed source snapshots. Adapt pending input and all relevant sessions rather than selecting only the latest. Subscribe to TaskSessionStateChanged, BuildPermissionRequestWildcardSubject and source-resolution events. Add durable last-notified revision and outbox enqueue; use injected clock for periodic reconciliation tests. Keep source request IDs and retry budgets bounded.

## Verification

Run each parenthesized command from the repository root. Use the repository Go/Node/pnpm toolchains. Scoped Go tests are intentional: the available make test target runs the entire backend. New test filters must select the named new tests; a no-tests-to-run result does not satisfy acceptance.

```sh
(cd apps/backend && go test -tags fts5 -count=1 ./internal/orchestration/... ./internal/backendapp -run 'TestAssistant(Attention|Wake|Reconcile)|TestConversationRunsWithoutOffice|TestRestartDoesNotReplayAnInterruptedConversation')
(cd apps/backend && go test -count=1 ./internal/task/statussummary)
```

## Dependencies and risks

Dependencies: `01-durable-intake`, `02-objectives-routing`. Execute in the primary session unless the user explicitly authorizes subagents.

Task WAITING_FOR_INPUT and session WAITING_FOR_INPUT are different; neither proves an unresolved question. Source messages can expire and all worker sessions matter.

## Output

Event-driven, deduplicated attention with recovery and no-op semantics.

## Detailed implementation checklist

1. Define attention DTOs/states and an Orchestration-owned store keyed by binding,
   workspace, task, session, native request/source identity and revision. Keep
   pending, resolved, expired, unknown and inactive distinct; store bounded safe
   summaries/references rather than full task transcripts or tool arguments.
2. Implement a core `AttentionReader` adapter that inspects every relevant managed
   task session, canonical questions/permissions/authentication blockers and active
   errors. A newer active session must not hide an older still-pending request.
   Recheck ownership/access before projecting any source.
3. Subscribe to existing lifecycle/input/resolution/permission/integration events.
   Add bounded source IDs to missing producers where needed. Task summaries are
   dirty signals; read the canonical source to establish actual request identity.
4. Add bounded startup and 60-second reconciliation over managed links only. Use
   event-driven local refresh for the five-second healthy-instance target. An
   unchanged scan performs no model turn. Use an injectable clock for timing tests.
5. Commit attention revision and wake outbox atomically. Use stable occurrence
   identity and last-notified revision; retry through existing run/idempotency
   infrastructure. Test crash before/after enqueue and acknowledgement, duplicate
   events, reordering, restart and partial source failure. Do not copy plugin
   process-local locks or dispatch-then-state persistence as durable guarantees.
6. Coalesce redundant task status noise while retaining separate questions/sessions.
   Paused/disabled assistants preserve records but launch nothing. Re-enabling
   first revalidates current sources so expired requests cannot be revived.
7. Extend required-store conformance and upgrade coverage for any attention/outbox
   tables. Expose authorized paginated reads plus a small revision-only frontend
   invalidation event; polling never receives hidden conversation data.

## Detailed evidence map

| Criterion | Planned test | Required edge cases |
| --- | --- | --- |
| AC-ORCHESTRATION-ASSISTANT-005.1 | `TestAssistantAttentionAllSessions` | Older pending/newer running, permission/question/auth/error, foreign task |
| AC-ORCHESTRATION-ASSISTANT-005.2 | `TestAssistantAttentionRestartDedup` | Duplicate/reordered events, outbox crash points, unknown enqueue receipt |
| AC-ORCHESTRATION-ASSISTANT-005.3 | `TestAssistantAttentionLatencyAndNoop` | Fake-clock 5s event/60s repair, bounded paging, zero model calls on unchanged state |
| AC-ORCHESTRATION-ASSISTANT-005.4 | `TestAssistantAttentionPausedAndExpired` | Off/paused, expiry, missing provider handle, re-enable without resurrection |

Use repository transaction tests, runtime reconciler tests and backend adapter
fixtures. Run race tests plus SQL guard and both-engine fresh/replay conformance
with a disposable PostgreSQL DSN after the targeted work-order commands.

## Scope boundaries and delivery

Central Coordinator task grouping remains a lightweight canonical projection;
this richer attention ledger belongs to the assistant. Native resolution is task
07. No new scheduler or automatic permission policy is introduced here.

## Parallelism

`sequential`

## Results

Implemented a native attention reader over every managed task session, the
canonical interaction service, live permission handles, live clarification
requests and native task/session errors and results. Storage retains bounded,
redacted summaries and source references. Waiting without a native request
produces no question; a newer running session cannot hide an older question.
Unavailable sources produce unknown cards; lost handles expire.

Projection and native-occurrence wake outbox commit atomically. Each occurrence
has a stable queue identity, including across lost acknowledgements and restart.
Repeated task status noise and unchanged scans produce no model turn. The
existing scheduler repairs missed events every 60 seconds in 100-link batches;
native lifecycle/message/permission/resolution events refresh immediately.
Private legacy callbacks are replaced by this current-source path. Queued wakes
recheck scope and current source before launch. Pause/disable preserves records
without dispatch; expiry before resume prevents resurrection.

Added owner/runtime pages with bounded scope-bound cursors, per-task visibility
rechecks, the named broker read and an owner-only revision invalidation event.
An integration check found that an empty objective could still select the legacy
coordinator dispatch contract. Private broker delivery now always requires an
objective and current context, so new managed work has a durable link.

Observed red/green: attention APIs/models were absent; the untracked-delivery
probe returned 201 before the fix and now returns 422 without an external effect.
All fixtures and request text are synthetic.

Verification in local `assistant-attention-*` logs:

- Exact work-order filter: **11 top-level tests** (nine runtime, two native
  adapters). Source-less waiting, older/newer sessions, expiry, pause, queue
  acknowledgement loss, event repair, missing sources and removed links pass.
- Full Orchestration and CLI packages pass with the race detector. Native
  adapter and owner-only WebSocket tests also pass with the race detector.
- Both-engine attention CRUD/no-op/wake/replay tests pass. Full required-store
  conformance and v0.93 previous-stable upgrade pass on SQLite and PostgreSQL.
- Native task status-summary tests, SQL guard and architecture lint pass.
  Public-doc validation passes 61 tests and 48 pages.

No provider turn, live service change, public post or real prompt fixture was
used. Native resolution and stop controls continue in task 07.
