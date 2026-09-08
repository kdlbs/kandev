---
id: "05-e2e-tasks-list-badge"
title: "E2E: /tasks list MR badge, desktop and mobile"
status: pending
wave: 2
depends_on: ["02-tasks-row-mr-badge", "03-tasks-page-mr-hydration"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/gitlab-mr-task-list-badges.md"
---

# Task 05: E2E for the `/tasks` list MR badge

Two new specs: `apps/web/e2e/tests/gitlab/mr-tasks-list-badge.spec.ts` (E4 to E6)
and `apps/web/e2e/tests/gitlab/mobile-mr-tasks-list-badge.spec.ts` (E7).

E2E is **required**: `apps/web/**` is touched and none of the files are on the
Testing step's exemption allowlist.

## Scenarios

All four run on `/tasks`, whose unconditional `useWorkspaceMRs` owner task 03
adds. The route does not mount `TaskPageContent`, so none of these is exposed to
the two hydration holes the spec records and accepts.

`mr-tasks-list-badge.spec.ts`:

- **E4 — Rich row shows the badge.** Navigate to `/tasks`, enable "Show task
  details", locate the `tasks-list-row`, assert `mr-task-icon-<taskId>` inside
  that row. (AC8, AC19)
- **E5 — Compact row shows neither badge.** Same task, details off: inside the
  row, both `pr-task-icon-<taskId>` and `mr-task-icon-<taskId>` have count 0.
  (AC10)
- **E6 — Rich row shows PR before MR.** Task with both; inside the row, PR
  precedes MR by `compareDocumentPosition`. (AC9)

`mobile-mr-tasks-list-badge.spec.ts`:

- **E7 — Mobile row.** Mobile viewport, details on, task with both badges: the MR
  badge is visible and `assertNoDocumentHorizontalOverflow` passes. Mirrors
  `e2e/tests/gitlab/mobile-mr-task-card-badge.spec.ts`. (AC20)

## Five things that are easy to get wrong here

**1. `tasks-list-row` carries no task-id attribute.** The element at
`apps/web/app/tasks/tasks-list-view.tsx:333` has `data-testid="tasks-list-row"`
and `data-level`, but **no per-task id**. The only per-row identifier is the
title, via `data-testid="tasks-list-row-title"`. Locate the row by filtering on
its title text. Do not go looking for a `data-task-id` on this row; there is none,
and its absence is not a bug to fix here.

**2. Toggling "Show task details" goes through the display menu.** Follow
`e2e/tests/**/task-listing-view-preferences.spec.ts`:

```ts
await page.getByTestId("display-button").click();
await page.getByText("Show task details", { exact: true }).click();
```

**3. Seed tasks with NO agent**, for the reason `mr-task-card-badge.spec.ts`'s
`seedBoardTask` comment records: `createTaskWithAgent` auto-starts a session and
the task transitions out from under the assertion. The badge is a pure function of
the task's linked MRs.

**4. Copy the seed helpers; do not import them.** `nextMRIID`, `seedMR`,
`linkMR`, `ensureGitLabConfigured` and `seedBoardTask` are file-local and
**unexported** in `mr-task-card-badge.spec.ts`. Each of the two new spec files
defines its own local copies. That file is **not** edited to export them, which
keeps AC16 ("passes unmodified") literally true, and it matches
`mobile-mr-task-card-badge.spec.ts`'s existing precedent.

Import `GITLAB_HOST` and `GITLAB_PROJECT` from `e2e/helpers/gitlab.ts`; do not
re-declare those constants.

**Lint consequence.** `sonarjs/no-identical-functions` sits in the **global**
block of `apps/web/eslint.config.mjs` (no `files`, no `ignores`), so it **is live
on `e2e/**`** — the `e2e/**` entry in an `ignores` array belongs to the i18n guard
block, and the `files: ["e2e/**/*.ts"]` override disables `max-lines` and
`sonarjs/no-duplicate-string` but **not** `no-identical-functions`. It cannot fire
across files, because ESLint sees one file at a time. Copy helpers across the two
new files freely; **never write two copies of the same helper inside one file**,
because `pnpm --filter @kandev/web lint` runs `eslint --max-warnings 0`.

**5. E6 and E7's GitHub PR comes from the association call, not the catalogue
call.** Use `apiClient.mockGitHubReset()`, then
`apiClient.mockGitHubSetUser("test-user")`, then
`apiClient.mockGitHubAssociateTaskPR({ task_id, owner: "testorg",
repo: "testrepo", pr_number, pr_url, pr_title, head_branch, base_branch,
author_login: "test-user", state: "open" })` — `state` is optional and is passed
explicitly.

**`mockGitHubAddPRs()` is the wrong call and must not be used.** It seeds the mock
provider's PR *catalogue* and creates **no task-to-PR association**, so
`taskPRs.byTaskId[taskId]` stays empty, `PRTaskIcon` renders nothing, and the
PR-before-MR assertion that is the point of E6 and E7 can never run.
`mockGitHubAssociateTaskPR()` populates the association the badge reads; no
catalogue call is needed alongside it.

The reference is `mr-task-card-badge.spec.ts`'s own `"AC30/AC37"` test. Reading it
does not modify it, so AC16 stays literally true.

**And, as in task 04: scope every `mr-task-icon-*` accessor to a container and
never use `.first()`.** One task id can be emitted by two mounted rows on one
route — on `/tasks` that is the sidebar row plus the list row. Scope to
`app-sidebar`, `tasks-list-row`, or `task-card-<id>`. (AC19)

## Acceptance

1. `apps/web/e2e/tests/gitlab/mr-tasks-list-badge.spec.ts` exists and E4, E5 and
   E6 pass.
2. `apps/web/e2e/tests/gitlab/mobile-mr-tasks-list-badge.spec.ts` exists and E7
   passes, including `assertNoDocumentHorizontalOverflow`. (AC20)
3. `e2e/tests/gitlab/mr-task-card-badge.spec.ts` and
   `e2e/tests/gitlab/mobile-mr-task-card-badge.spec.ts` are **unmodified** and
   pass (AC16).

## Verification

```
cd apps && pnpm install --frozen-lockfile \
  && cd web && pnpm e2e:raw -- e2e/tests/gitlab/mr-tasks-list-badge.spec.ts \
  && pnpm e2e:raw -- e2e/tests/gitlab/mobile-mr-tasks-list-badge.spec.ts \
  && pnpm e2e:raw -- e2e/tests/gitlab/mr-task-card-badge.spec.ts \
  && pnpm e2e:raw -- e2e/tests/gitlab/mobile-mr-task-card-badge.spec.ts
```

The last two are the AC16 non-regression checks and must pass with those files
untouched. Then:

```
cd apps && pnpm --filter @kandev/web lint
```

`--max-warnings 0`, so a duplicated helper **within** either new spec file fails
the build.

## Files likely touched

- `apps/web/e2e/tests/gitlab/mr-tasks-list-badge.spec.ts` (new)
- `apps/web/e2e/tests/gitlab/mobile-mr-tasks-list-badge.spec.ts` (new)

Explicitly **not** touched: `e2e/tests/gitlab/mr-task-card-badge.spec.ts`,
`e2e/tests/gitlab/mobile-mr-task-card-badge.spec.ts`,
`e2e/tests/pr/pr-status-badge.spec.ts`, `e2e/helpers/gitlab.ts` (imported from,
not edited).

## Dependencies

Tasks 02 and 03. The `/tasks` row must render the badge (02) and the page must
hydrate the MRs (03) before any of these scenarios can pass.

## Parallelism

`parallel-safe` with task 04 once both their dependencies have landed: disjoint
spec files, disjoint page objects.

## Inputs

- Spec: "E2E (Playwright, required)" in full, including "Where the seed helpers
  come from" and "Where the mock GitHub PR comes from"; "Responsive and
  coarse-pointer behaviour"; "Accessibility and duplicate mounts"; AC8, AC9,
  AC10, AC16, AC19, AC20
- Plan: E2E Tests section
- Patterns: `e2e/tests/gitlab/mr-task-card-badge.spec.ts` and
  `e2e/tests/gitlab/mobile-mr-task-card-badge.spec.ts` (read, do not edit),
  `task-listing-view-preferences.spec.ts` (the display-menu toggle)
- `MRTaskIcon`'s Radix tooltip is inherited unchanged on both new surfaces: no
  `useTouchDrawer`, no `useHoverPopover`, no tap-to-open handling is added, on
  mobile or anywhere else. Upgrading that disclosure is its own card.

## Output contract

Report: summary; files changed (reconciled against the actual diff, **including
any existing spec modified as E2E evidence**); tests run with exact commands and
pass/fail counts; blockers; risks. Set this task's `status` to `in_progress` at
start and `done` at finish, update `## Results` below, and synchronize the Wave 2
checkbox and `## Verification Results` in `plan.md`.

## Results

Pending. Before marking this task done, replace this with every exact command
actually run and its outcome/count, generated artifact paths, and cleanup or
teardown evidence (including temporary capture-spec removal and
`git diff --check` when used). Record security/trust and external side-effect
boundaries when applicable, or explicitly state `None`.
