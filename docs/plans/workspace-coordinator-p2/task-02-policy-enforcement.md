---
id: "02-policy-enforcement"
title: "Policy and Watches enforcement"
status: pending
wave: 3
depends_on:
  - "01-shared-interface"
  - "03-activity-log-backend"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-COORDINATORS-007
  - REQ-COORDINATOR-PERMISSIONS-001
  - REQ-COORDINATOR-PERMISSIONS-002
  - REQ-COORDINATOR-PERMISSIONS-003
  - REQ-COORDINATOR-PERMISSIONS-004
acceptance_criteria:
  - AC-COORDINATOR-COORDINATORS-007.2
  - AC-COORDINATOR-COORDINATORS-007.4
  - AC-COORDINATOR-PERMISSIONS-001.2
  - AC-COORDINATOR-PERMISSIONS-001.3
  - AC-COORDINATOR-PERMISSIONS-001.4
  - AC-COORDINATOR-PERMISSIONS-001.5
  - AC-COORDINATOR-PERMISSIONS-002.1
  - AC-COORDINATOR-PERMISSIONS-002.2
  - AC-COORDINATOR-PERMISSIONS-002.3
  - AC-COORDINATOR-PERMISSIONS-002.4
  - AC-COORDINATOR-PERMISSIONS-002.5
  - AC-COORDINATOR-PERMISSIONS-002.6
  - AC-COORDINATOR-PERMISSIONS-003.2
  - AC-COORDINATOR-PERMISSIONS-003.3
  - AC-COORDINATOR-PERMISSIONS-003.5
  - AC-COORDINATOR-PERMISSIONS-004.1
  - AC-COORDINATOR-PERMISSIONS-004.2
system_design:
  - ../../specs/coordinator/system-design/permissions.md
  - ../../specs/coordinator/system-design/coordinators.md
---

# Task 02: Policy and Watches Enforcement (WP-7)

## Summary

Make the tool profile the output of the policy: settings routes, the derived
and bound `CoordinatorToolPolicy`, registration and auto-approval from the
bound list, the guard's bound-list, live-policy and Watches checks with
refused log rows, workflow-deletion handling and the approve re-check.

## In scope

- `GET` and `PUT .../settings` with validation (`automatic_not_available`,
  stop denied-only, Watches 1 to 50, duplicates, foreign workflows), the
  locked save, the equal-save no-op, `policy_revision` and
  `resetConversation` (`001.2` to `001.5`, `003.2`, `004.1`, `004.2`).
- `toolprofile.go` `ToolNames(policy, phase2)` (`002.1`).
- `CoordinatorToolPolicy` in `internal/mcp/profile` with marshal, parse and
  validate; stamping at conversation-task creation; transport through the
  executor's coordinator branch, `buildLaunchMetadata` strip-and-re-set, and
  `mcpHandlerFor` parse; refusing the `kandev.coordinator_` metadata prefix
  on HTTP and MCP task create and update (`002.3`).
- `registerCoordinatorTools` from the bound names; agentctl
  `CoordinatorToolNames` for auto-approval by full qualified name (`002.4`).
- Guard: bound-list check, live `Allows` for propose actions, the Watches
  filter per tool, `RecordRefusal` with reason codes, the unchanged refusal
  text, and no settings-shaped action on the surface (`002.2`, `002.5`,
  `003.3`).
- Workflow-deleted subscriber (`003.5` backend half; the Configure notice
  is task 06's and the Needs you notice task 11's).
- Approve re-check 409 `policy_denied` in the phase-1 approve route
  (`002.6`; the card copy is task 09's).
- Flag-off behaviour: phase-1 tools, ignored bindings and stored policy,
  unregistered phase-2 routes (`AC-COORDINATOR-COORDINATORS-007.2`, `007.4`;
  the web half of `007.2` is each UI task's flag-off test).
- `001.4`: a test that no registered action, route or proposal kind merges
  or targets a `CompleteTaskOnEnter` step (the kind validators are task 04's;
  this task's test walks the registry).

## Out of scope

- May do and Watches screens (task 06).
- New propose tools' handlers (task 04); this task lists their names.

## Acceptance

- The session's registered tools, the auto-approved names and the bound list
  are the same set, and equal `ToolNames(policy)`.
- A setting tightened mid-conversation refuses the next call and logs it.
- A forged or unparsable binding refuses every action.

## Verification

```bash
make -C apps/backend test PKG=./internal/coordinator/...
make -C apps/backend test PKG=./internal/mcp/...
make -C apps/backend test PKG=./internal/agent/runtime/lifecycle/...
make -C apps/backend test PKG=./internal/orchestrator/executor/...
make -C apps/backend test PKG=./internal/agentctl/...
make -C apps/backend test PKG=./internal/task/...
cd apps/web && pnpm e2e:run tests/coordinator/policy-enforcement.spec.ts
```

The guard table test is extended over every registered action times each
setting times Watches `all`, `selected` and empty. Store tests on both
dialects: two concurrent saves bump the revision twice and the last commit
wins (`004.2`). A Playwright spec with the mock agent sets Message to
Denied, opens a conversation and asserts the mock's `propose_message_kandev`
call is refused and a refused row appears through the activity route.

## Likely files

- `apps/backend/internal/coordinator/settings_routes.go`, `toolprofile.go`,
  `conversation.go`, `watches.go`, `subscribers.go`, `proposals.go`
- `apps/backend/internal/mcp/profile/profile.go`
- `apps/backend/internal/mcp/server/coordinator_tools.go`
- `apps/backend/internal/mcp/handlers/coordinator_authorization.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`,
  `mcp_identity.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/agentctl/` permission policy
- `apps/backend/internal/task/service/` metadata prefix refusal

## Dependencies

- Task 01 (store, policy value, `resetConversation`); task 03
  (`RecordRefusal`).

## Risks

- A missed wiring site would grant the phase-1 set, not more: a missing
  binding is the phase-1 profile, never the full Kanban set. The executor
  test asserts the coordinator surface is never downgraded.
