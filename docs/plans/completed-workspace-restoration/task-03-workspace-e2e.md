---
id: "03-workspace-e2e"
title: "Prove completed workspace recovery"
status: complete
wave: 3
depends_on:
  - 02-workspace-feedback
plan: "plan.md"
requirements:
  - REQ-TASKS-COMPLETION-002
  - REQ-TASKS-COMPLETION-003
acceptance_criteria:
  - AC-TASKS-COMPLETION-002.1
  - AC-TASKS-COMPLETION-002.2
  - AC-TASKS-COMPLETION-002.3
  - AC-TASKS-COMPLETION-002.4
  - AC-TASKS-COMPLETION-003.1
  - AC-TASKS-COMPLETION-003.2
  - AC-TASKS-COMPLETION-003.3
  - AC-TASKS-COMPLETION-003.8
  - AC-TASKS-COMPLETION-003.9
  - AC-TASKS-COMPLETION-003.10
system_design:
  - ../../specs/tasks/system-design/task-completion.md
---

# Task 03: Prove completed workspace recovery

## Summary

Prove passive workspace access and failure recovery on desktop and phone using
the managed runtime. Verify subsequent explicit Resume still continues the
same conversation, and document the shipped distinction for users.

## In scope

- Create the two restoration specs named in the plan. Share seeding and
  controlled-error helpers in `completed-workspace-restoration-helpers.ts`.
- Seed a real checkout and conversation, mark it completed, then restart only
  the isolated worker backend to remove its live execution. Prove this
  precondition; do not let a warm fixture hide the terminal admission bug.
- Open through the shipped UI and read a known file, inspect Git state, and run
  a harmless shell command. Assert session/task state and agent-turn counts
  before any Resume, including after page reload.
- Inject one restore failure with existing WebSocket interception patterns,
  verify local Details/Retry and loading settlement, then restore through the
  real backend. Register transport waits before actions; restore fixture state.
- Extend both existing completed-session Resume specs to follow workspace
  restoration. Preserve their prior message-count, ownership, and task-state
  assertions. Include a restart between workspace restoration and explicit
  Resume in desktop coverage to exercise persisted provider identity.
- Exercise mobile Files navigation, viewer Back, Changes, and terminal controls
  with `.tap()`. Check actual 44 px action bounds, containment, keyboard access,
  no document overflow, and inspect a phone screenshot. Include desktop at its
  legal narrow width and breakpoint checks around the changed responsive rules.
- Update `docs/public/workflow-tips.md` with a short explanation of opening a
  completed workspace versus Resume. Link existing Git/terminal guidance rather
  than duplicate it. Synchronize actual results in this package and add a
  follow-up link in the existing task-completion package without rewriting its
  historical verification counts.

## Out of scope

Full E2E suites, generic QA/review tasks, live-instance mutation, screenshots of
the user's task, deployment, or CI/PR monitoring.

## Acceptance

- Both viewports show a working retained workspace with no agent startup or
  top-level restoration error; local failure/retry recovers through the backend.
- Explicit Resume afterward yields one follow-up in the original conversation
  and preserves completed task state, primary ownership, and session count.
- Rendered mobile evidence, exact test counts, docs/spec checks, and cleanup
  results are recorded; public instructions match the implemented behavior.

## Verification

Run the whole block from the repository root after Task 02's dependency install.
Managed E2E rebuilds production assets; run the two projects separately. Never
overlap full suites or override the memory-aware worker limits.

```bash
(cd apps/web && rtk pnpm e2e:run --project chromium tests/session/completed-workspace-restoration.spec.ts tests/session/completed-session-resume.spec.ts)
(cd apps/web && rtk pnpm e2e:run --project mobile-chrome tests/session/mobile-completed-workspace-restoration.spec.ts tests/session/mobile-completed-session-resume.spec.ts)
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
rtk python3 scripts/lint-spec-files.test.py
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
rtk git status --short -- docs/plans/completed-workspace-restoration
```

Confirm each project discovers the new and existing specs. Task 01 supplies the
initial behavioral RED; preserve that evidence. E2E after integration proves
the real cold runtime path, not a second selector-only RED.

## Files likely touched

- `apps/web/e2e/tests/session/completed-workspace-restoration.spec.ts` (new).
- `apps/web/e2e/tests/session/mobile-completed-workspace-restoration.spec.ts` (new).
- `apps/web/e2e/tests/session/completed-workspace-restoration-helpers.ts` (new).
- `apps/web/e2e/tests/session/completed-session-resume.spec.ts`.
- `apps/web/e2e/tests/session/mobile-completed-session-resume.spec.ts`.
- `apps/web/e2e/pages/session-page.ts` only for reusable shipped-control helpers.
- `docs/public/workflow-tips.md`.
- This plan/work orders and the companion task-completion plan links.

## Dependencies

Tasks 01 and 02. Follow `/e2e`, its fixture-state and UI-state references,
`/mobile-parity`, and `/docs-maintainer`.

## Risks

Fixture restart preserves data but changes runtime liveness. Require causal
backend evidence before navigation. Do not use `waitForChatIdle` recovery helpers
to prove passive behavior. Reset interception and dispose temporary checkout
files even after assertions fail. A skipped test or undiscovered project is not
passing evidence.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/task-completion.md), IDs above.
- [Design](../../specs/tasks/system-design/task-completion.md), workspace and mobile sections.
- Existing completed-session specs, `backend.restart`, session page objects,
  and `e2e/helpers/archived-session-recovery.ts` interception patterns.

## Results

Implemented cold-runtime desktop and mobile coverage, shared the retained
worktree fixture setup with the existing completed-session Resume specs, and
added public recovery guidance.

Verification passed:

- Chromium block discovered and passed 2/2 tests.
- Mobile Chromium block discovered and passed 2/2 tests.
- The mobile capture was inspected. Terminal output was readable, touch
  controls were contained, and the page had no horizontal overflow.
- Public docs validation passed 62 tests and 46 pages.
- Specification validation passed 36 tests and all specification files.
- `rtk git diff --check` passed.
- Temporary `.pr-assets` capture output was removed after inspection. No live
  task or persistent runtime state was changed.
