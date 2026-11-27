---
id: "03-browser-regressions"
title: "Prove desktop and phone recovery"
status: complete
wave: 3
depends_on:
  - "02-dialog-recovery"
plan: "plan.md"
requirements:
  - REQ-TASKS-CREATE-RECOVERY-001
acceptance_criteria:
  - AC-TASKS-CREATE-RECOVERY-001.1
  - AC-TASKS-CREATE-RECOVERY-001.2
  - AC-TASKS-CREATE-RECOVERY-001.4
  - AC-TASKS-CREATE-RECOVERY-001.5
  - AC-TASKS-CREATE-RECOVERY-001.7
  - AC-TASKS-CREATE-RECOVERY-001.8
  - AC-TASKS-CREATE-RECOVERY-001.9
  - AC-TASKS-CREATE-RECOVERY-001.10
  - AC-TASKS-CREATE-RECOVERY-001.11
  - AC-TASKS-CREATE-RECOVERY-001.12
  - AC-TASKS-CREATE-RECOVERY-001.13
  - AC-TASKS-CREATE-RECOVERY-001.14
system_design:
  - ../../specs/tasks/system-design/task-create-recovery.md
---

# Task 03: Prove desktop and phone recovery

## Summary

Prove the repaired user flow against the real isolated backend and mock agent.
Cover lost responses, restored receipts, and phone reachability.
Document the shipped recovery actions after implementation passes.

## In scope

- Desktop and phone lost-response tests using `route.fetch()` before hiding the successful response.
- An explicit retry whose lookup temporarily misses, then whose POST reaches real backend idempotency with the same key.
- One backend task, one expected session, and one initial user prompt for start mode.
- Create-only recovery with no session or agent start.
- Incomplete lookup state without replacement, followed by read-only settlement observation.
- Close/reopen, same-tab reload, newer edits, and responsive viewport transitions.
- Scoped action visibility, phone bounding boxes, safe-area reachability, and document containment.
- Public recovery guidance in the existing task/workflow page.
- Actual work-order results and final requirement/design lifecycle reconciliation.

## Out of scope

- A new production recovery mechanism or changes to the user's live tasks.
- Full-suite E2E, a browser matrix unrelated to this regression, or global sleep conversions.
- Replacing server creation with a completely mocked success response.
- Publishing public docs during this planning turn or claiming unimplemented behavior is available.

## Acceptance

1. Desktop tests prove one retained native task through lost responses and explicit retries, including start and create-only modes.
2. Phone tests prove the same recovery plus visible actions, measured touch targets, internal scrolling, and preserved drafts across viewport changes.
3. Public guidance and all package results match actual behavior and task-defined verification. Every requirement criterion has its named unit, handler, or E2E evidence.

## ASCII UI preview

Validate [UI-01 and UI-02](plan.md#ascii-ui-preview) through the actual dialog.

```text
UI-01 phone
[Scrolling draft and status]
[Check status    ]
[Retry submission]
[Close           ]
[Bottom safe area]

UI-02 phone
[Existing task creation is incomplete]
[Open task       ]
[Check status    ]
[Close           ]
```

The primary actions must remain reachable with a reduced viewport.
Match the plan's desktop actions and the existing full-height phone composition.
Map these checks to criteria .5, .7, .8, .12, and .13.
Capture a focused phone image through the existing E2E capture fixture and inspect it.
Do not equate a CSS class or visible label with a measured 44px target.

## Test method

Read `/e2e` and its fixture guidance. Use the existing isolated `testPage` and `apiClient` fixtures.
Reuse `KanbanPage`, `MobileKanbanPage`, and the description-editor helper.
Both new specs belong under `e2e/tests/task/`.
Seed a one-step workflow without a completion-triggered move, then select it explicitly.
This fixture makes the expected single opening turn deterministic.

For the core scenario:

1. Arm a native POST route before clicking Start task.
2. Forward the request to the real backend and retain its actual task ID.
3. Replace the browser response with a failure after that backend create completes.
4. Prove the recovery status and captured draft remain visible.
5. Make lookup return a temporary miss without treating it as permission for a fresh identity.
6. Explicitly retry. Forward its POST unchanged to the backend.
7. Assert the same external ID and task ID, plus backend task/session/prompt counts.

Use fixture API readers, including `listTaskSessions` and `listSessionMessages`, for backend evidence.
Use the task-list HTTP route or existing workflow snapshot reader to count fixture-owned tasks.
Filter fixture-owned records only. Do not count unrelated seeded tasks.
For incomplete UI, intercept only the lookup projection of the real matching task.
Restore its genuine complete projection before the next Check status action.
The test must observe no extra creation or launch during those reads.

Mount/reload tests must observe requests across restoration and assert no automatic POST.
Use causal waits armed before each action. Do not add raw sleeps or all-worker overrides.
Run desktop and phone commands sequentially. The managed runner rebuilds production assets.

## Verification

Run from the repository root after task 02 passes.

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/task/task-create-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-create-recovery.spec.ts)
(cd apps/web && pnpm exec eslint e2e/tests/task/task-create-recovery.spec.ts e2e/tests/task/mobile-task-create-recovery.spec.ts e2e/tests/task/task-create-recovery-helpers.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/task-create-recovery docs/public/tasks-and-workflows.md
```

Verify that each E2E command discovers the intended project and nonzero tests.
Record actual scenario counts, relevant backend counts, and image paths in Results.
Rerun only a product check affected by a new failure or change.
Run the exact [package coverage preflight](plan.md#package-coverage-preflight) from the repository root.
Its complete shell block is part of this work order's verification.
Also evaluate `validateCoverage` against final actual changed paths and the same linked artifacts.

## Files likely touched

- `apps/web/e2e/tests/task/task-create-recovery.spec.ts` (new)
- `apps/web/e2e/tests/task/mobile-task-create-recovery.spec.ts` (new)
- `apps/web/e2e/tests/task/task-create-recovery-helpers.ts` (new, shared fixture helpers)
- `docs/public/tasks-and-workflows.md`, creation and recovery sections only.
- This plan and its three work orders, for actual results and accurate statuses.
- The paired requirement and design frontmatter, only after all criteria and task checks pass.

The new recovery section is a how-to guide within the existing public page.
Keep its current structure and omit technical receipt or identity details.
Internal implementation names do not belong in the product recovery flow.
No new docs page, navigation entry, screenshot catalog, or landing-repository change is needed.

## Dependencies

[Task 02](task-02-dialog-recovery.md). The native recovery flow must pass its unit and component tests first.
Task 02's behavior-specific RED evidence remains the regression gate. This task supplies rendered integration evidence.

## Risks

- Failing before `route.fetch()` creates the task does not reproduce the incident.
- A request-count assertion can miss two tasks, two sessions, or a duplicated prompt.
- A completely mocked accepted task cannot prove backend deduplication.
- Wrong Playwright project selection can discover no tests.
- A stale Vite build can verify the baseline instead of the implementation.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/task-create-recovery.md).
- [System design](../../specs/tasks/system-design/task-create-recovery.md), recovery surface and verification sections.
- [Plan E2E matrix](plan.md#e2e-tests) and [UI previews](plan.md#ascii-ui-preview).
- `e2e/tests/chat/quick-chat-startup-acceptance.spec.ts`: real-create/lost-response test precedent.
- `e2e/tests/task/mobile-task-create-escape.spec.ts`: native phone dialog entry and focus precedent.
- `/docs-maintainer`, `/simple-english`, and `docs/public/README.md` before editing public copy.

## Results

Completed on 2026-10-28.

- Desktop Chromium: 5 scenarios passed. They cover a real backend create with a lost browser response and same-key retry, reload restoration without a create, incomplete reporting followed by read-only settlement, create-only retry without an opening prompt, and the creation-pending route suppressing automatic session ensure.
- Phone `mobile-chrome`: 1 scenario passed. It covers lost-response recovery, stacked recovery actions with measured targets of at least 44px, status placement above the footer, document containment, reload restoration, same-key retry, and Open task handoff.
- The desktop lost-response scenario observed one backend task, one session, and one opening prompt. The reload and incomplete scenarios observed one task and one session without another create POST or launch.
- The creation-pending regression seeded a zero-session task, opened it with the read-only marker, observed no session ensure or start, then proved the same task without the marker attempts ensure.
- Updated `docs/public/tasks-and-workflows.md` with recovery guidance. No new page or navigation entry was added.
- `node --test scripts/validate-public-docs.test.mjs`: 63 passed.
- `node scripts/validate-public-docs.mjs`: 54 pages validated.
- `python3 scripts/list-docs.py validate`: 409 decisions and 1765 specifications validated.
- `python3 scripts/lint-spec-files.test.py` and `python3 scripts/lint-spec-files.py --all`: passed.
- The package coverage preflight: covered, all three work orders.
- `git diff --check`: passed.
