---
id: "03-removal-e2e"
title: "Prove removal transitions"
status: done
wave: 3
depends_on: ["02-departure-presentation"]
plan: "plan.md"
requirements:
  - REQ-TASKS-REMOVAL-NAVIGATION-001
  - REQ-TASKS-REMOVAL-NAVIGATION-002
acceptance_criteria:
  - AC-TASKS-REMOVAL-NAVIGATION-001.1
  - AC-TASKS-REMOVAL-NAVIGATION-001.2
  - AC-TASKS-REMOVAL-NAVIGATION-001.3
  - AC-TASKS-REMOVAL-NAVIGATION-001.4
  - AC-TASKS-REMOVAL-NAVIGATION-001.5
  - AC-TASKS-REMOVAL-NAVIGATION-002.1
  - AC-TASKS-REMOVAL-NAVIGATION-002.2
  - AC-TASKS-REMOVAL-NAVIGATION-002.3
  - AC-TASKS-REMOVAL-NAVIGATION-002.4
  - AC-TASKS-REMOVAL-NAVIGATION-002.5
system_design:
  - ../../specs/tasks/system-design/removal-navigation.md
---

# Task 03: Prove removal transitions

## Summary

Prove the entire removal interval on desktop and phone using controlled
transport ordering. Retain the existing destination, cascade, and confirmation
coverage while adding assertions that detect transient outgoing UI.

## In scope

- Execute the plan's E2E matrix with isolated managed backend fixtures.
- Add a mobile delete scenario file and shared observation/response-gate helpers
  only where multiple scenarios need them.
- Observe outgoing mounts and forbidden state changes from acceptance through
  departure; assert before releasing delayed responses.
- Verify no outgoing ensure request, no main-frame reload, matching URL/content,
  later navigation, preview closure, bulk partial failure, and failure restoration.
- Inspect a rendered phone status/destination and record focus and layout evidence.
- Record new results and synchronize this package's statuses after all checks pass.

## Out of scope

Broad E2E/full verification, generic QA/review, backend behavior changes,
rewriting historical companion-package test counts, commit/push/PR operations.

## Acceptance

- Both projects prove the outgoing task cannot display teardown/recovery UI
  during a pending request, including the last-task path.
- Failure, cascade, batch, preview, and later-navigation scenarios exercise user
  controls and assert intermediate as well as final state.
- All listed commands pass with discovered test counts and exact results recorded;
  every affected AC has executable evidence and the phone rendering is inspected.

## Verification

Run from the repository root, sequentially. Managed commands rebuild the product.
Do not overlap them or use additional worker overrides.

```bash
(cd apps/web && rtk pnpm e2e:run --project chromium tests/task/delete-task-redirect.spec.ts tests/task/archive-task-redirect.spec.ts tests/kanban/card-menu-delete-archive.spec.ts tests/task/sidebar-multi-select.spec.ts tests/task/archive-confirmation-preference.spec.ts)
(cd apps/web && rtk pnpm e2e:run --project mobile-chrome tests/task/mobile-delete-task-redirect.spec.ts tests/task/mobile-archive-task-redirect.spec.ts tests/task/mobile-archive-confirmation-preference.spec.ts tests/kanban/mobile-card-archive-confirmation.spec.ts)
rtk python3 scripts/lint-spec-files.test.py
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
rtk git status --short -- docs/plans/task-removal-navigation
```

Confirm the runner discovers every intended file; zero tests is not success.
If an implementation correction is needed, rerun its 01/02 checks and affected
E2E files after the final correction. Use the recorded component RED from 02
when the browser fixture cannot independently establish pre-fix ordering; explain
that limitation rather than claim a browser RED that was not run.

## Files likely touched

- All E2E files in the plan's scenario table
- `apps/web/e2e/tests/task/mobile-delete-task-redirect.spec.ts` (new)
- `apps/web/e2e/tests/task/removal-transition-helpers.ts` (proposed shared helper)
- `apps/web/e2e/pages/session-page.ts` and phone page helpers as needed
- This plan/work orders and paired spec lifecycle fields

## Dependencies

02 supplies rendered integration and its unit/component RED/GREEN evidence.

## Risks

Polling only for a final URL can pass despite the original flicker. Install
observation before the action and explicitly assert during controlled pending
HTTP and after correlated WS events. Release gates and dispose observers in
`finally`, including failed assertions. Avoid helper methods that silently reload
or resume a session to make the test pass.

## Parallelism

`sequential`

## Inputs

- Full plan E2E matrix and paired requirements.
- Existing archive/delete redirect and mobile archive specs.
- E2E skill, causal-wait helpers, session page objects, and mobile-parity guide.
- 01/02 recorded outcomes.

## Results

Done on 2026-09-10.

- Chromium removal matrix: 23 tests passed.
- Mobile Chromium removal matrix: 5 tests passed.
- Coverage includes active/last-task archive and delete, cascade exclusion,
  held mutation responses, outgoing DOM protection, preview actions, bulk
  selection, and desktop/mobile archive confirmation behavior.
- The delete page-object waits for the coordinator's localized success toast
  before starting a second removal; the held-response scenario opts out so it
  can inspect the pending interval.
- Specification tests: 36 passed; full specification lint passed.
- Targeted E2E-sleep lint for changed files and `rtk git diff --check` passed.
- The repository-wide E2E-sleep config still reports unrelated baseline errors;
  the changed E2E files are clean under the same config.
