---
id: 01-reuse-prepared-primary
title: Reuse prepared primary on dependency resolution
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-TASKS-TASK-DEPENDENCIES-001
acceptance_criteria:
  - AC-TASKS-TASK-DEPENDENCIES-001.1
system_design:
  - ../../specs/tasks/system-design/task-dependencies.md
  - ../../specs/tasks/system-design/deferred-start-step-entry.md
---

# Reuse prepared primary on dependency resolution

## Outcome and scope

One eligible prepared primary becomes the running dependency recipient and
continues to be selected by MCP moves. Ordinary starts still create sessions.
Reject ineligible prepared conversations and active sibling ownership. Preserve
workflow routing, Office identity, launch intent and durable incarnation.

## Owned files

- apps/backend/internal/orchestrator/event_handlers_workflow.go
- apps/backend/internal/orchestrator/session_launch.go
- apps/backend/internal/orchestrator/task_operations.go
- apps/backend/internal/orchestrator/dependency_prepared_session.go
- apps/backend/internal/orchestrator/dependency_prepared_session_test.go
- apps/backend/internal/mcp/handlers/dependency_prepared_move_test.go
- docs/specs/tasks/system-design/task-dependencies.md
- docs/specs/tasks/system-design/deferred-start-step-entry.md

## Acceptance

1. TestDependencyResolutionReusesPreparedPrimary fails on current main and passes
   with one unchanged primary/session incarnation after dependency resolution.
2. Ineligible prepared sessions remain untouched, workflow selectors remain
   authoritative, ordinary starts retain creation behavior.
3. Relevant MCP move checks continue to defer the active primary and preserve
   task/service authorization.

## Verification

From apps/backend, run focused Go tests with -trimpath and -race for dependency
reuse, auto-start dependency gate, explicit workflow routing and MCP moves.
Run scripts/lint-go-changed on changed Go files from the repository root.
Run python3 scripts/list-docs.py validate and
python3 scripts/lint-spec-files.py --all.
Record exact commands and results below before parent validation.

## Results

RED reproduced both failures on the unchanged initial integration base. The final
candidate passed the bounded targeted race check (orchestrator 6.684s, MCP
handlers 5.379s), scoped lint (zero issues), catalog/specification validation
and whitespace checks. Implementation is done; controller validation passed and
delivery is authorized. CI/review and protected merge remain pending.
Exact commands, RED/GREEN evidence, integration base and limits are recorded in
[the plan](plan.md#verification-results).

## Parent validation follow-up

Production Go files remain byte-for-byte identical to the initial candidate.
Added actual full-ceiling cases to the existing consumer matrix (20 scenarios)
and MCP integration (unlimited/full). Both confirm existing manual admission,
no replay row, consumed dependency intent, the same primary/incarnation and
the authorized deferred MCP move. Focused `-trimpath -race` checks pass:
orchestrator 2.430s; MCP handlers 2.049s. The proposed ceiling-replay blocker
was withdrawn after its assumed automatic admission proved inapplicable.
No speculative replay plumbing or admission-policy change was made.

## PR documentation coverage remediation

The trusted PR validator rejected the changed historical task-dependencies
work order's legacy `spec` frontmatter, which local catalog/spec lint does not
validate. Reproduced the exact 12-file PR set with the unchanged base-controlled
`validateCoverage`: RED, `frontmatter field spec is unsupported`. Updated only
the historical task-03 and sibling plan metadata to the current requirements,
acceptance-criteria and system-design links, retaining their existing content
and status. No source/test or product behavior change. GREEN: the same validator
accepts both changed work orders; catalog/spec/whitespace checks also pass.
The check exercised `validateCoverage` from `.github/scripts/pr-docs.cjs`
against the exact changed-file set and linked document contents. At that
remediation checkpoint, exact-head CI/review and protected merge were pending.

## Review documentation cleanup

Historical validation and pre-approval statements in the plan are now explicitly
labelled as checkpoints. External receipt paths were replaced by self-contained
command/result summaries. Prior-head CI passed 82 checks and skipped 13; final
head checks, review disposition and protected merge remain tracked externally.
No source/test changes were needed for the review findings.
