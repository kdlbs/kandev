---
created: 2026-09-11
status: complete
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
legacy_specs: []
---

# Implementation Plan: Same-profile fresh workflow sessions

## Overview

Honor the destination's `new` policy when its resolved profile matches the
active session. One sequential work order corrects routing and verifies the
manual-move outcome. The task system owns this change because it selects the
conversation that receives workflow entry actions.

## Evidence and requirement conformance

At checkout `407ed4f1a5`, `prepareWorkflowStepSession` returns the active session
when profile IDs match, before it reads `ProfileSessionStartPolicy`.
`preflightWorkflowStepCredentials` has the same shortcut. This violates
[AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.2 and 001.5](../../specs/tasks/requirements/workflow-profile-session-lifecycle.md).
The existing requirement and design already specify the correction; their draft
status also covers a broader recipient feature and is not changed by this repair.

The reported live task is compatibility evidence, not a reproduction of this
defect. Its saved destination policy was `reuse`. Its session retained the Luna
profile ID after a user selected Astra as a runtime override. Reusing that
session was correct. Profile identity remains the routing comparison; runtime
model and reasoning overrides do not trigger a switch or a reset.

Minimal reproduction: create a task session with profile A, then enter a second
step with profile A and start policy `new`. The current shortcut returns the
original session instead of a fresh conversation. Use a focused service test
for RED; no failing test has been executed during package creation.

## Scope

### In scope

- Profile-only entry with the same nonempty resolved profile and `new`.
- Consistent credential preflight and source `park`/`complete` handling.
- Same-profile `reuse` and default reuse preserving runtime overrides.
- Entry prompt delivery to the replacement and its promotion to primary.

### Out of scope

- Comparing effective models, resetting overrides, or changing agent profiles.
- Changing empty-profile fallback, initial creation, or explicit recipient rules.
- New settings, UI markup, translations, schema, APIs, or Office behavior.
- Reworking routing persistence or unrelated retry behavior.

## Technical approach

In `apps/backend/internal/orchestrator/event_handlers_workflow.go`, resolve the
destination start policy before the same-profile return in
`prepareWorkflowStepSession`. Keep the empty-profile fallback. Keep a matching
profile only for normalized `reuse`; send matching-profile `new` through
`switchSessionForStepWithPolicies` with the existing source end policy.
Apply the same decision in `preflightWorkflowStepCredentials` so replacement
validation cannot be skipped. Do not add model comparisons.

Use the existing replacement, promotion, queue transfer, source-binding, and
stamped runtime-stop paths. Preserve the explicit-target branch in
`workflow_session_target.go`. No source session may be retired before a valid
destination is prepared. A replacement keeps the task environment and executor
inheritance, and starts its own provider conversation.

## Tests

Add `event_handlers_workflow_same_profile_policy_test.go` beside the existing
profile-policy fixtures. Planned test names and evidence:

| Test | Evidence | Criteria |
| --- | --- | --- |
| `TestPrepareWorkflowStepSession_SameProfileNew` | Distinct session with profile A, no reuse of another matching session; park/complete table cases | 001.2, 001.4, 001.5 |
| `TestPrepareWorkflowStepSession_SameProfileReusePreservesOverrides` | Same ID and metadata for explicit/default reuse, even with another model override | 001.5 |
| `TestPreflightWorkflowStepCredentials_SameProfileNew` | Credential rejection preserves source and prevents destination prompt | 001.11 |
| `TestProcessOnEnter_SameProfileNewPromptRecipient` | Entry actions reach the fresh primary; source has no destination prompt | 001.2, 001.5 |

Use `newProfileSwitchFixture` and real repository session rows. Capture launch
and prompt recipients rather than asserting only that a helper was called.
Retain existing different-profile and explicit-target regressions.

## E2E tests

Extend `apps/web/e2e/tests/workflow/workflow-session-targeting.spec.ts` in project
`chromium` with `same-profile new session from topbar`. Seed a disposable
workflow using the same profile for source and destination, destination `new`,
source `park`, and a unique destination prompt marker. Move through the task
topbar. Assert a distinct primary session, visible replacement conversation,
and marker delivery to that conversation. Reload and confirm the primary
selection persists. Use API state to corroborate identities and source parking.
This covers 001.2, 001.4, and 001.5 through the reported entry point.

No UI structure changes are planned, so no UI preview is required. Existing
mobile recipient tests remain compatibility evidence; this package changes
the shared backend policy rather than phone interaction.

## Work orders

- [x] [Task 01: Honor same-profile new-session policy](task-01-honor-new-session-policy.md)

## Related delivery records

[Workflow session targeting](../workflow-session-targeting/plan.md) delivered
explicit initial and source-step recipients. Its completed results remain
historical evidence, not proof of this profile-only case. This package owns
the missing case and does not reopen its eight work orders.

## Verification results

Implementation and package validation on 2026-09-11:

- RED: `go test ./internal/orchestrator -run '^TestPrepareWorkflowStepSession_SameProfileNew$' -count=1` failed all three table cases before the production change because the matching-profile shortcut returned `switched=false`.
- GREEN: the focused same-profile preparation, reuse, credential preflight, and entry-prompt tests passed 8 cases.
- `go test -race ./internal/orchestrator -run 'TestPrepareWorkflowStepSession|TestPreflightWorkflowStepCredentials|TestProcessOnEnter|TestSwitchSessionForStep|TestWorkflowSessionTarget' -count=1`: 99 passed with no race reports.
- `make -C apps/backend build`: passed.
- `make -C apps/backend lint`: passed with 0 issues.
- `pnpm e2e:run --project chromium tests/workflow/workflow-session-targeting.spec.ts`: 5 passed with a fresh managed build.
- `pnpm e2e:run --no-build --project chromium tests/workflow/workflow-session-targeting.spec.ts`: 5 passed after refreshing the E2E plugin artifact.
- `pnpm run typecheck`: passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- `gofmt -l` and the targeted Prettier check: passed.

## Risks

- A policy-blind preflight could skip credential validation for new sessions.
- Broader equality changes could reset user overrides or alter explicit targets.
- Wrong source-step propagation could complete a session intended to be parked.
- A topbar assertion based only on labels could miss wrong-session delivery.

## Documentation impact

Public workflow documentation already says `new` always starts a fresh
conversation. No public copy change is needed. Reconcile the older ADR's
same-profile exception with the existing requirement; do not change defaults
or unrelated historical decisions in this package.
