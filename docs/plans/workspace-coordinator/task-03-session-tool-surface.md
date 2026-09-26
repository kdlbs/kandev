---
id: "03-session-tool-surface"
title: "Coordinator session, tool surface and proposals written"
status: pending
wave: 2
depends_on:
  - "01-shared-interface"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-COORDINATORS-002
  - REQ-COORDINATOR-COORDINATORS-005
  - REQ-COORDINATOR-COPILOT-001
  - REQ-COORDINATOR-COPILOT-002
  - REQ-COORDINATOR-COPILOT-003
  - REQ-COORDINATOR-PROPOSALS-001
acceptance_criteria:
  - AC-COORDINATOR-COORDINATORS-002.7
  - AC-COORDINATOR-COORDINATORS-002.8
  - AC-COORDINATOR-COORDINATORS-005.1
  - AC-COORDINATOR-COORDINATORS-005.2
  - AC-COORDINATOR-COORDINATORS-005.3
  - AC-COORDINATOR-COPILOT-001.1
  - AC-COORDINATOR-COPILOT-001.2
  - AC-COORDINATOR-COPILOT-001.3
  - AC-COORDINATOR-COPILOT-001.4
  - AC-COORDINATOR-COPILOT-001.5
  - AC-COORDINATOR-COPILOT-001.6
  - AC-COORDINATOR-COPILOT-001.7
  - AC-COORDINATOR-COPILOT-001.8
  - AC-COORDINATOR-COPILOT-002.1
  - AC-COORDINATOR-COPILOT-002.2
  - AC-COORDINATOR-COPILOT-002.3
  - AC-COORDINATOR-COPILOT-003.1
  - AC-COORDINATOR-COPILOT-003.2
  - AC-COORDINATOR-COPILOT-003.3
  - AC-COORDINATOR-COPILOT-003.4
  - AC-COORDINATOR-COPILOT-003.5
  - AC-COORDINATOR-COPILOT-003.6
  - AC-COORDINATOR-COPILOT-003.7
  - AC-COORDINATOR-COPILOT-003.8
  - AC-COORDINATOR-COPILOT-003.9
  - AC-COORDINATOR-PROPOSALS-001.1
  - AC-COORDINATOR-PROPOSALS-001.2
  - AC-COORDINATOR-PROPOSALS-001.3
  - AC-COORDINATOR-PROPOSALS-001.4
  - AC-COORDINATOR-PROPOSALS-001.5
system_design:
  - ../../specs/coordinator/system-design/copilot.md
  - ../../specs/coordinator/system-design/proposals.md
  - ../../specs/coordinator/system-design/coordinators.md
---

# Task 03: Coordinator Session, Tool Surface and Proposals Written (WP-2)

## Summary

Give each coordinator a conversation task and an attended session whose Kandev
surface can only read the workspace and propose tasks. `propose_task_kandev`
writes through task 01's store and publishes task 01's `coordinator.updated`.
Deciding proposals is task 07. Backend only. On the critical path.

## In scope

- First step: check main for a `coordinator` surface, mode or origin name, an
  `app/coordinator/` directory or a `/coordinator` route already taken (D7);
  rename in the design if one is.
- Conversation route handler (task 01 declared its types) in the
  conversation registration function of `backendapp/coordinator.go`: create
  once with `coordinator_id` metadata and no auto-start marker, race-safe
  including a failed loser cleanup and a racing coordinator delete, recreate,
  session ensured through `EnsureSession` with `AutoStart: &false` and
  `ActivationSource: session_open`, 502 on an ensure failure with the task
  kept, 409 with the `coordinator_profile_unavailable` body on a status other
  than `ok` from task 01's `profileStatus`.
- `TaskOriginCoordinator` (task 01's constant) refused at HTTP and MCP task
  create. The `coordinator-proposal:` external-id prefix refusal is task 07's.
- `ListCoordinatorOriginTasks` in the task repository (SQLite and PostgreSQL)
  and the startup conversation cleanup: delete tasks whose `coordinator_id`
  names no coordinator, archive unarchived tasks that are not their
  coordinator's current conversation, considering only tasks created before
  `T0`. This cleanup hooks into task 01's conversation registration function
  (its startup-pass hook slot), not into the shared pass's call site.
- Quick Chat idle expiry excludes origin `coordinator` in
  `ListExpiredQuickChatTasks` and `DeleteExpiredQuickChatTask`.
- Standing-instructions system block (`sysprompt`) and launch-prompt branch;
  a context change archives the current conversation task and clears the
  reference; coordinator delete additionally removes every conversation task
  of the coordinator (current and archived).
- Every resolution site uses task 01's constants: `principalSurface`,
  `CoordinatorLookup`, the coordinator branches in
  `Executor.resolveTaskSessionMCPMode` and `resolveTaskSessionMCPProfile`, the
  agentctl mode cases, plugin tools skipped.
- `registerCoordinatorTools` with exactly the seven tools of
  `AC-COORDINATOR-COPILOT-003.1`; `propose_task_kandev` (open-proposal cap
  under a per-coordinator lock) writing through task 01's proposal insert and
  publishing `coordinator.updated`; the `coordinator.propose_task` action; the
  guard in `coordinator_authorization.go`.
- Fail-closed start checks, including the profile check at session start;
  exact-name auto-approval; `AutoApprovePermissionsOverride=false` on every
  lifecycle path.
- `autoResumeEligibility` returns `coordinator_message_only`;
  `IsRestorableQuickChatTask` excludes the origin; `message.add` needs
  `workspace.manage`.
- Mock agent support for `e2e:mcp:kandev:propose_task_kandev({...})`.

The settings-page and copilot halves of `AC-COORDINATOR-COORDINATORS-005.1`
to `005.3` are built in tasks 02 and 06; this work order owns the criteria
because the route's 409 and the session-start check are the enforcement.

## Out of scope

- Approve and reject routes, claim, recovery, the reserved external-id prefix
  (task 07).
- Any UI (tasks 02, 04, 05, 06, 08).
- Containment of the agent CLI's own tools (gate G4).

## Mockup screenshots and scenarios

Screenshots (visual reference; the acceptance criteria govern):

- [`docs/plans/workspace-coordinator/assets/p1-05-chat-create-task-proposal.png`](assets/p1-05-chat-create-task-proposal.png)

Mockup scenario specs to port (in the workspace-coordinator analysis
mockup's `mockup/e2e/tests/`, outside this repository; see the plan's [Mockup scenario to repo test](plan.md#mockup-scenario-to-repo-test)):

- None ported as Playwright here: this work order is backend only. The tool-call and proposal behaviour `p1-05` shows is covered by the Go tests below and ported as Playwright in tasks 06 and 08.

## Acceptance

- With the mock agent, a coordinator session lists tasks and writes one
  `pending` proposal; no task is created and every other Kandev action is
  refused with its name.
- No path other than a manager's `message.add` starts or resumes the
  conversation session, and the session never starts in the regular task mode.
- Auto-approval covers only the coordinator tools, regardless of the profile's
  `auto_approve` or `AGENTCTL_AUTO_APPROVE_PERMISSIONS`, on first launch,
  re-created, resumed and promoted executions.

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... ./internal/mcp/... ./internal/agentctl/... ./internal/task/... ./internal/orchestrator/...
cd apps/backend && go test ./internal/mcp/handlers/ -run 'TestCoordinator' -count=1
cd apps/backend && make lint
```

Required Go tests:

- a table over every registered Kandev MCP action: only allowlisted actions
  run for a coordinator principal;
- `Legacy` instance and `SetMcpMode` from task mode both list exactly the
  coordinator tools, with no user-question, title or plugin tool;
- foreign workspace ids refused, including a `workspace_id` argument other
  than the principal's to `list_workflows_kandev` and
  `list_repositories_kandev` (refused before the handler runs, no data
  returned); non-coordinator principal refused for the
  proposal action; HTTP and MCP create refuse the origin;
- a `workspace.read` member's `message.add` on the conversation task is
  refused and starts no turn, while a `workspace.manage` member's succeeds
  (`AC-COORDINATOR-COPILOT-002.3`);
- 30 concurrent proposes against an empty coordinator leave exactly 25 open
  on SQLite and PostgreSQL;
- a racing coordinator delete during a conversation open returns 404 and
  leaves no task;
- two racing opens return the same task and one session, and the losing task
  is deleted; an open of a live task with no session creates one `CREATED`
  session with no agent; an ensure failure returns 502 and the next open
  retries with the same task; the created task carries no
  `auto_start_on_create` marker, and no agent runs after create or open;
- a context change archives the old conversation task (a running turn
  stops), the next open creates a new one, and the archived one never
  resolves to a coordinator; coordinator delete removes current and archived
  conversation tasks and all proposals, and leaves untouched a task seeded
  with a `coordinator-proposal:`-prefixed external id and an `approved`
  proposal row created directly through task 01's store (not through task
  07's approve route, which this work order does not depend on)
  (`AC-COORDINATOR-COORDINATORS-002.8`); the startup cleanup deletes a task
  with an unknown `coordinator_id`, archives an unreferenced unarchived one,
  and changes nothing on a second run; a conversation task created at or
  after `T0` is left alone, and an already-deleted or already-archived task
  counts as done;
- a conversation task idle for eight days is not listed or deleted by Quick
  Chat expiry, on SQLite and PostgreSQL, while an ordinary quick chat still is;
- flag off, missing row, unreadable task, passthrough or missing agent
  profile, missing executor profile: no start, and the conversation route
  returns 409 with the `coordinator_profile_unavailable` body carrying both
  statuses (a table over agent `missing`, agent `passthrough`, executor
  `missing` and agent `passthrough` with executor `missing`); a profile read
  error other than not found returns 500 from the route and starts nothing;
- `mcp__kandev__list_tasks_kandev` auto-approved and
  `mcp__kandev__move_task_kandev` not, with the profile flag and the
  environment variable set, on each lifecycle path;
- `auto_resume_allowed` false; conversation route sends no prompt; a table
  over every backend path that can start a turn (stall and
  `workspace.deleted` subscribers, proposal decisions, startup recovery,
  session recovery on restart) shows none starts the session; whichever of
  tasks 03, 04 and 07 merges last into this table adds the rows for the
  paths owned by the other two, so the table is complete regardless of merge
  order;
- propose refuses an auto-start step, a foreign repository, a step that is
  neither start nor manual-move, a 26th open proposal; two identical calls
  make two proposals; each successful propose publishes one
  `coordinator.updated`.

## Likely files

- `apps/backend/internal/coordinator/{conversation,prompt,propose}.go` and tests
- `apps/backend/internal/task/handlers/` and the MCP `create_task` handler
- `apps/backend/internal/mcp/handlers/coordinator_authorization.go`
- `apps/backend/internal/mcp/scope/` (`principalSurface`, `CoordinatorLookup`)
- `apps/backend/internal/mcp/server/server.go` (`registerCoordinatorTools`, mode cases)
- `apps/backend/internal/agentctl/` mode handling (`normalizeMode`, `surfaceForMode`, `modeForProfile`, `SetMode`)
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go` (`resolveTaskSessionMCPMode`, `resolveTaskSessionMCPProfile`)
- `apps/backend/internal/task/repository/sqlite/task.go` (`ListCoordinatorOriginTasks`, the expiry predicates)
- `apps/backend/internal/task/service/quick_chat_expiration.go` tests
- `apps/backend/internal/backendapp/coordinator.go` (conversation registration function only)
- mock agent script support under `apps/backend/cmd/mock-agent/`

## Dependencies

- Task 01 (store, types, constants, event, flag). While G0 is open the branch
  starts from task 01's branch and rebases onto main after each predecessor
  merges.
- Open PRs #2756, #2841, #2909, #2974, #3048, #3155 and #3165 also edit
  `internal/mcp`; whichever lands second rebases.

## Risks

- A future MCP action added without updating the guard: the table test fails
  closed by design.
- The permission override must be explicit `false`, not absent, or the
  environment variable wins.
