---
id: "01-move-approver-guard"
title: "Atomic human Office completion"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-HUMAN-DONE-001
  - REQ-OFFICE-HUMAN-DONE-002
acceptance_criteria:
  - AC-OFFICE-HUMAN-DONE-001.3
  - AC-OFFICE-HUMAN-DONE-001.4
  - AC-OFFICE-HUMAN-DONE-001.5
  - AC-OFFICE-HUMAN-DONE-002.1
  - AC-OFFICE-HUMAN-DONE-002.2
  - AC-OFFICE-HUMAN-DONE-002.3
  - AC-OFFICE-HUMAN-DONE-002.4
  - AC-OFFICE-HUMAN-DONE-002.5
system_design:
  - ../../specs/office/system-design/human-done-final-step.md
---

# Task 01: Atomic human Office completion

## Summary and checkpoint

Complete REQ-OFFICE-HUMAN-DONE-002 across genuine human moves, same-step state
completion, session approval and Office final-step PATCH. The parent accepted
the bounded HTTP409/copy implementation content, but has not authorized delivery. V6 design review is complete and a later explicit implementation
instruction now authorizes this work order, including the no-new-retry rule. The delivered version still has the preflight window; the local candidate
implements the selected boundary and is undergoing scoped validation.

## Selected design and necessary boundary change

Use the [selected transaction/workspace boundary](../../specs/office/system-design/human-done-final-step.md#selected-atomic-completion-design-reviewed-for-implementation).
That design supplies the intent, callback, canonical tx-reader,
scalar completer and human task-update APIs, every writer's acquisition order,
locator revalidation, policy fencing, name/error translation and test matrix.
Do not substitute a task-role lock, preflight-only guard or System actor.

The concrete obstruction is transfer's blocking SHARE ROW EXCLUSIVE reservation:
it permits prior row owners and can then wait on a row whose owner needs a
conflicting table-lock upgrade. Both table-before-hierarchy and workspace-first
SRE NOWAIT produced real PostgreSQL wait-graph cycles. The selected
additional change is transfer-only sorted source/destination workspace locks,
then complete EXCLUSIVE NOWAIT relation reservation, rollback/audit/existing
conflict on55P03, before any task-row access. Add currently omitted `tasks` and skipped `task_step_transitions` to the
reservation base without changing preservation counts. The [task transfer design](../../specs/tasks/system-design/cross-workspace-task-transfer.md#workspace-first-concurrency-boundary)
and AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.6 disclose this stronger reservation and
contention conflict. Ordinary human completion uses no table locks. Parent
accepted this tradeoff; keeping blocking SRE is not a safe fallback.

## Authorized implementation scope

1. Add transaction-bound intent and fresh actual membership/placement/policy
   checks at task write sites; preserve generic completion criteria and exact
   source/version/claim fences. Actual WIP-queued noncompletion is not refused
   based on speculative completed state.
2. Single-source destination/workflow canonical slate and active-decision
   reads through caller `*sql.Tx`; compose in backendapp with names only after
   rollback. Keep ID fallback and read-error refusal.
3. Put every listed seat/template/role decision, bulk supersession/clearing,
   marker-atomic clear and workflow-policy writer under workspace-first
   exclusion. Revalidate mutable task/by-ID scopes. Roll back/replan the whole
   set only through existing bounded retries that already repeat authorization.
   Otherwise roll back and return existing conflict; no new retry loops,
   authorization callbacks, foreign-scope following or late workspace locks.
4. Correct transfer reservation/order/conflict/census, preserving receipt,
   mapping, audit, idempotency and event contracts. No schema/new namespace.
5. Route final-step human PATCH through the scalar completer, generic human
   HTTP/WS state completion through the human wrapper, and session approval
   through its existing Human move. Human refusals use the established
   no-status rollback response; agent redirects stay unchanged.

## Explicit exclusions

No permission framework, generic transaction framework, post-write undo,
lifecycle/recovery redesign, Human-to-System bypass, new public request fields,
new controls or layout changes. Atomic implementation and causal concurrency tests are authorized by the later
explicit request; no new schema or generic framework is authorized. No commit, push,
public disposition, CI waiter or walkthrough rerun is authorized.

## Exact files and responsibilities

- `apps/backend/internal/task/models/completion_gates.go`: internal human
  completion intent/context, following existing completion override pattern.
- `apps/backend/internal/task/repository/repoerrors/`: typed pending-ID and
  internal scope-change errors; no new public wire vocabulary.
- `apps/backend/internal/task/repository/sqlite/task.go`,
  `office_completion.go`, `task_field_updates.go`, `task_hierarchy_admission.go`,
  `step_entry_dispatch.go` and
  `completion_gates.go`: locate/revalidate workspace scopes, fresh task/policy
  evaluation in shared writes, scalar Office human completion and reader
  configuration. Keep exact/source fences and ordinary hierarchy order.
- `apps/backend/internal/task/service/service_workflow.go`,
  `service_tasks.go`, `service_events.go`, `office_approval_guard.go`: bound intent at genuine human
  entry points, configured atomic versus legacy test behavior, scalar method
  and after-rollback ID/name error translation. ApproveSession stays Human.
- `apps/backend/internal/backendapp/adapters_office_completion.go`,
  `adapters_office_final_step.go`, `main.go`: compose canonical tx reader and
  name resolver; bind actual-final-step intent and scalar completer; assert
  production wiring before serving requests.
- `apps/backend/internal/office/repository/sqlite/participants.go`: canonical
  tx slate reader; `participant_completion_scope.go` owns the workspace-first
  Add/Remove/claim writer prefixes.
- `apps/backend/internal/workflow/repository/phase2_sqlite.go`, `sqlite.go`:
  `office_completion_scope.go` supplies canonical scope/locator exclusion;
  canonical tx decision reader; participant/template/decision/bulk writers
  and step create/update/reorder/delete policy writers in the selected order.
- `apps/backend/internal/task/repository/sqlite/step_entries.go`: workspace
  prefix in ClearStepDecisionsAndCompleteMarker without splitting its commit.
- `apps/backend/internal/task/repository/sqlite/task_transfer.go`,
  `task_transfer_relations.go`: workspace-before-reservation, exact source
  revalidation, EXCLUSIVE NOWAIT and complete reservation census; stable
  conflict/audit after rollback. Review `task_transfer_pending.go` writes,
  `task_transfer_projection_catalog.go` and `step_transitions.go` as consumers.
- `apps/backend/internal/task/repository/sqlite/workflow.go`, `workspace.go`
  and workspace-field update paths: verify deletion/membership row locking
  participates; avoid late workspace locking during cascades.
- `apps/backend/internal/office/dashboard/human_done_final_step.go`,
  `service_tasks.go`, `decisions.go`: scalar completer seam, unchanged agent
  route, one post-success activity/event sequence and pure pending/name helpers.
- `apps/backend/internal/task/handlers/task_http_handlers.go`,
  `task_ws_handlers.go`, `errors.go`: human update wrapper and existing
  HTTP409/WS conflict mappings for field completion as well as move/approval.
- Adjacent service/repository/handler Go tests, existing
  `board-done-final-step.spec.ts` and `mobile-board-done-final-step.spec.ts`:
  deterministic route and concurrency evidence described below. Verify
  `office-status-gate.ts`, board drag, optimistic mutation, status picker and
  session-approve consumers; change no TS production unless causal evidence
  shows the established refusal shape is insufficient.
- `docs/public/automation-and-mcp.md`: after implementation, explain the busy
  relation conflict and manual retry without promising automatic recovery.
  Preserve rejected-key semantics:55P03 is audited as conflict, not failed;
  review `task_transfer_audit.go` and its existing denied/failed key predicate.
  No public behavior claim is added before implementation.
- Owning Office requirements/design/plan and linked task transfer
  requirements/design remain the only authoritative contract sources.

## UI-01: existing refusal presentation on desktop and phone

The [plan's UI-01](plan.md#ui-01-board-done-drop-on-a-work-step-task-ac-office-human-done-0011-3)
also applies when a human explicitly sets Done on the final step. No new
control or layout is proposed; the existing localized pending message remains.

```text
Before, final-step human pending approval:
  desktop card / phone status picker: Todo -> In review
After, atomic refusal:
  desktop card / phone status picker: Todo -> Todo
  message: Approval is pending: <existing localized approver names>
```

The atomic state does not show an In Review redirect or wake an approver.
Native phone press-and-hold and picker capability, scrolling and desktop drag
remain. Copy/44px/layout work is not added; existing coverage limits stay
explicit. A scalar successful Done retains existing success side effects once.

## Review regression closure

Keep ordinary title/description/metadata edits on fresh same-workspace placement,
with the authorized workspace fence retained. Explicit state/step requests keep
completion placement fences. End human intent at committed entry dispatch and
service postcommit callbacks/queue reconciliation. Initially missing Office task
registration returns unchanged before role locking; later calls locate scope
normally. Prove registered HTTP/WS content success and foreign-scope refusal,
real engine decision-clear/markers/later action on SQLite and PostgreSQL, event
context isolation, and actual PostgreSQL missing-to-created role-lock ordering.

## Implementation acceptance

- Each human route fails a real causal RED before production changes, then
  GREEN orders eligibility and completion atomically. Refusal leaves complete
  task/session/decision/transition/marker/run/event state unchanged; a same
  fixture with no pending approver succeeds. Include actual queued admission.
- Every selected writer and transfer ordering has deterministic SQLite and
  real PostgreSQL two-connection evidence, including opposite transfers,
  prior/new row owners, by-ID/template scope changes, full census, policy
  flips,55P03/cancellation rollback and authorization/source retries. A skip
  is unverified, not a pass.
- Existing engine/agent/nonOffice, optional nil test wiring, criteria/exact
  fences and names/error semantics remain; actual desktop and native phone
  completion/refusal/queued flows preserve existing localized outcomes.

## Verification commands

Run sequentially through the explicit normal shared guard. Go uses
GOMAXPROCS=2, GOMEMLIMIT=3GiB and GOFLAGS=-p=1. Use the real repository's
existing PostgreSQL fixture DSN contract with an owned disposable database;
never a developer/live DB. The product test families below exercise the real composition. PostgreSQL
requires an owned disposable DSN; skips do not count as dialect evidence.

```bash
cd apps/backend
# RED first, then GREEN after approved implementation; include both dialect cases.
go test -race -count=1 -run 'TestOfficeHumanCompletion|TestOfficeMutationScope|TestTransferWorkspaceFirst|TestTransferRelationReservation' ./internal/backendapp ./internal/task/repository/sqlite ./internal/task/service ./internal/task/handlers ./internal/office/repository/sqlite ./internal/office/dashboard ./internal/workflow/repository
# Then run the affected existing seat/decision, hierarchy, exact/fence and transfer tests.
cd ../..
bash scripts/lint-go-changed <exact-changed-go-files>
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
cd apps/web
pnpm exec vitest run --maxWorkers=1 lib/api/domains/office-status-gate.test.ts lib/api/domains/office-task-normalize.test.ts
pnpm run i18n:check
pnpm run i18n:ratchet
pnpm e2e:run --host --shards 1 --project chromium tests/office/board-done-final-step.spec.ts
pnpm e2e:run --host --shards 1 --no-build --project mobile-chrome tests/office/mobile-board-done-final-step.spec.ts
```

## Current results and limits

Bounded HTTP/copy implementation content is accepted, not delivered. Atomic
implementation and scoped verification are complete locally and uncommitted.
Independent actual-diff acceptance and later delivery authorization remain pending.
Product causal RED/GREEN covers stale
preflight writes, scalar and registered human routes, fresh Office membership,
workflow policy, authorization scope and queued admission. Real PostgreSQL
checks cover canonical readers against 18 writers, prior/new row owners,
complete transfer reservation census, cancellation, opposite scopes and
READ COMMITTED revalidation. Canonical reader failures are closed on both
dialects, with no task/history/event writes and a same-fixture success control.
Registered-route refusals preserve task/session/history/marker/decision/activity
and run tables; same-fixture successes reach event publication.

Affected-package and final combined-tree race checks, changed-Go lint, catalog
validation, spec-linter tests and all-spec lint passed. The actual-main
`8ba5b6909139ec2e9e14b0e2eb2f96a4e342e88b` combined tree passed 20 focused API
tests, i18n checks/ratchet, three desktop and five native Pixel-5 phone E2Es
(success, refusal and queued behavior, including phone touch drag and picker).
The managed desktop run rebuilt all product assets; phone reused those fresh
assets sequentially. Initial Corepack/pnpm-version setup failure ran no browser
tests; the retry used existing pinned pnpm9.15.9 without a version-check bypass
or installation change. Exact commands, original failures and source/main
bases are retained in the contributor review evidence packet.

The earlier V6 SQL-shape probe and setup failure remain historical evidence,
not product proof. Its network-none container differs from the later product
runner's loopback-published disposable PostgreSQL container. Neither used a
live/developer database. No public disposition, commit, push or CI rerun is
included. Broad unchanged typecheck/UI suites and a full large-file audit are
not claimed.
