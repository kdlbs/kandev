---
created: 2026-09-14
status: complete
requirements:
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-002
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-003
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-004
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-005
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-006
system_design:
  - ../../specs/integrations/system-design/task-change-link-mcp.md
legacy_specs: []
---

# Implementation plan: Provider-neutral change request MCP tools

## Overview

Deliver the four-tool contract through five sequential work orders.
Implement management and reads first, then automation and bound outcomes.
Finish with caller migration, catalog cutover, and protocol evidence.
Intermediate commits may retain old registrations until the final cutover.

Baseline: PR [#3506](https://github.com/kdlbs/kandev/pull/3506), merged
2026-09-14 at 14:00:36 UTC, commit `262ecdf270b5171538a68e9937dda7894ee01673`.
Fetched `origin/main` and HEAD both matched that commit. The working tree was clean.
Maintainer fixes in that merge are included. Contributor review and CI remediation are excluded.
Refresh the base before implementation and reconcile only relevant later changes.

The integration system owns this package because it owns contribution identity,
automation settings, and provider capabilities. No rendered UI changes are required.

## Scope

### In scope

- Shared discovery, strict schemas, backend dispatch, and provider adapters.
- Explicit automation targeting and provider-scoped prompts.
- Existing principal, origin, credentials, cleanup, and replacement guarantees.
- Bound GitHub outcome reporting under the neutral name.
- Caller migration, transport retirement, public reference updates, and protocol tests.

### Out of scope

- Provider-store rewrite, new schema, cross-provider replacement, or GitLab outcome tracking.
- UI changes, automation algorithm changes, new providers, or a broad verification audit.

## Technical approach

Use [the system design](../../specs/integrations/system-design/task-change-link-mcp.md)
for exact request/response contracts and failure semantics.
Reuse `TaskChangeLinkRequest`, `taskChangeLinkCoordinator`, provider automation
service methods, existing event publishers, and orchestrator active-turn resolution.
Add neutral DTOs and backend actions without exporting provider persistence shapes.

The read exposes provider-labelled capabilities and prompt settings. Automation
uses `target.scope` and an explicit task `providers` array. The outcome tool
accepts only outcome and summary. Keep list-task projections unchanged.

The new catalog removes all eight old names in task 05. Existing WS actions
remain for one release transition, with original scope and principal checks.
Remove compatibility in the first stable release after introduction. At delivery,
record the introducing version and this removal obligation in the release notes.
No aliases remain advertised by the new runtime. Old processes require resume
before the compatibility-removal release; registry refresh cannot upgrade binaries.

## Tests

The following tests provide the implementation evidence for the mapped criteria.

| Acceptance criteria suffixes in `AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-*` | Implementation evidence |
| --- | --- |
| 001.1, 001.2, 001.5; 003.1 | `mcp/server/task_change_request_tools_test.go:TestManageTaskChangeRequestSchemaAndDispatch`; `mcp/handlers/task_change_request_test.go:TestManageTaskChangeRequestDispatchesWithTrustedCaller` |
| 001.3, 001.4, 001.6; 003.2, 003.3 | `backendapp/task_change_link_coordinator_test.go:TestTaskChangeCoordinatorReplaceRollsBackNewLinkWhenOldUnlinkFails`; `TestTaskChangeCoordinatorReportsRollbackFailure`; retained provider detach/unlink tests |
| 002.1, 002.3, 002.4 | `backendapp/task_change_request_read_test.go:TestTaskChangeRequestReaderPreservesProviderAvailabilityAndFailures`; `mcp/handlers/task_change_request_read_test.go:TestGetTaskChangeRequestsUsesBoundPrincipal` |
| 002.2; 006.1, 006.2 | `mcp/server/server_test.go:TestServerSurfaceAutomationHasFixedCoordinatorCatalog`; `mcp/server/task_change_request_protocol_test.go:TestTaskChangeRequestProtocolCatalogAndCallAcrossProtocolEras` |
| 004.1, 004.2, 004.3 | `mcp/server/task_change_request_tools_test.go:TestChangeRequestAutomationToolSchemaAndDispatch`; `backendapp/task_change_request_automation_test.go:TestChangeRequestAutomationTargetMatrix` |
| 004.4, 004.5 | `backendapp/task_change_request_automation_test.go:TestChangeRequestAutomationPreflightRejectsUnresolvedWithoutWrites`; `TestChangeRequestAutomationPartialFailure`; `TestGitLabTaskChangeRequestReadResolvesOnlyVerifiedIdentity` |
| 005.1, 005.2, 005.3 | `mcp/server/pr_auto_fix_outcome_scope_test.go`; `mcp/handlers/task_pr_automation_outcome_scope_test.go`; `orchestrator/ci_automation_outcome_scope_test.go`; `github/store_ci_outcome_scope_test.go` |

Paths in this table are relative to `apps/backend/internal/`.

Preserve existing tests that prove the merged guarantees:
`TestTaskChangeCoordinatorUnlinksStaleAssociationWithoutCurrentRepository`,
`TestTaskChangeCoordinatorReportsRollbackFailure`,
`TestTaskPRDetachRetiresOnlyExactAutomationAndCIState`, and
`TestUnlinkTaskMRPublishesWorkspaceScopedDeletedEvent`.
Their provider suites also cover persistence, tombstones, and workspace routing.

## End-to-end protocol evidence

The changed user surface is MCP. The implementation uses the existing in-process
protocol harness instead of introducing a browser test for unchanged UI.
`mcp/server/task_change_request_protocol_test.go` exercises serialized modern
direct requests and legacy initialize/list/call requests, verifies the neutral
catalog in both protocol eras, and proves superseded calls do not reach the
backend. Focused coordinator and handler tests cover provider writes, read
failures, identity resolution, authorization, replacement failures, mixed
provider partial results, and bound outcomes. Orchestrator tests cover queued
protocol delivery, edited prompt preservation, catalog selection, and current
turn scope.

The implemented package covers 001.1–001.6, 002.1–002.4, 003.1–003.3,
004.1–004.5, 005.1–005.3, and 006.1–006.2 through focused coordinator,
handler, catalog, and protocol tests. The serialized protocol fixture covers
both MCP protocol eras, the neutral catalog, bound task dispatch, and rejection
of a superseded tool call without a backend mutation.

## Representative agent requests

Use fixture IDs from discovery. These examples assess intended tool selection;
argument schema tests and serialized-call tests provide executable evidence.

| Agent intent | Tool and arguments | Required result |
| --- | --- | --- |
| Show this task's PRs and MRs and available automation | `get_task_change_requests_kandev {}` | Both providers, separate prompts, truthful capabilities |
| Link GitLab MR 42 to sibling task B | `manage_task_change_request_kandev {"operation":"link","task_id":"B","provider":"gitlab","repository_id":"repo-gl","number":42}` | Workspace-authorized link; no agent project path |
| Remove that MR after detaching its repository | Same tool with `operation:"unlink"` and the same triple | Exact stale association removed |
| Replace GitHub PR 7 with PR 8 | Same tool with `operation:"replace"`, explicit new triple and `old_provider`, `old_repository_id`, `old_number` | One call; link-before-unlink |
| Disable auto-fix only for PR 8 | `update_task_change_request_automation_kandev {"target":{"scope":"association","provider":"github","repository_id":"repo-gh","number":8},"patch":{"auto_fix_enabled":false}}` | Other contributions unchanged |
| Enable merge notifications for this task's GitHub PRs and GitLab MRs | Same tool with `target:{"scope":"task","providers":["github","gitlab"]}`, `patch:{"prompt_on_merged":true}` | Explicit mixed-provider update |
| Clear the task's GitLab custom prompt | Same tool with `target:{"scope":"task","providers":["gitlab"]}`, `patch:{"auto_fix_prompt_override":""}` | GitHub prompt unchanged; valid without MRs |
| Give only PR 8 a different auto-fix prompt | Association target with prompt patch | Reject; explain task/provider prompt scope |
| Fix PR 8 manually, then report completion | No outcome tool call | Ordinary completion does not report an automation outcome |
| Report a Kandev-dispatched GitHub fix | `report_change_request_auto_fix_outcome_kandev {"outcome":"action_taken","summary":"Pushed the check fix."}` | Exact bound attempt recorded |
| Report a GitLab auto-fix turn | No outcome call; GitLab capability is false | No accidental GitHub report |
| Enable auto-merge with no target | Automation patch without `target` | Reject before writes |
| Replace GitHub PR with GitLab MR | Manage replace with different providers | Reject before writes |
| Set automation on sibling task B | Automation request with `task_id:"B"` | Reject supplied execution identity |

Also test typo operations, unknown providers, fractional/zero numbers, null fields,
partial triples, duplicate provider selections, unknown patch keys, and empty patches.
Assessment passes only if each valid intent selects one tool and each invalid
intent fails without a broader target or side effect. Record any ambiguity in task 05.

## Work orders

- [x] [Task 01: Shared management contract](task-01-management.md) (`done`)
- [x] [Task 02: Read contributions and capabilities](task-02-read-capabilities.md) (`done`)
- [x] [Task 03: Explicit automation targets](task-03-automation-targets.md) (`done`)
- [x] [Task 04: Bound outcome tool](task-04-bound-outcome.md) (`done`)
- [x] [Task 05: Catalog and caller cutover](task-05-catalog-cutover.md) (`done`)

Work orders were executed sequentially: 01 → 02 → 03 → 04 → 05.
Each work order specifies its source and test ownership.

## Release follow-up

- Introducing stable release: `0.95.0` (planned).
- [ ] Before the first stable release after `0.95.0`, remove the retained
  provider-specific backend WebSocket action handlers and their tests.
- [ ] Verify that no old MCP registration or active caller reference remains
  after the compatibility handlers are removed.

## Verification results

Implementation validation on 2026-09-14:

- `make -C apps/backend lint`: passed.
- `make -C apps/backend build`: passed.
- `go test ./internal/backendapp ./internal/mcp/handlers ./internal/mcp/server ./internal/orchestrator ./config/workflows ./internal/task/dto ./pkg/api/v1 -count=1`: passed.
- Provider and orchestration focused tests: passed.
- MCP server and backend application race checks: passed.
- Review regressions passed:
  - `go test ./internal/mcp/handlers -run 'TestManageTaskChangeRequest' -count=1`
  - `go test ./internal/mcp/server -run 'TestManageTaskChangeRequest|TestTaskChangeRequestTool' -count=1`
  - `go test ./internal/backendapp -run 'TestTaskChangeManagementDispatches|TestTaskChangeCoordinator|TestChangeRequestAutomationAssociation' -count=1`
  - `go test ./internal/orchestrator -run 'TestReplaceCIAutoFixOutcomeProtocol' -count=1`
  These cover rich handler-to-coordinator replacement dispatch, serialized
  management failure details, historical queued protocol migration for wrapped
  and passthrough prompts, targeted GitLab affected identities, and read-to-
  management cleanup of legacy GitLab associations.
- `make -C apps/backend test`: the changed packages passed, but the aggregate
  target returned nonzero on existing environment-sensitive config, launcher,
  Office migration, and process-probe tests. Config and launcher passed when
  rerun without inherited internal config variables; the Office migration test
  passed in isolation, and deterministic process-probe tests passed. None of
  those packages are changed by this implementation.
- `git diff --check`: passed.

Documentation and design-package validation on 2026-09-14:

- `python3 scripts/list-docs.py validate`: passed (268 decisions, 909 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- Catalog discovery includes the updated pair and accepted ADR.
- `git diff --check -- docs/specs docs/decisions docs/plans`: passed.
- Work-order links, requirement IDs, and acceptance IDs: validated locally.

Public docs, workflow callers, requirements, system design, and the decision
record were updated during task 05. The implementation adds permanent backend,
handler, protocol, and coordinator tests.

## Risks

- Old runtime binaries keep their catalog until resume; migration must not emit an unavailable tool name.
- Historical GitLab rows can lack canonical identity. Ambiguous rows must remain visible and fail closed.
- Provider transactions are independent. Readback can fail after a successful write.
- Repeated switch writes can reset provider checkpoints; avoid replaying already-applied mixed updates.
- Another main-branch change can alter provider capabilities or caller inventory before implementation.
