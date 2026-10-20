---
created: 2026-10-20
status: done
requirements:
  - REQ-TASKS-TASK-DEPENDENCIES-001
system_design:
  - ../../specs/tasks/system-design/task-dependencies.md
  - ../../specs/tasks/system-design/deferred-start-step-entry.md
legacy_specs: []
---

# Implementation Plan: Dependency prepared session reuse

## Overview

Restore the single-session launch contract for issue #4579. Opening a blocked
task prepares a primary session, but dependency resolution currently issues an
unbound start that creates another session. MCP moves then inspect the empty
primary and fail the service's active-sibling guard.

The user authorized a bounded regression repair without a separate planning
handoff. Parent validation of the uncommitted candidate precedes delivery.

## Scope

Reuse an eligible prepared primary for a recorded start-when-unblocked launch.
Keep workflow recipient and effective-profile selection authoritative. Preserve
ordinary explicit starts and existing MCP move authorization. Exclude failed
session revival, queue retargeting, primary reassignment and checkout refresh
policy (#4582).

## Technical approach

Thread an internal prepared-primary reuse option from deferred launch through
session launch to startTask. Select only after workflow recipient/profile
resolution. Hold the existing session lifecycle lock across eligibility checks
and launch. Reject conversation evidence, mismatched profile/executor, and
active siblings; retain existing launch and durable-delivery admission.

## Tests

AC-TASKS-TASK-DEPENDENCIES-001.1: dependency_prepared_session_test.go drives
blocked LaunchSession preparation and dependency resolution through the real
executor and SQL repository. Add ineligibility and ordinary-start controls.
dependency_prepared_move_test.go additionally drives the real task dependency
store and predecessor event through orchestrator launch to the MCP move handler
and task-service validation. Existing MCP move controls remain applicable.

## E2E tests

Backend consumer/wiring evidence covers this backend-only repair. No browser
or live developer instance is needed; no rendered UI changes.

## Work orders

- [x] [Task 01: Reuse prepared primary](task-01-reuse-prepared-primary.md)

## Verification results

The machine crashed during a pre-crash final race run and its `/tmp` evidence
was lost. Those earlier timings are historical, not final-candidate proof.
Fresh evidence is stored outside the repository in the owning task directory:
`../4579-validation/` (relative to this repository's root).

### Fresh RED on current main

A disposable detached checkout at
`74427cb50c24e281e5b3fa826c7136ffed5be5db` received only the two regression
test files. Production code was unchanged; the candidate was never overwritten.
The checkout was removed after the checks.

From its `apps/backend` directory:

```sh
go test -trimpath ./internal/orchestrator -run '^TestDependencyResolutionReusesPreparedPrimary/eligible$' -count=1
go test -trimpath ./internal/mcp/handlers -run '^TestMoveTaskAfterDependencyPreparedLaunch$' -count=1
```

Both exited 1 for the expected behavioral assertions: two sessions instead of
one (0.658s), and MCP CONFLICT because the task has an active sibling (0.396s).
Logs: `red-orchestrator.log` and `red-mcp.log` in the evidence directory.

### Fresh GREEN on the preserved candidate

From `apps/backend`, with bounded build parallelism:

```sh
GOMAXPROCS=2 GOMEMLIMIT=3GiB go test -p=1 -trimpath -race ./internal/orchestrator ./internal/mcp/handlers -run 'Test(DependencyResolution|LaunchSession_AutoStartDependencyGate|LaunchSession_ProfileExplicit|ResolutionLaunchesOnly|GateFiringMidLaunch|TheLaunchIntentIsClaimed|ManualStartConsumes|StartCreatedSessionConsumes|FailedLaunchReleases|MoveTaskAfterDependencyPreparedLaunch|HandleMoveTask_ActiveSessionWithoutPrompt|DeferMoveTask_|MoveTaskErrorMessage_)' -count=1
```

PASS: orchestrator 6.684s; MCP handlers 5.379s. The prepared-session matrix
contains 19 cases. Log: `final-race.log` in the evidence directory.

### Other checks

Scoped Go lint: PASS, zero issues (`lint.log`). Exact command from the repository root:

```sh
bash scripts/lint-go-changed apps/backend/internal/orchestrator/dependency_prepared_session.go apps/backend/internal/orchestrator/dependency_prepared_session_test.go apps/backend/internal/orchestrator/event_handlers_workflow.go apps/backend/internal/orchestrator/session_launch.go apps/backend/internal/orchestrator/task_operations.go apps/backend/internal/mcp/handlers/dependency_prepared_move_test.go
```

Documentation checks: PASS, 394 decisions and 1681 specifications validated;
all specification files passed (`docs.log`). Commands from the repository root:

```sh
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

`git diff --check`: PASS.

The integration uses the real task dependency store, task turn service, event
watcher, orchestrator, executor, MCP handler, task service and authoritative
memory queue. It verifies the pending move's session incarnation and an active
turn while workflow placement remains unchanged. Provider processes are faked.
No browser, live developer instance, real provider, or PostgreSQL run is claimed.
No schema or public wire changes. Existing stranded or ineligible conversations
are not retroactively reassigned or revived. No commit, push, PR or issue reply
is authorized before the controller validates this candidate.

## Risks

Prepared workspace execution is legitimate and may already have a runtime row;
provider conversation, turns, and messages are not workspace-only evidence.
Explicit workflow targets and Office identity must retain their own selectors.

## Documentation impact

The docs-maintainer check found no new user command, setting, API, or UI
contract. This repair restores the dependency launch contract; technical
selection details remain in the owning system design. Public coordination
documentation was checked without expanding the dependency product surface.

## Integration base

Initial reproduction base: 7914b4110baaa68a5633eec3204233a0c9875a65.
Advanced base: 74427cb50c24e281e5b3fa826c7136ffed5be5db (#4638), a disjoint
web routing/doc change, fast-forwarded before final checks.

## Delivery checkpoint

Implementation and task-defined verification are complete. The controller
independently validated the candidate, including the full-ceiling consumer
cases, and authorized commit, push, PR review and protected merge. CI/review
and actual merge remain delivery gates; the exact public issue reply requires
separate parent approval after merge verification. No task/session/agent was
created during this repair.

## Parent validation follow-up: full ceiling

The controller's initial replay concern assumed the dependency consumer used
automatic ceiling admission. Investigation showed `launchDeferredTask` leaves
`AutoStart` and `ActivationSource` unset; `launchStart` derives `autoStart=false`
and `startTask` derives manual origin. The existing ceiling admits a manual
override at capacity. Parent independently confirmed this and withdrew the
hypothetical replay blocker. No reachable production caller currently combines
the internal reuse flag with automatic ceiling deferral. No payload/replay
plumbing or admission-policy change is included.

The initial exploratory test expected a deferred row and timed out because the
consumer launched instead. This was a disproven reproduction assumption, not
RED proof for a production change. The exploratory cases were removed. The
original causal RED logs remain valid; production Go hashes are unchanged from
the first candidate independently checked by the parent.

Added `full_ceiling` to the prepared-primary consumer matrix and MCP integration.
Both occupy the only ceiling slot before resolving the dependency. Assertions
check the same primary/incarnation, a recorded manual override, consumed dependency
intent and no ceiling-deferred record. The MCP case uses an actual persisted
running session for population and verifies the same authorized pending move,
active turn and unchanged workflow placement as the ordinary case. The unit
matrix now has 20 cases and the MCP test has unlimited/full-ceiling cases.

Final focused command, from `apps/backend`:

```sh
GOMAXPROCS=2 GOMEMLIMIT=3GiB go test -p=1 -trimpath -race ./internal/orchestrator ./internal/mcp/handlers -run '^(TestDependencyResolutionReusesPreparedPrimary|TestMoveTaskAfterDependencyPreparedLaunch)$' -count=1
```

PASS: orchestrator 2.430s; MCP handlers 2.049s. Log:
`../4579-validation/full-ceiling-race.log`. The earlier broader
focused GREEN and parent's independent repeat remain evidence for unchanged
production logic; this command covers the necessary new consumer test inputs.

Final scoped lint: PASS, zero issues (`lint-full-ceiling.log`), using the exact
six-file command above. Final catalog/spec lint and whitespace checks: PASS
(`docs-full-ceiling.log`). Current main remains
`74427cb50c24e281e5b3fa826c7136ffed5be5db`; issue OPEN with no comments/linked
closing PR and no PR for this branch. Final candidate remains uncommitted for
parent validation. Initial review artifacts are preserved separately under
`../4579-validation/initial-candidate/`.

## PR documentation coverage remediation

The trusted PR validator rejected the changed historical task-dependencies
work order's legacy `spec` frontmatter, which local catalog/spec lint does not
validate. Reproduced the exact 12-file PR set with the unchanged base-controlled
`validateCoverage`: RED, `frontmatter field spec is unsupported`. Updated only
the historical task-03 and sibling plan metadata to the current requirements,
acceptance-criteria and system-design links, retaining their existing content
and status. No source/test or product behavior change. GREEN: the same validator
accepts both changed work orders; catalog/spec/whitespace checks also pass.
Reproduction command from repo root: `node ../4579-validation/check-docs-coverage.cjs`.
Exact-head CI/review and protected merge remain pending after the fixup push.
