---
id: "01-self-placement"
title: "Implement and verify automatic self sibling placement"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-SELF-SIBLING-001
  - REQ-TASKS-SELF-SIBLING-002
acceptance_criteria:
  - AC-TASKS-SELF-SIBLING-001.1
  - AC-TASKS-SELF-SIBLING-001.2
  - AC-TASKS-SELF-SIBLING-001.3
  - AC-TASKS-SELF-SIBLING-001.4
  - AC-TASKS-SELF-SIBLING-002.1
  - AC-TASKS-SELF-SIBLING-002.2
  - AC-TASKS-SELF-SIBLING-002.3
  - AC-TASKS-SELF-SIBLING-002.4
  - AC-TASKS-SELF-SIBLING-002.5
system_design:
  - ../../specs/tasks/system-design/mcp-self-sibling-placement.md
---

# Task 01: Implement and verify automatic self sibling placement

## Summary

Complete the MCP path for literal-self sibling placement, including a truthful
result notice. Preserve the service invariant, creation source and current
parent-owned coordination. The maintainer authorized implementation on
2026-09-21 and renewed that authorization on 2026-09-24. Implementation and
required validation are complete; the local changes are ready for review.

## In scope

The entire creation slice and test matrix in the [plan](plan.md), task-mode
schema descriptions, and matching public coordination/MCP guidance.

## Out of scope

Service depth relaxation, public intent flags, authority changes, UI, Office
MCP enablement, extra workers, commits/pushes/PR publication without authorization.

## Ordered implementation steps

1. Read the linked requirement/design and backend AGENTS.md. Load `/tdd` and
   backend test conventions. Mark this work order in progress only when the
   workflow authorizes implementation. Keep the current branch and user edits.
2. Write the focused red tests listed in the plan. Include a real dispatcher
   journey, not only a fake backend; verify the tests fail for missing fallback
   or notice, not broken fixtures. Preserve existing literal-ID rejection tests.
3. Add the internal marker and authenticated resolution helper. Resolve a
   maximum-depth caller to its direct valid parent before destination checks.
   Keep source identity unchanged and avoid side effects on rejected input.
4. Add the result metadata/notice across all success branches. Test both
   deduplication states, identity-loss settlement, error paths, output JSON,
   inheritance precedence and launch/dependency behavior. Reuse the existing
   narrow response and admission shapes; avoid growing giant handlers.
5. Update task-mode tool guidance plus public coordination and MCP reference
   pages. State the common parent's controls and the caller's ordinary messaging
   access. Read `/docs-maintainer`; retain external-mode restrictions.
6. Run all commands below, inspect the diff and record actual results here.
   Present results for review within the existing task; do not publish to GitHub.
   Mark done only after the required checks pass. The coordinator reconciles
   spec/plan approval and delivery status against the resulting implementation.

## Acceptance

- The full MCP journey places a depth-limited self request once under the
  common parent, preserving creator attribution and reporting the result.
- Explicit-ID, Office/external/automation, permission, inheritance, deduplication,
  dependency/launch and direct-parent controls satisfy every linked criterion.
- Tool/public docs agree with tested behavior, and all required checks pass
  with results recorded; no implementation occurs before authorization.

## Verification

Run from the repository root. Ensure Go is on PATH; this environment has
`/usr/local/go/bin/go`, so prefix commands with `PATH=/usr/local/go/bin:$PATH`
when needed. These are implementation checks, not checks run during planning.

```sh
(cd apps/backend && PATH=/usr/local/go/bin:$PATH go test -tags fts5 ./internal/mcp/server ./internal/mcp/handlers -count=1)
(cd apps/backend && PATH=/usr/local/go/bin:$PATH go test -tags fts5 ./internal/task/service -run 'TestCreateTask_SubtaskOfSubtask_|TestService_UpdateTask_RejectsNestingUnderSubtask' -count=1)
(cd apps/backend && PATH=/usr/local/go/bin:$PATH go test -tags fts5 ./internal/orchestrator -run 'TestProcessOnChildrenCompleted_|TestHandleTaskMovedToTerminalStepProcessesParentChildrenCompleted' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Also run `gofmt -l` on changed Go files; require no output. The first test command
covers the whole two affected packages, including existing stop, question,
message, creator-session, admission and external-ID regression tests. Do not
count a regex matching no tests as evidence. Name the new tests exactly as in
the plan, or update both artifacts to match before reporting results.

## Files changed

- `apps/backend/internal/mcp/server/handlers.go`
- `apps/backend/internal/mcp/server/server.go`
- New `apps/backend/internal/mcp/server/create_task_self_placement_test.go`
- New `apps/backend/internal/mcp/server/create_task_self_placement_integration_test.go`
- `apps/backend/internal/mcp/handlers/create_task_mode.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- New `apps/backend/internal/mcp/handlers/create_task_self_placement.go`
- New `apps/backend/internal/mcp/handlers/create_task_self_placement_test.go`
- New `apps/backend/internal/mcp/handlers/create_task_self_placement_admission_test.go`
- New `apps/backend/internal/mcp/handlers/create_task_self_placement_inheritance_test.go`
- New `apps/backend/internal/mcp/handlers/create_task_self_placement_outcomes_test.go`
- `docs/public/coordination.md`
- `docs/public/automation-and-mcp.md`
- `docs/specs/tasks/requirements/mcp-self-sibling-placement.md`
- `docs/specs/tasks/system-design/mcp-self-sibling-placement.md`
- `docs/decisions/2026-09-19-mcp-self-sibling-placement.md`
- `docs/plans/mcp-self-sibling-placement/plan.md`
- `docs/plans/mcp-self-sibling-placement/task-01-self-placement.md`

## Dependencies

None beyond approval and the existing task worktree/toolchain.

## Risks

See the plan's compatibility section. A marker is intent, never authorization.
Do not broaden access or change service behavior to make a test fixture pass.
If implementation requires a different public contract or ownership model,
return to the coordinator before editing that boundary.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/mcp-self-sibling-placement.md)
- [Design](../../specs/tasks/system-design/mcp-self-sibling-placement.md)
- [Decision](../../decisions/2026-09-19-mcp-self-sibling-placement.md)
- `create_task_mode_test.go`, `create_task_creator_session_test.go`,
  `create_task_external_id_test.go`, `create_task_atomicity_test.go`,
  `task_plan_safe_edits_integration_test.go`, and `service_child_task_test.go`.

## Results

Implemented on 2026-09-24 in `/workspace`, branch
`feature/issue-3773-define-de-8d9203`, base
`10726dfe8ea41240846db4a6d2ba26ebe9ce9c1a`. The user renewed implementation
authorization in this session. The 18 files listed above, including all five
package documents, remain uncommitted. No implementation commit hash,
push, PR, GitHub comment, task/session creation or live agent launch was produced.
The current coordinator owns this same checkout; no worker transfer is needed.

The recovered implementation was inspected before changes. Additional
behavioral red tests exposed four defects, then passed after the corresponding
fixes:

| Red evidence | Fix |
| --- | --- |
| Deduplicated response omitted the human-readable Kanban depth explanation | Include the depth limit while preserving the existing task's actual parent and no-creation wording |
| Parent repository failure returned `NOT_FOUND` instead of `INTERNAL_ERROR` | Preserve operational error classification and sanitize returned details |
| An ephemeral caller with a stored parent was redirected successfully | Reject the ephemeral caller before placement resolution |
| Releasing an external ID during synchronous workspace attachment still launched the created task | Settle against the service-normalized request identity instead of the refreshed task's now-empty identity |

The settlement correction applies to the shared MCP create path and preserves
the existing external-ID guarantee. It adds no public behavior beyond the
approved placement contract. No service depth or coordination control changed.
Tests added solely to verify existing contracts passed without production
changes; fixture/compilation corrections were not counted as behavioral red.

The acceptance evidence in the plan now covers invalid/inaccessible parents,
source authentication, parent scope/materialized workspace versus caller runtime,
ledger attribution, pending/settled deduplication with matching or different
parents, identity loss, rollback failures, dependency deferral and launch-once
behavior. The real dispatcher journey includes scope resolution and validates
serialized results, persistence, attribution, one launch, retry and explicit-ID
rejection. Existing direct-parent controls and Office allowance remain covered
by the full MCP packages and selected service/orchestrator regressions.

All required commands above passed on 2026-09-24:

- Full MCP packages: server 2.284s; handlers 18.404s.
- Selected service depth/reparenting tests: 0.287s.
- Selected orchestrator children-completion tests: 0.273s.
- Catalog validation: 295 decisions and 1068 specifications.
- Specification validator tests: 36 passed; all-spec lint passed.
- Public-doc validator tests: 62 passed; 47 published pages validated.
- `gofmt -l`: no output for all 11 changed Go files; `git diff --check` passed.

Additional checks:

```sh
(cd apps/backend && PATH=/usr/local/go/bin:$PATH go test -race -tags fts5 ./internal/mcp/server ./internal/mcp/handlers -run 'Test(CreateTaskSelfPlacement|MCPCreateTaskSelfPlacement)' -count=1)
(cd apps/backend && PATH=/usr/local/go/bin:$PATH golangci-lint run ./internal/mcp/server ./internal/mcp/handlers --new-from-patch=/tmp/mcp-self-sibling-lint.patch --timeout=5m)
```

The focused race tests passed: server 1.416s; handlers 5.054s. Changed-code lint
passed with zero issues. Its patch includes tracked changes
and every untracked Go file, using `git diff HEAD` plus `git diff --no-index`
from `/dev/null` for each new file.

Public documentation covers the effective parent's controls, creator profile
precedence, the additive result and truthful deduplication. No rendered UI,
mobile layout, screenshot, or Playwright change is needed. The five planning
documents' 2026-09-22 recovery provenance is retained in the plan; historical
verification claims are superseded by the checks recorded here.

No implementation blocker or unresolved design mismatch remains. The user
authorized opening the PR on 2026-09-26 and continued that request on
2026-09-27. Publication preflight confirmed all 18 files are present and the Go
diff exactly matches the patch validated above. This work order records local
implementation evidence; publication identifiers and commit hook results are
recorded in the Kandev task handoff. Merging remains a separate decision.
