---
id: "02-policy-enforcement"
title: "Policy and Watches enforcement"
status: pending
wave: 2
depends_on:
  - "01-shared-interface"
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

- `GET` and `PUT .../settings` with validation and the design's closed set of
  400 codes and `field` values (`automatic_not_available`, stop denied-only,
  Watches 1 to 50, duplicates, foreign workflows, `null` members as absent,
  workflow ids read before the lock), the locked save, the equal-save no-op,
  the PUT returning the coordinator DTO, `policy_revision` and
  `resetConversation` (`001.2` to `001.5`, `003.2`, `004.1`, `004.2`).
- `toolprofile.go` `ToolNames(policy, phase2)` (`002.1`).
- `CoordinatorToolPolicy` in `internal/mcp/profile` with marshal, parse and
  validate; stamping at conversation-task creation; transport through the
  executor's coordinator branch, `buildLaunchMetadata` strip-and-re-set, and
  `mcpHandlerFor` parse; `sameProfile` comparing the binding; refusing the
  `kandev.coordinator_` metadata prefix in the task service's `CreateTask`
  and `UpdateTask`, so HTTP, MCP and WebSocket `task.create` and
  `task.update` all inherit it, with `AllowReservedMetadata` for the
  coordinator service (`002.3`).
- `registerCoordinatorTools` from the bound names, registering each bound
  name that has a handler in the tool catalog (`ToolForAction`'s table); in
  this task's wave that is the phase-1 seven, and tasks 03 and 04 add their
  tools' rows and handlers. Agentctl `CoordinatorToolNames` carries every
  bound name for auto-approval by full qualified name (`002.4`).
- Guard: the design's ordered checks 0 to 4 and result table, with
  `ToolForAction`, bound-list check, live `Allows` for propose actions, the
  Watches filter per tool, `RecordRefusal` with reason codes and ids taken
  from the principal, the unchanged refusal text, and no settings-shaped
  action on the surface (`002.2`, `002.5`, `003.3`).
- Workflow-deleted subscriber, the post-save reconcile step and the startup
  pass cleanup of watch rows for absent workflows (`003.5` backend half; the
  Configure notice and the Needs you notice are task 06's).
- Approve re-check 409 `policy_denied` in the phase-1 approve route, on new
  claims only and only with the flag on (`002.6`; the card copy is task 09's;
  task 04 adds the kind-specific cases).
- Flag-off behaviour: phase-1 tools, ignored bindings and stored policy,
  unregistered phase-2 routes (`AC-COORDINATOR-COORDINATORS-007.2`, `007.4`;
  the web half of `007.2` is each UI task's flag-off test).
- `001.4`: a test that walks the coordinator tool catalog, the guard
  allowlist, the settings and proposal routes and the settings validator and
  asserts none of them names an action, route or field that merges a pull
  request or moves a task to a `CompleteTaskOnEnter` step. The walk over the
  proposal-kind registry is task 04's, where the registry exists.

## Out of scope

- May do and Watches screens (task 06).
- New propose tools' handlers (task 04); this task lists their names.

## Acceptance

- The auto-approved names equal the bound list; the registered tools equal
  the bound names that have a catalog handler; and the bound list equals
  `ToolNames(policy, true)` of the policy as read at conversation open (the
  phase-1 seven for a conversation with no binding). Task 04 asserts full
  registered equals bound once every tool has landed.
- A setting tightened mid-conversation refuses the next call and logs it.
- A forged or unparsable binding refuses every action.

## Verification

Write the interleaving table first, before code: it is the design's
[Interleavings](../../specs/coordinator/system-design/permissions.md#interleavings)
table, rows 1 to 10, one test per row, each asserting the stated result
(including that row 2 leaves a pending proposal whose approve is 409
`policy_denied`, and that row 5 executes). Commit it failing, then the code.

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
wins (`004.2`). Guard handler tests cover each refused-row case of the
design's Guard table: an unparsable binding (`binding_invalid`), an action
outside the bound names sent straight to the coordinator MCP endpoint
(`not_in_profile`), and a bound propose action whose stored setting was
changed to Denied after binding (`policy_denied`), each asserting one
`refused` row with its class; an action outside the phase-1 allowlist sent
by a coordinator principal asserts one `not_in_profile` row with class
`unknown`; a binding-invalid call writes its row with the principal's
coordinator and workspace ids; a failed policy read refuses and writes no
row; an invalid payload keeps the 400 and writes no row; unwatched reads and propose targets assert
no row. A Playwright spec with the mock agent sets Message to Denied, opens
a conversation and asserts `propose_message_kandev` is not registered for
the session, the mock's call to it creates no proposal, and no refused row
is written (the call never reaches the guard, `002.2`).

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

- Task 01 (store, policy value, `resetConversation`, `RecordRefusal`).

## Risks

- A missed wiring site would grant the phase-1 set, not more: a missing
  binding is the phase-1 profile, never the full Kanban set. The executor
  test asserts the coordinator surface is never downgraded.
