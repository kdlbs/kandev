---
id: "01-honor-new-session-policy"
title: "Honor same-profile new-session policy"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.2
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.4
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.5
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.11
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 01: Honor same-profile new-session policy

## Summary

Apply the destination start policy before returning a matching-profile session.
Keep profile-ID comparison and preserve overrides whenever reuse is selected.

## In scope

- Add the four backend regressions named in the plan; run the `new` case RED
  before changing production code. Test both source end policies and seed an
  extra matching session to prove `new` does not choose it.
- Correct preparation and credential preflight with the same policy decision.
- Exercise `processOnEnter` prompt recipient and primary-session promotion.
- Add the topbar E2E case from the plan using disposable mock-agent fixtures.
- Record actual targeted results and update this work order and plan status.

## Out of scope

Model-based routing, runtime override resets, no-profile fallback changes,
explicit-target redesign, UI changes, database migrations, and live-task edits.

## Acceptance

1. Matching nonempty profiles plus `new` create a distinct conversation using
   the selected profile and existing environment. Source `park` and `complete`
   outcomes remain correct. The replacement receives the step prompt.
2. Matching profiles plus explicit/default `reuse` retain the same session and
   runtime overrides. Different-profile and explicit-target tests still pass.
3. Credential/preparation failure leaves the source recoverable without a
   destination prompt. The topbar scenario persists the new primary on reload.

## Verification

Run from the repository root. Install workspace dependencies once if absent.
The focused Go commands are used because the Make test target runs all packages.

```bash
(cd apps && rtk pnpm install --frozen-lockfile)
(cd apps/backend && rtk go test ./internal/orchestrator -run '^TestPrepareWorkflowStepSession_SameProfileNew$' -count=1)
(cd apps/backend && rtk go test -race ./internal/orchestrator -run 'TestPrepareWorkflowStepSession|TestPreflightWorkflowStepCredentials|TestProcessOnEnter|TestSwitchSessionForStep|TestWorkflowSessionTarget' -count=1)
(cd apps/web && rtk pnpm e2e:run --project chromium tests/workflow/workflow-session-targeting.spec.ts)
rtk proxy python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

First run the new focused test before the fix and record the expected assertion
failure. Run it again after the fix, then the remaining checks. Use a fresh
managed E2E build. Do not treat missing tests or fixture failure as RED evidence.
Use causal waits and existing session/page helpers; do not add timed sleeps.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_same_profile_policy_test.go` (new)
- `apps/web/e2e/tests/workflow/workflow-session-targeting.spec.ts`
- This package's plan and work order for status and results.

## Dependencies

None. Work is sequential in the primary session after an explicit implementation request.

## Risks

Use the existing profile-switch fixtures and failure injection. Avoid treating
a source model override as another profile. Keep source-step validation and
the replacement-before-retirement order intact.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/workflow-profile-session-lifecycle.md), criteria listed above.
- [Design](../../specs/tasks/system-design/workflow-profile-session-lifecycle.md), Control flow and Failure and recovery.
- `event_handlers_workflow_profile_session_policy_test.go`: `newProfileSwitchFixture` and independent start/end policy tests.
- `event_handlers_workflow_profile_test.go`: existing same-profile reuse test.
- `workflow_session_target.go`: explicit-target compatibility path.
- [Plan](plan.md): root cause, test names, topbar scenario, and exclusions.

## Results

Implemented the policy-aware profile-only routing path. The preparation and
credential preflight paths now resolve the destination start policy before
their same-profile shortcut. Matching profiles preserve the current session
only for normalized `reuse` or an empty profile; matching `new` uses the
existing replacement lifecycle and the source end policy. Runtime model and
reasoning overrides remain untouched.

Added backend regressions for same-profile `new` with both source end policies,
explicit/default `reuse` override preservation, credential preflight failure,
and `processOnEnter` prompt delivery to the replacement. Added the Chromium
topbar E2E regression with API identity checks and reload persistence.

TDD and verification results:

- RED focused test: 3 table cases failed before the production change because
  the same-profile shortcut returned `switched=false`.
- GREEN focused policy tests: 8 passed.
- Race-filtered orchestrator tests: 99 passed with no race reports.
- Backend build and lint: passed; lint reported 0 issues.
- Workflow session-targeting Chromium E2E: 5 passed after the final backend
  build and E2E plugin packaging.
- Web typecheck, specification lint, Go formatting, targeted Prettier check,
  and `git diff --check`: passed.
