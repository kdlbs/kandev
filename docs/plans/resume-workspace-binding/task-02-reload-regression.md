---
id: "02-reload-regression"
title: "Verify Changes after recovery and reload"
status: done
wave: 2
depends_on: ["01-persist-resume-binding"]
plan: "plan.md"
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.5
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.2
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.27
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.28
system_design:
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
  - ../../specs/tasks/system-design/environment-owned-git-status.md
  - ../../specs/platform/system-design/workspace-git-status.md
---

# Task 02: Verify Changes after recovery and reload

## Summary

Prove that a recovered task supplies fresh changed-file membership after reload.
Exercise the real backend on desktop and phone while retaining source validation.

## In scope

- Add the two isolated browser regressions described in the plan.
- Capture and correlate real `session.git.refresh` responses for the recovered session.
- Assert dirty-file visibility after reload without a later repository mutation.
- Assert preserved environment identity, worktree identity, and file contents.
- Use native phone Changes navigation and the shipped file-selection surface.
- Run existing negative source-validation tests without relaxing their assertions.

## Out of scope

New controls, copy, layout, status ordering, polling, successful response stubs,
real container/remote executor certification, and mutations of the user's instance.

## Acceptance

- Desktop and phone show the target changed file after recovery and reload through a real fresh response.
- Missing, mismatched, removed, and replaced Git sources retain their existing rejection behavior.
- No runtime workaround changes the workspace or manufactures clean status.

## Verification

Run from the repository root. If `apps/node_modules` is absent, first run
`pnpm --dir apps install --frozen-lockfile`. Run browser projects sequentially.

```bash
(cd apps/backend && go test -race -tags fts5 ./internal/backendapp -run 'TestAppendLiveGitStatusMessage.*(Canonical|Mismatched|Unrecorded|Unverified)|TestSessionGitRefresh.*(RegisteredRepoWorkspace|RemovedDuringRequest|ReplacedSource)' -count=1)
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/git/changes-panel-resume-workspace-binding.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/git/mobile-changes-panel-resume-workspace-binding.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Record each project's discovered test count and result. The managed runner owns
builds and teardown. No overlapping full suite or all-worker override is permitted.

## Files likely touched

- `apps/web/e2e/tests/git/changes-panel-resume-workspace-binding.spec.ts` (new)
- `apps/web/e2e/tests/git/mobile-changes-panel-resume-workspace-binding.spec.ts` (new)
- `apps/web/e2e/tests/git/resume-workspace-binding-helpers.ts` (only if both specs need a shared fixture)
- `docs/plans/resume-workspace-binding/plan.md` and work-order Results

## Dependencies

Task 01.

## Risks

Existing cached status can hide the defect. Verify after a real page reload.
Fixture seeding must target the owned task/session and database by exact identity.
Do not infer a successful fresh response from a later stream event.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md), E2E tests and compatibility matrix.
- [Task design](../../specs/tasks/system-design/additional-session-workspace-reuse.md).
- [Git ownership design](../../specs/tasks/system-design/environment-owned-git-status.md).
- [Progressive Git design](../../specs/platform/system-design/workspace-git-status.md).
- Existing `mobile-changes-panel-refresh-recovery.spec.ts`, Git helpers, session helpers, and isolated backend fixture.
- `/e2e` and `/mobile-parity` guidance.

## Results

Passed:

- Negative Git source validation race tests in `internal/backendapp` for
  canonical, mismatched, unrecorded, unverified, removed, and replaced sources.
- Desktop Chromium regression: 1 test passed. It restores the raw session
  binding, reloads the task, and correlates the dirty file with a real fresh
  `session.git.refresh` response.
- Phone `mobile-chrome` regression: 1 test passed. It uses touch navigation,
  selects the dirty file, and verifies the full-height diff sheet and layout.
- Specification catalog validation and full spec lint.
- `pnpm --dir apps/web typecheck`, `pnpm --dir apps/web build:vite`, and
  `git diff --check`.

The mobile fixture disables automatic resume on open so the regression invokes
the manual recovery action explicitly, then restores the setting during cleanup.

### PR review follow-up (2026-10-02)

The fixture now removes its dirty file, resets its disposable task, and restores
the auto-resume setting if any seeding step fails. The reload helper waits for
the authoritative backend `FAILED` state before reloading and uses Playwright's
default enabled-state timeout. Stable test IDs identify the Git-status Retry
control and phone Changes navigation button.

Passed after these changes:

- Desktop Chromium reload regression: 1 test passed.
- Phone `mobile-chrome` reload regression: 1 test passed.
- Web typecheck, changed-file ESLint, Prettier, and production Vite build through
  the managed E2E runs.
