---
id: "16-host-step-approval-rpcs"
title: "Host: workflow step approval (ApproveStep, GetStepApproval)"
status: pending
wave: 4
depends_on: ["01-host-control-rpcs-adr", "15-host-delegated-human-responses"]
plan: "plan.md"
spec: "../../specs/integrations/discord.md"
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
Pending.
