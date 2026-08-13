---
id: "06-e2e-disposition"
title: "E2E coverage for recording a disposition"
status: done
wave: 5
depends_on: ["05-frontend-disposition-control"]
plan: "plan.md"
spec: "../../specs/pr-outcome-attribution/spec.md"
---

# Task 06: E2E coverage for recording a disposition

One Playwright spec covering the user-visible surface end to end against the
mock GitHub provider.

- **Acceptance:**
  1. GIVEN a task with a closed, unmerged PR, WHEN the user opens the CI popover
     and records `superseded` with a valid superseding PR URL, THEN the control
     shows the recorded value and still does after `page.reload()`.
  2. GIVEN a task with a merged PR, WHEN the user opens the CI popover, THEN the
     disposition control is absent.
  3. The spec passes in the default (non-container) Playwright project and
     leaves no seeded state behind for sibling specs.

- **Verification:**
  ```
  cd apps && pnpm install --frozen-lockfile
  cd apps/web && pnpm e2e:raw -- e2e/tests/pr/pr-disposition.spec.ts
  ```

- **Files likely touched:**
  - `apps/web/e2e/tests/pr/pr-disposition.spec.ts` (new)
  - `apps/backend/internal/github/mock_controller.go` — only if the merged
    fixture turns out to need an explicit `merged_at`; add the field to
    `associateTaskPRRequest` and `buildTaskPRFromRequest` rather than weakening
    the UI's visibility gate.

- **Dependencies:** task 05.
- **Parallelism:** sequential — it exercises the surface task 05 builds.

- **Inputs:**
  - Spec: "Verification surfaces", which marks E2E required for this change, and
    AC-31 through AC-34 for the observable outcomes.
  - Plan: "E2E Tests".
  - Patterns: `apps/web/e2e/tests/pr/pr-multi-popover.spec.ts` and
    `pr-topbar-popover.spec.ts` for opening the popover and asserting on its
    testids; seeding via `POST /api/v1/github/mock/task-prs`
    (`mock_controller.go:56`), which already accepts `state`. A `closed` row
    leaves `merged_at` nil and a `merged` row is excluded by the state check, so
    both fixtures should be reachable without extending the mock request.
  - Follow `apps/web/e2e/README.md` for project selection and fixture
    conventions, and the `e2e` skill for the TDD loop.

- **Output contract:** summary of the scenarios covered; files changed; the
  exact Playwright command and its pass count; confirmation that any temporary
  capture spec was removed and `git diff --check` is clean; blockers; risks;
  status update in this file and `plan.md`.

## Results

**Status: done.** Both scenarios implemented and pass. Mock task-PR seeding
via `POST /api/v1/github/mock/task-prs` needed no extension — confirmed by
reading `mock_controller.go`'s `buildTaskPRFromRequest`: it never sets
`MergedAt`, so `state: "closed"` yields `merged_at: nil` for the
closed-unmerged fixture, and `state: "merged"` is excluded by the frontend's
`pr.state !== "closed"` gate regardless of `merged_at` — no field addition
needed to `associateTaskPRRequest`.

**Operational finding worth recording:** `pnpm e2e:raw -- <spec path>`
(the exact form given in this task file's own Verification section and in
`e2e/README.md`'s per-project examples) does **not** scope Playwright to the
named spec when pnpm's `--` argument separator is combined with Playwright's
own CLI parsing — a first invocation with that exact form silently ran the
**entire 2202-test suite** instead (confirmed by capturing its live output
mid-run: `Running 2202 tests`, executing unrelated `office-routing-*`,
`auth-*`, `automations-*`, `chat/*` specs). Killed after ~140 tests once
diagnosed. The working form drops the redundant `--` and calls the local
binary directly:
```
node_modules/.bin/playwright test --config e2e/playwright.config.ts e2e/tests/pr/pr-disposition.spec.ts
```
Verified the fix first with `--list` (`Total: 2 tests in 1 file`) before
spending another real run on it. Not fixed in this task — flagged here
rather than in `docs/plans/` prose only, since the task-file/README command
form is what a future run (human or agent) will copy-paste. No leftover
`kandev-e2e-*` temp directories remained after the runaway run was killed
(Playwright's own teardown handled it); no manual cleanup was required.

**Files changed:** `e2e/tests/pr/pr-disposition.spec.ts` (new). No
`mock_controller.go` change needed.

**Commands run:**
```
cd apps && pnpm install --frozen-lockfile
cd .. && make build-backend                    # backend binary for e2e global-setup
make build-web-e2e                             # QA-locale SPA build for e2e global-setup
make -C apps/backend e2e-plugin-package         # fixture plugin package for e2e global-setup
cd apps/web
node_modules/.bin/playwright test --config e2e/playwright.config.ts e2e/tests/pr/pr-disposition.spec.ts --list
  # Total: 2 tests in 1 file
node_modules/.bin/playwright test --config e2e/playwright.config.ts e2e/tests/pr/pr-disposition.spec.ts
  # 2 passed (24.5s)
```

**Cleanup/teardown evidence:** no `kandev-e2e-*` directories remain under
the OS temp dir after the run; `git status` shows only the new spec file
under `e2e/tests/pr/` — no other e2e artifacts left in the working tree.

**Security/trust and external side effects:** None. Runs entirely against
the mock GitHub provider and a local ephemeral backend/database; no real
GitHub API calls.
