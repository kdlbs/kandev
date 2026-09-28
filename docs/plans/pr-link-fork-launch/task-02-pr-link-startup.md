---
id: "02-pr-link-startup"
title: "Prove browser PR-link startup"
status: done
wave: 2
depends_on:
  - "01-attachment-identity"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
acceptance_criteria:
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.11
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.15
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.16
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.17
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.18
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.19
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
---

# Task 02: Prove browser PR-link startup

## Summary

Prove that the existing desktop and phone task-creation flows launch a fork PR.
Use the normal browser payload and backend preparation, without preseeded
contribution or comparison bindings.

## In scope

- Extend `create-task-github-url.spec.ts` with a test named
  `starts a target-attached fork PR from its URL`.
- Extend `mobile-create-task-remote-repo.spec.ts` with the same scenario.
  Reuse the phone remote picker and touch submission patterns from that file.
- Extract shared Git/provider fixture setup into
  `pr-link-fork-launch-helpers.ts` in the same directory.
- Create disposable upstream and fork repositories. Give same-named base
  branches different commits. Publish the fork head in upstream `refs/pull/N/head`.
  Configure provider metadata with distinct head and target repository identities.
- Paste the upstream PR URL through the existing UI, select the worktree
  executor, and submit. Assert agent output, exact checkout head, target base,
  and the visible PR association. Reload and assert retained branch/association.
- Confirm that the fixture does not insert `RemoteContribution` or
  `ComparisonTarget` metadata. The browser must use its ordinary creation path.
- Preserve existing local-executor, same-repository, and missing-PR-ref scenarios.
  Restore fixture changes and remove owned repositories even after assertions fail.

## Out of scope

New UI layout, controls, copy, translation keys, page objects unrelated to this
flow, live-instance testing, and broad E2E runs.

## Acceptance

1. Desktop creation launches the mock agent from the fork head and target base.
   Reload retains the selected branch and PR association.
2. Phone creation proves the same outcome through touch-accessible controls.
   Existing layout, navigation, and scroll ownership remain unchanged.
3. Both focused suites pass with freshly built assets. Existing scenarios remain
   present. Results identify the discovered and executed test counts.

## Verification

Run from the repository root. Install dependencies once in a fresh worktree.
Managed E2E commands build and tear down their own isolated instances.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --project chromium tests/task/create-task-github-url.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-create-task-remote-repo.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Task 01 owns the deterministic pre-fix RED test. Browser tests verify the same
regression through production builds after that correction. Do not claim RED
from a missing selector, unavailable binary, or failed fixture setup.

## Files likely touched

- `apps/web/e2e/tests/task/create-task-github-url.spec.ts`
- `apps/web/e2e/tests/task/mobile-create-task-remote-repo.spec.ts`
- `apps/web/e2e/tests/task/pr-link-fork-launch-helpers.ts` (new).
- `docs/plans/pr-link-fork-launch/plan.md` and these work-order result sections.

## Dependencies

Task 01. Its backend correction must pass its targeted commands first.

## Risks

Existing mock PR defaults can accidentally make the head and target identical.
A contribution-binding fixture would bypass the failed path. Shared Git changes
can leak into later tests unless fixtures own and remove their temporary paths.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/workspaces/requirements/worktree-base-refresh.md), .11 and .15-.19.
- [Design clarification](../../specs/workspaces/system-design/worktree-base-refresh.md#ordinary-pr-link-launch-compatibility).
- Existing worktree PR launch in `create-task-github-url.spec.ts`.
- Existing phone remote creation in `mobile-create-task-remote-repo.spec.ts`.
- `.agents/skills/e2e/SKILL.md` and `.agents/skills/mobile-parity/SKILL.md`.

## Results

Completed. The fixture uses disposable upstream and fork repositories with
different `main` commits, publishes the fork head under the upstream PR ref,
and seeds distinct provider head/target identities. The ordinary creation
request contains neither `remote_contribution` nor `comparison_target`.

Commands and results:

- `(cd apps/web && pnpm e2e:run --project chromium tests/task/create-task-github-url.spec.ts)`: 10 passed.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-create-task-remote-repo.spec.ts)`: 7 passed.
- Both suites used freshly built backend and pseudo-locale Vite assets. The mock agent launched from the expected checkout branch. Terminal checks verified the full fork `HEAD` and upstream target `main` commit OIDs.
- After reload, both flows retained `checkout_branch`, `base_branch`, PR summary number, and exact persisted PR association. The desktop flow also displays `#3879` in the session PR topbar. The phone flow uses touch submission and opens the created task from its mobile task card.
- Cleanup is in `finally` paths and removes the task, test repository, and temporary upstream/fork Git directories.
- `make -C apps/backend build` and web `pnpm run typecheck`: passed. Targeted ESLint for both changed specs and the shared fixture: passed.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py` (36 tests), `python3 scripts/lint-spec-files.py --all`, and `git diff --check`: passed.
- Additional broad `pnpm run lint:e2e-sleeps` check: failed on existing errors outside the changed E2E files, including unsanctioned waits in unrelated tests and unresolved rule references. Targeted ESLint for changed files passes.
