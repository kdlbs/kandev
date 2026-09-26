---
spec: docs/specs/office/requirements/scheduler.md
created: 2026-09-26
status: complete
---

# Implementation Plan: Office New Task Create Contract (assignee)

## Overview

The Office "New Task" dialog let a user pick an assignee, but the assignee was
never seated: the dialog sent it inside `metadata.assignee_agent_profile_id`
(a location nothing reads), and even had it been sent correctly the HTTP
create handler had no field to receive it. The task was created successfully
(2xx, success toast) with zero runner participants and zero
`task_assigned` runs — a silent drop, not a visible failure.

The service and repository layers already fully supported a real
`AssigneeAgentProfileID`: `insertTaskTx` unconditionally seats the runner via
`upsertRunnerInTx` inside the create transaction whenever the field and
`WorkflowStepID` are both set, and `assignee_agent_profile_id` on a fetched
task is a live computed projection from `workflow_step_participants`. The gap
was entirely upstream — the HTTP contract and the dialog payload — plus the
complete absence of create-time validation that the named profile is actually
a usable Office assignee.

This plan closes that gap: accept and validate `assignee_agent_profile_id` on
task creation (rejecting before any row is written), wire the dialog to send
it top-level, and remove the dead reviewer/approver "Stages" UI and its
`execution_policy` payload field, which had no reader and is superseded by
derived seat-casting on gated-step entry (REQ-OFFICE-REVIEW-SEATS-001/002).
Explicit reviewer/approver picks are deliberately out of scope here and are
tracked by a separate follow-up card.

This card is stacked on PR #3952 (`fix(office): don't launch agents when
assigning tasks on non-auto-start steps`, REQ-OFFICE-SCHEDULER-002): that PR
already gates every `task_assigned` wake — including the one this card's
fixed create path now correctly triggers — on the task's current workflow
step having an `auto_start_agent` on_enter action. This plan does not modify
that gate.

## Backend

### Create contract: accept the field

`apps/backend/internal/task/handlers/task_http_handlers.go`:

- `httpCreateTaskRequest` gains `AssigneeAgentProfileID string
  \`json:"assignee_agent_profile_id,omitempty"\``.
- `httpCreateTask` forwards it into the `service.CreateTaskRequest{...}`
  literal it builds for `s.svc.CreateTask`.

### Validation: reject before any row is written

`apps/backend/internal/task/service/workflow_agent_overrides.go`:

- New sentinel `ErrInvalidAssigneeAgentProfile = errors.New("invalid
  assignee_agent_profile_id")`. The message contains `"invalid"`, so the
  existing substring match in `internal/task/handlers/errors.go`'s
  `isValidationError` maps it to an HTTP 4xx with no changes needed there.
- New exported `(s *Service) ValidateAssigneeAgentProfile(ctx, workspaceID,
  assigneeAgentProfileID string) error`, decoupled from `CreateTaskRequest`:
  1. Empty `assigneeAgentProfileID` → `nil` (no assignee, no-op — the
     existing no-assignee create path is unaffected).
  2. Look up the profile via the already-injected `AgentProfileReader`
     (`s.agentProfiles`, wired at `Service` construction — no new
     dependency injection needed).
  3. Reject (wrapping `ErrInvalidAssigneeAgentProfile`) unless the profile
     exists, is not soft-deleted, and `WorkspaceID == workspaceID` with
     `WorkspaceID` non-empty. Deliberately does not gate on `Enabled`
     (Review round 1 finding R1-F1): `ListAgentInstances`, the picker the
     dialog's assignee list is drawn from, never filtered on it either, so
     pre-existing `enabled=0` Office agent rows would otherwise become
     unassignable after upgrade.

  The last check is deliberately **stricter** than
  `normalizeWorkflowAgentOverrideSource`'s existing rule elsewhere in the
  same file, which treats `WorkspaceID == ""` (global/kanban-legacy) as
  universally allowed. This mirrors `internal/office`'s own
  `ListAgentInstances` eligibility filter (`agentInstanceFilter`:
  `workspace_id != '' AND deleted_at IS NULL`, plus a workspace match) —
  the New Task dialog never offers a global profile as a choice, so this
  path holds to the stricter, Office-only rule instead of the override rule.

- New unexported `CreateTaskRequest.RequireAssigneeAgentProfileValidation
  bool` field (`json:"-"`, never accepted from a request body).
  `AssigneeAgentProfileID` is shared by trusted internal callers (agent-created
  subtasks, the onboarding adapter, routine-created tasks) that already trust
  their own value; gating the new check behind a flag only the HTTP handler
  sets keeps those callers unaffected instead of failing them closed when no
  `AgentProfileReader` is wired in their contexts.

`apps/backend/internal/task/handlers/task_http_handlers.go`:

- `httpCreateTask` sets `RequireAssigneeAgentProfileValidation: true` on the
  `service.CreateTaskRequest{...}` literal it builds — this is the only
  caller that sets it.

`apps/backend/internal/task/service/service_tasks.go`:

- `prepareTaskForCreation` calls `s.ValidateAssigneeAgentProfile(ctx,
  req.WorkspaceID, req.AssigneeAgentProfileID)` immediately after the
  existing `validateWorkflowAgentOverrides` call, before
  `prepareContributionDestination` and before any task row is built —
  but only when `req.RequireAssigneeAgentProfileValidation` is set.

No repository or transaction change: `insertTaskTx`
(`internal/task/repository/sqlite/task.go`) already calls
`upsertRunnerInTx(ctx, tx, r.db.Rebind, task.WorkflowStepID, task.ID,
task.AssigneeAgentProfileID)` unconditionally whenever both IDs are
non-empty. That write path was already correct; it just never received a
validated, non-empty value from the HTTP layer.

**Idempotent duplicate `external_id`:** the existing Found-outcome
short-circuit in `service_tasks.go` returns the already-created task before
`prepareTaskForCreation` runs again, so a duplicate request cannot re-trigger
this validation, write a second runner seat, or queue a second wake. This is
the existing idempotent-create contract; this plan does not change it.

## Frontend

### Dialog: send the field top-level, remove the dead Stages UI

`apps/web/app/office/components/new-task-dialog.tsx`:

- `handleCreate` sends `assignee_agent_profile_id: draft.assigneeId ||
  undefined` as a top-level field on the `createTask` call, not nested in
  `metadata`.
- `buildMetadata` no longer writes `assignee_agent_profile_id` or
  `execution_policy`.
- The `NewTaskStages` import, its `stages`/`updateStages` state, and its
  `<NewTaskStages .../>` render are removed. `buildExecutionPolicy` /
  `EMPTY_STAGES` are no longer referenced.

`apps/web/app/office/components/new-task-stages.tsx` is deleted — confirmed
via `rg new-task-stages` across `apps/web` that no other file imports it.

`apps/web/lib/api/domains/kanban-api.ts`: `createTask`'s payload type gains
`assignee_agent_profile_id?: string`.

### Locales

Six real catalogs (`en`, `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`, `ja`) plus the
generated `pseudo` catalog drop the keys that only the deleted Stages UI
used: `reviewStages`, `optional`, `reviewers`, `selectApprover`, `ship`,
`autoCommitAfterApproval`. `office:none`, `office:addReviewers`, and
`office:approver` are kept — they are still used by the separate,
pre-existing (and pre-existing-broken) reviewer/approver picker in
`new-task-participant-row.tsx`, which this card does not touch.

The existing error toast in `handleCreate`'s `catch` block already surfaces
`err.message` (the server's rejection message on an invalid assignee); no
change was needed there.

## Tests

- **Backend regression, TDD (red before the fix):**
  `apps/backend/internal/task/handlers/assignee_agent_profile_contract_test.go`,
  `TestBetaOfficeCreateContract`. Uses a real temp-file SQLite database (not
  a mock repository) because `assignee_agent_profile_id` is a computed
  projection over `workflow_step_participants`, not a stored column — only a
  real-SQLite-backed test can authentically prove the runner seat exists.
  Covers: valid Office assignee seats the runner; nonexistent profile
  rejected with no task row written; profile scoped to another workspace
  rejected; disabled profile rejected; global (no-workspace) profile
  rejected; duplicate `external_id` short-circuits without re-validating a
  second (invalid) assignee; no assignee is unaffected.
- **Frontend regression:**
  `apps/web/app/office/components/new-task-dialog-create-payload.test.tsx`.
  Renders `NewTaskDialog` with a pre-filled draft (via `defaultProjectId` /
  `defaultAssigneeId`, avoiding the need to drive the project/agent picker
  popovers), submits, and asserts the mocked `createTask` call received
  `assignee_agent_profile_id` as a top-level field and that `metadata`
  carries neither `assignee_agent_profile_id` nor `execution_policy`.
  Verified against the pre-fix code (temporarily reproducing the
  metadata-nesting bug in place) to confirm it fails for the right reason
  before confirming it passes against the fix.

## Not touching

- Anything in PR #3952's diff (`shared.IsAssignmentWakeEligible`, the
  reactivity/event-subscriber/recovery-sweep eligibility gate,
  `step_complete_kandev` response handling).
- Pause semantics (PR #3947).
- The seat-casting engine (`engine_adapters/seat_caster.go`,
  `office/service/review_stage.go`).
- Explicit reviewer/approver selection — tracked separately by follow-up
  task `1d4259a9-486f-40f4-b5ae-ba9acc8660d7`.

## Implementation Waves

Execution is sequential in the primary conversation; no subagents are
authorized.

Wave 1:

- [x] [Task 01: Create-time runner seat](task-01-create-runner-seat.md)

## Risks

- The stricter (workspace-required) eligibility rule for create-time
  assignees intentionally diverges from `normalizeWorkflowAgentOverrideSource`'s
  looser rule for workflow agent overrides elsewhere in the same file. A
  future refactor that consolidates these two rules must preserve the
  divergence or re-derive it deliberately, not assume they should match.
- `assignee_agent_profile_id` remains a computed projection, not a stored
  column: any future test of this contract must use a real SQLite-backed
  fixture, not a mock repository that would only echo back the struct field
  it was given.
