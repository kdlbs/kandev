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
- apps/web/e2e/tests/chat/mobile-chat-status-bar.spec.ts
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

## Test-only CI geometry follow-up

The controller authorized a bounded mobile test synchronization candidate after
CI attempts at unchanged backend code failed at two geometry assertions. No
production UI, hover behavior, launch policy or backend code changes are part
of this follow-up. Public documentation has no additional impact.

The old phone helper naturally failed with nine indexed button locators after
the optional Jump to latest control disappeared, leaving eight current buttons.
Diagnostics retained its identity, viewport and current bounds. The installed
Playwright implementation also selects the array before a separate awaited
`evaluateAll` callback. A controlled real transcript scroll between these two
phases reproduced a disconnected Jump to latest node measuring zero on both
base d5387f0 and CI merge 0bd1474; fresh current controls were connected and at
least 44 pixels. The precise zero-width control from CI remains unidentified.

Both local helpers now acquire current descendants, parent bounds and notice
geometry in one browser callback, then poll the complete invariant set using
the existing default assertion timeout. Required named controls must be unique
and rendered. No zero-size required target is filtered out. The 44-pixel
minimums, containment tolerances, overlap limit, notice order/row, viewport
coverage and existing named-control/action scenarios remain enforced. Failures
include control identity, connected/rendered state, viewport and sampled bounds.
No test retry, timeout, sleep, skip or product behavior change was added.

Verification in the CI runtime image uses one worker, 2 CPUs and 4 GiB memory,
with CI profiles and strict WebSocket accounting. Test-only probes are restored
before the final unchanged scenario runs:

- Natural old phone helper: 7 passed, 1 expected timeout across four repeats of
  the two failing scenarios, retries disabled (1 CPU pressure).
- Controlled old sampling: expected zero-width assertion failure on base and CI
  merge. Corrected current-snapshot helper passes the same lifecycle boundary.
- Sustained zero-width, hidden, overlapping and duplicate required Share controls
  each fail the corrected helper. Positive boundary case passes; four expected
  mutant failures and one pass in 54.4 seconds on base, retries disabled.
- Final base full file: 6 passed in 1.0 minute, retries disabled.
- Final CI merge full file: 6 passed in 59.7 seconds. Two focused scenarios
  repeated four times under the same 1-CPU pressure as natural RED: 8 passed
  in 2.7 minutes, retries disabled.
- Current-main synthetic merge (21949f6 + approved PR head): 6 passed in
  59.0 seconds, retries disabled, after rebuilding production web assets.
- Scoped ESLint passes without warnings. Formatting, documentation coverage,
  catalog/specification and whitespace validation pass.

Exact browser commands inside the isolated runtime image, from `apps/web`:

```sh
pnpm e2e:raw --project=mobile-chrome e2e/tests/chat/mobile-chat-status-bar.spec.ts --retries=0 --trace=on --reporter=line
pnpm e2e:raw --project=mobile-chrome e2e/tests/chat/mobile-chat-status-bar.spec.ts --grep 'wraps status and transcript controls|gives the terminal notice' --repeat-each=4 --retries=0 --trace=on --reporter=line
```

The controller approved the exact three-file candidate and current-main
integration after independently passing the full mobile file (6/6, 1.0 minute)
in the same constrained CI image with retries disabled. Normal hook commit,
exact-lease push and new-head CI/review are authorized. The old-head failed gate
is historical evidence and cannot authorize merge; all required checks and
semantic reviews must complete for the new head. The preexisting hover
dismissal finding remains deferred.
