---
id: "16-host-step-approval-rpcs"
title: "Host: workflow step approval (ApproveStep, GetStepApproval)"
status: done
wave: 4
depends_on: ["01-host-control-rpcs-adr", "15-host-delegated-human-responses"]
plan: "plan.md"
requirements:
  - REQ-PLUGINS-CHAT-BRIDGE-HOST-006
acceptance_criteria:
  - AC-PLUGINS-CHAT-BRIDGE-HOST-006.1
  - AC-PLUGINS-CHAT-BRIDGE-HOST-006.2
  - AC-PLUGINS-CHAT-BRIDGE-HOST-006.3
  - AC-PLUGINS-CHAT-BRIDGE-HOST-006.4
  - AC-PLUGINS-CHAT-BRIDGE-HOST-006.5
  - AC-PLUGINS-CHAT-BRIDGE-HOST-006.6
system_design:
  - ../../specs/plugins/system-design/chat-bridge-host-controls.md
---

# Task 16: Workflow step approval RPCs

Repository: `kdlbs/kandev`. Use `/tdd`. This delivers requester decision 2:
`/approve` also approves workflow review steps.

- **Acceptance:**
  1. **`ApproveStep`.**
     - It is an exact command with request ID, workspace, task, session,
       observed session version, idempotency key, approval revision, manifest
       digest, and a human-response receipt from task-15 (receipt kind
       `step_approval`).
     - It delegates to `task/service.Service.ApproveSession`, the path behind
       `POST /api/v1/sessions/:id/approve` and the UI Approve button, so it
       fires `MoveTask` → `task.moved`/`on_enter` and sets
       `review_status=approved`.
     - It returns `{from_step_id, to_step_id}`. A final step approves without
       moving.
  2. **`GetStepApproval(session_id)`.**
     - Gated on `api_read: sessions`. It returns `{awaiting, from/to step id
       and name}`.
     - `awaiting` is true only when all of the following hold:
       - the session is not `RUNNING`/`STARTING`;
       - `review_status != approved`;
       - either `review_status == pending`, or the current step has an
         `on_turn_complete` transition with `requires_approval=true` and a
         turn completed in this step.
     - The target uses `resolveApprovalNextStep`, extracted into a shared
       helper and not duplicated.
  3. **Error cases.**
     - `ApproveStep` while not awaiting → `FailedPrecondition`.
     - A repeated approval → `ALREADY_APPLIED`; the task does not move twice.
     - An office approval stage (`StageTypeApproval`) → `Unimplemented`.
- **Verification:**
  - Regenerate protos.
  - `cd apps/backend && go test ./internal/task/service/... ./internal/plugins/... ./internal/backendapp/...`
  - `cd pluginsdk && go test ./...`
  - `make -C apps/backend lint`
- **Files likely touched:** `apps/backend/internal/task/service/service_workflow.go` (+ test), `pluginsdk/proto/kandev/plugin/v1/plugin.proto` (+ generated), `pluginsdk/exact_*.go`, `apps/backend/internal/plugins/host_exact*.go`, `apps/backend/internal/backendapp/adapters_plugin_*.go`.
- **Dependencies:** task-01, task-15 (receipt kind).
- **Parallelism:** sequential (proto).
- **Inputs:**
  - `ApproveSession` and `resolveApprovalNextStep` in `service_workflow.go`
    (`origin/main` 018808f8).
  - The handling of approval-gated transitions in
    `apps/backend/internal/orchestrator/event_handlers_workflow.go`
    (`processTurnCompleteActions`).
  - The UI rule in `apps/web/hooks/use-session-layout-state.ts`
    (`showApproveButton`).
- **Risks:**
  - "A turn completed in this step" may need step-entry time. If no such field
    exists, use `ListTaskStepTransitions` and record the choice in the ADR.
  - `ApproveSession` calls `authorizeTaskID`. Confirm how plugin calls are
    scoped and pass the verified context; never synthesize a caller.
- **Output contract:** summary, files changed, tests run, blockers, risks, and task/plan status update.

## Results
`HostExecutionCommands(host)` gains `GetStepApproval` (exact read,
`host.v2.read:sessions`) and `ApproveStep` (exact command,
`host.v2.write:execution`, `step_approval` receipt from task-15). The task
service owns the rule (`GetStepApprovalStatus`, `ApproveAwaitingStep` in
`apps/backend/internal/task/service/step_approval.go`), reusing
`resolveApprovalNextStep` and `ApproveSession` unchanged.

- **"Turn completed in this step":** turns already carry the step and the
  step-transition entry they started in
  (`workflow_step_id_at_start`, `workflow_step_transition_id_at_start`). The
  latest non-lifecycle turn must be completed and stamped with the current
  step and the task's newest transition ID, so an earlier visit to the same
  step does not count. The ADR records this choice.
- **Fields:** the command takes the session (the task is derived), the
  resource version from `GetStepApproval`, and no management-claim fence:
  like an interaction response it is a human decision.
- **Error mapping:** following the exact-command convention, not awaiting is
  `CONFLICT` `step_not_awaiting_approval` (the command form of
  `FailedPrecondition`), a changed target is `CONFLICT`
  `step_approval_changed`, and an Office approval stage is `Unimplemented` on
  the read and `UNSUPPORTED` on the command. A replay is `ALREADY_APPLIED`
  and returns the recorded from/to steps without moving again. The approval
  runs as the linked Kandev user, so `ApproveSession`'s `authorizeTaskID`
  checks that user.
- **Files:** `task/service/step_approval.go` (+ test),
  `plugins/host_step_approval.go`, `plugins/host_exact.go`,
  `plugins/exact_execution_contract.go`, `plugins/human_responses.go`
  (step-approval receipts), `backendapp/adapters_plugin_step_approval.go`
  (+ test), `backendapp/main.go`, `pluginsdk/exact_execution.go`,
  `pluginsdk/step_approval_grpc.go`, classification, proto (+ generated), and
  `docs/public/plugins-authoring.md`.
- **Tests:** `step_approval_test.go` (awaiting after a stamped completed
  turn, not after re-entry, pending/approved/running/starting rules, Office
  stage, approve once then not awaiting), `human_responses_test.go`
  (`TestApproveStepActsAsTheLinkedUserOnce`,
  `TestApproveStepWithoutAwaitingIsConflict`), the backendapp adapter test,
  and the SDK round trip.
