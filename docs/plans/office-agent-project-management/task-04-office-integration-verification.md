---
id: "04-office-integration-verification"
title: "Verify the Office setup contract"
status: done
wave: 3
depends_on: ["01-project-runtime-capability", "02-project-cli", "03-office-capability-context"]
plan: "plan.md"
spec: "../../specs/office/overview.md"
---

# Task 04: Verify the Office setup contract

## Acceptance

- A signed CEO run can create a project and then create a task assigned to that project through the same endpoints used by the CLI.
- A non-authorized Office run cannot create a project, and cross-workspace input cannot escape token scope.
- The default onboarding brief requests no mutation absent from the CEO's advertised CLI/tool capability catalog.

## Verification

```bash
cd apps/backend && rtk go test ./internal/office/runtime ./internal/office/onboarding ./cmd/agentctl ./internal/mcp/server -count=1
```

## Files Likely Touched

- `apps/backend/internal/office/runtime/handler_test.go`
- `apps/backend/internal/office/onboarding/service_test.go`
- `apps/backend/cmd/agentctl/kandev_test.go`
- `apps/backend/internal/mcp/server/sysprompt_sync_test.go`

## Inputs

- Completed Tasks 01-03.
- Default brief in `apps/web/app/office/setup/setup-task-defaults.ts`.

## Output Contract

Act as test-engineer. Report scenario coverage, exact commands/results, missing coverage, security risks, blockers, and task status. Do not broaden the feature or edit `plan.md`.

## Completion Evidence

- Added a SQLite-backed signed-runtime test proving a CEO-capable run persists a
  project only in the workspace carried by its JWT, even when the request body
  supplies a different `workspace_id`.
- Added a denial test proving a run without `create_project` receives HTTP 403
  and leaves project persistence empty.
- Added a socket-free CLI workflow test that performs `projects create` followed
  by `task create --project`, then verifies the task payload contains both the
  returned project ID, omits caller-selected workspace data, and that the project
  call carries the signed run token.
- Added an onboarding contract test tying every default-brief mutation to the
  CEO's advertised Office context or bundled skills. It also rejects workspace
  creation and `step_complete_kandev` in the Office brief. Role selection is the
  advertised path that supplies role-derived responsibilities, permissions, and
  operating defaults.
- `GOCACHE=/tmp/kandev-go-cache rtk go test ./internal/office/runtime ./internal/office/onboarding -count=1`
  passed: 61 tests in 2 packages.
- Focused socket-free runtime, onboarding, CLI, and exact Office MCP inventory
  commands passed: 11 tests across 4 packages.
- The assigned combined command reached 71 passing tests, then failed only when
  existing `TestNewKandevClient_NormalizesVersionedAPIURL` and
  `TestAskUserQuestion_StreamsKeepAliveDuringWait` attempted to bind loopback
  `httptest` sockets; the sandbox returned `operation not permitted`.
- `git diff --check` passed. No production defect or new security risk was found.

## Corrective Evidence

- Closed the final review blockers by moving Office CLI task creation off the generic task API and onto the signed Office runtime surface.
- Signed `ws-1` handler coverage proves body `workspace_id` spoofing is ignored, successful project/assignee creation forwards `ws-1`, and cross-workspace project, parent, or assignee inputs return 403 with only a `runtime.denied` event and no creator invocation.
- Real task-service boundary tests passed for Office project persistence and `assignee_agent_profile_id` runner projection: 3 tests.
- Split onboarding and runtime-agent root-task creation at the Office service adapter contract. SQLite-backed adapter coverage now proves runtime-agent tasks persist `origin=agent_created` together with `project_id` and `assignee_agent_profile_id`, while onboarding tasks retain `origin=onboarding`; existing child-task origin behavior is unchanged.
- Dependent Office/backend packages compile with `GOCACHE=/tmp/kandev-go-cache rtk go test ./internal/office/... ./internal/backendapp ./cmd/agentctl -run '^$' -count=1`.
- Broader runtime/service/CLI execution reached 65 passing tests; only two pre-existing loopback `httptest` cases failed because the restricted sandbox denies `listen tcp6 [::1]:0`.
- Runtime task creation now trims titles at the action boundary and rejects
  whitespace-only root and child titles before task lookup or persistence. The
  handler returns HTTP 400 and records `runtime.denied` for both shapes; the CLI
  rejects the same input before constructing an HTTP client or request.
- Runtime project creation resolves every nonempty `lead_agent_profile_id`
  through the Office agent service and requires the trimmed input to equal the
  resolved profile ID in the signed run workspace. Missing IDs, names/aliases,
  and cross-workspace IDs return HTTP 403, emit `runtime.denied`, and never
  invoke project persistence; empty leads remain accepted.
- Runtime task assignment enforces the same canonical-ID and workspace contract,
  trims valid IDs before persistence, and denies same-workspace aliases before
  the task creator runs. Handler coverage exercises the real agent-service name
  fallback and verifies the denial audit event.
- Final focused results: all 72 runtime package tests and all 11 socket-free CLI
  task-create contract tests passed. The Office subtree and CLI packages also
  compiled with `-run '^$'`; full CLI execution remains limited by the sandbox's
  loopback socket restriction in pre-existing `httptest` cases.
- Closed the comment-path authorization gap: `tasks message` now reaches the
  authenticated runtime comment action, so task scope comes from signed
  `RunContext` and attribution comes from its agent identity. The CLI payload
  contains no caller-controlled author or source fields.
- Bundled escalation no longer attempts to comment on a newly created human
  task outside the current run's scope. The human task description and the
  current blocked-task comment retain the backlink, followed by the supported
  blocked-status update and truthful manual recovery instructions.
- Focused security results: 3 socket-free CLI message tests, all 91 Office skill
  tests, 4 existing runtime comment scope/attribution tests, and targeted lint
  passed.
- Final authorization follow-up removed the remaining task-update dashboard
  bypass and enforced `create_subtask` plus signed parent scope for every
  parent-bearing runtime task-create request. Denials occur before relationship
  lookup or task creation and emit only the existing `runtime.denied` event.
- Verification passed: 78 runtime tests, 4 socket-free CLI task-update tests, 91
  bundled Office skill tests, CLI compile-only, `git diff --check`, and backend
  lint with 0 issues. The unfiltered CLI suite remains blocked only by the
  sandbox's existing loopback `httptest` restriction.
- Closed the final generic-mutation bypass by making `kandev tasks move` and
  `kandev tasks archive` fail locally for Office agents. Neither command creates
  an HTTP request; the errors direct status changes to the signed runtime update
  command and reserve workflow-step moves and archival for human/admin actions.
- Removed both commands from the bundled task-operations skill and its embedded
  reference. A bundled-file contract test now scans top-level skill content and
  supporting files for these unsupported examples.
- Final focused verification passed: 9 socket-free CLI tests, all 91 Office
  skill tests, backend formatting, backend lint with 0 issues, `git diff
  --check`, and a direct scan confirming no bundled Office skill advertises
  either unsafe command.
