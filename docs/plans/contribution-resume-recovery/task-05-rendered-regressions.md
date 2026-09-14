---
id: "05-rendered-regressions"
title: "Prove recovery flows in the browser"
status: completed
wave: 5
depends_on:
  - "04-recovery-presentation"
plan: "plan.md"
requirements:
  - REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-002
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
acceptance_criteria:
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-002.1
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-002.2
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-002.5
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.11
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005.1
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.1
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.2
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.3
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.4
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.5
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.6
system_design:
  - ../../specs/tasks/system-design/remote-contribution-tasks.md
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
  - ../../specs/agents/system-design/session-recovery-failures.md
---

# Task 05: Prove recovery flows in the browser

## Summary

Extend existing desktop/mobile recovery suites with the package scenarios and rendered geometry. Reuse shared fixtures and one disposable real-Git contribution scenario. Update public recovery guidance after behavior is implemented.

## In scope

Extend existing desktop/mobile recovery suites with the package scenarios and rendered geometry. Reuse shared fixtures and one disposable real-Git contribution scenario. Update public recovery guidance after behavior is implemented.

## Out of scope

Broad QA audits, full E2E suite, production data, new isolated platform tasks, and replacing previous archive/branch-loss coverage.

## Acceptance

- Rendered flows prove one owner, safe labeled details, valid retry, workspace-only fallback, and persistence; history-only contribution resume reaches an active agent with unchanged local/remote work.
- Desktop keyboard and phone touch paths satisfy UI-01 through UI-04, including translated labels, 28px desktop and >=44px touch controls, viewport containment, and one scroll owner.
- Existing archive, branch-loss, fresh-start, and launch-card scenarios pass; public guidance describes only verified behavior.

## Regression and verification

For the Git regression, use isolated local repositories: record local HEAD and dirty file contents, advance remote from a second clone, resume through the UI, and compare both versions afterward. Test behind and diverged variants where fixture support permits; backend tests cover both regardless. Seed deterministic typed blocking errors for UI-only scenarios. Assert delayed request count, exact selected session, no duplicates after reload, dual operation labels, unrelated history, and no stale result after navigation. Run phone geometry at the configured device plus 767/768px boundaries without overriding mobile device identity. Save a screenshot for each changed composition.

Run from the repository root. Use the listed test names for new regressions.
If any existing test is changed beyond this list, add its exact command here
before marking results complete.

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/session/session-resume-recovery.spec.ts tests/task/launch-failure-recovery.spec.ts tests/task/archived-session-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-resume-recovery.spec.ts tests/task/mobile-launch-failure-recovery.spec.ts tests/task/mobile-archived-session-recovery.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/e2e/helpers/session-resume-recovery.ts`
- `apps/web/e2e/helpers/archived-session-recovery.ts`
- `apps/web/e2e/tests/session/session-resume-recovery.spec.ts`
- `apps/web/e2e/tests/session/mobile-session-resume-recovery.spec.ts`
- `apps/web/e2e/tests/task/launch-failure-recovery.spec.ts`
- `apps/web/e2e/tests/task/mobile-launch-failure-recovery.spec.ts`
- `docs/public/tasks-and-workflows.md`
- `docs/plans/contribution-resume-recovery/plan.md`

## Dependencies

[Task 04](task-04-recovery-presentation.md).
Execution is sequential.

## Inputs

- [Package evidence and design](plan.md).
- [System design](../../specs/tasks/system-design/remote-contribution-tasks.md).
- [System design](../../specs/tasks/system-design/task-launch-failure-recovery.md).
- [System design](../../specs/agents/system-design/session-recovery-failures.md).
- Applicable REQ and AC identifiers in frontmatter.
- Read scoped AGENTS.md and the existing adjacent tests before implementation.

## Risks

Managed E2E builds must be fresh. Do not use store-injected failures as proof of backend Git admission. Record exact discovered and passed counts; do not copy counts from earlier packages.

## Parallelism

`sequential`

## Results

Completed. Desktop and mobile browser coverage proves the correlated card,
safe details, action visibility, persistence, touch geometry, and absence of
duplicate outer recovery surfaces. Existing resume, archive, branch-loss, and
launch-failure cases remain covered.

Verification passed:

- `pnpm e2e:run --project chromium tests/session/session-resume-recovery.spec.ts tests/task/launch-failure-recovery.spec.ts tests/task/archived-session-recovery.spec.ts`: 8 passed.
- `pnpm e2e:run --project mobile-chrome tests/session/mobile-session-resume-recovery.spec.ts tests/task/mobile-launch-failure-recovery.spec.ts tests/task/mobile-archived-session-recovery.spec.ts`: 7 passed.
- `node --test scripts/validate-public-docs.test.mjs`: 62 passed.
- `node scripts/validate-public-docs.mjs`: 46 published docs validated.
- `python3 scripts/lint-spec-files.test.py`: 36 passed.
- `python3 scripts/lint-spec-files.py --all` and `git diff --check` passed.

Review remediation reran the changed browser files with fresh backend and
pseudo-locale artifacts:

- `pnpm e2e:run --host --no-build --project chromium tests/task/launch-failure-recovery.spec.ts`: 4 passed.
- `pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-launch-failure-recovery.spec.ts`: 3 passed.
