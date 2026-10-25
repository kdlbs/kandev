---
id: "04-e2e-sidebar-badge"
title: "E2E: sidebar MR badge"
status: pending
wave: 2
depends_on: ["01-sidebar-mr-badge"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/gitlab-mr-task-list-badges.md"
---

# Task 04: E2E for the sidebar MR badge

New spec `apps/web/e2e/tests/gitlab/mr-sidebar-badge.spec.ts`, plus a
row-and-badge accessor on the existing sidebar page object.

E2E is **required**: `apps/web/**` is touched and none of the files are on the
Testing step's exemption allowlist.

## Scenarios

All three run on `/`, where `components/kanban-board.tsx:321` mounts the
unconditional `useWorkspaceMRs` owner. Neither this route nor task 05's mounts
`TaskPageContent`, so neither is exposed to the two hydration holes the spec
records and accepts.

- **E1 — Sidebar shows the badge.** A task with one linked open MR. Scoped to
  `app-sidebar`, row located by `[data-task-row-id="<taskId>"]`,
  `mr-task-icon-<taskId>` is visible with `data-mr-count="1"` and
  `data-mr-state="open"`. (AC1, AC19)
- **E2 — Sidebar shows no badge without an MR.** Same route, task with no linked
  MR. Inside `app-sidebar`, `mr-task-icon-<taskId>` has count 0. (AC6)
- **E3 — Sidebar shows PR before MR.** A task with both a mock GitHub PR and a
  linked MR; inside the sidebar row, both badges visible and PR precedes MR by
  `compareDocumentPosition`. (AC2)

## Four things that are easy to get wrong here

**1. Seed tasks with NO agent.** `createTaskWithAgent` auto-starts a session, the
Kanban template's start step carries `on_turn_complete: move_to_step review`, and
the card leaves the start column the instant the mock agent's turn ends — which
detaches elements mid-assertion and burns the full timeout. The badge is a pure
function of the task's linked MRs, so an agent turn adds nothing.
`mr-task-card-badge.spec.ts`'s `seedBoardTask` comment records this at length.

**2. Copy the seed helpers; do not import them.** `nextMRIID`, `seedMR`,
`linkMR`, `ensureGitLabConfigured` and `seedBoardTask` are file-local and
**unexported** in `e2e/tests/gitlab/mr-task-card-badge.spec.ts`. Define local
copies seeded from that file's versions. That file is **not** edited to export
them, which is what keeps AC16 ("passes unmodified") literally true, and it
matches the in-repo precedent: `mobile-mr-task-card-badge.spec.ts` already writes
its own `seedBoardTaskWithMR` and imports only constants.

Shared **constants** are different: import `GITLAB_HOST` and `GITLAB_PROJECT`
from `e2e/helpers/gitlab.ts`, as the existing specs do. Do not re-declare them.

**Lint consequence.** `sonarjs/no-identical-functions` sits in the **global**
block of `apps/web/eslint.config.mjs` (no `files`, no `ignores`), so it **is live
on `e2e/**`**. The `e2e/**` entry in an `ignores` array belongs to the i18n guard
block, and the `files: ["e2e/**/*.ts"]` override disables `react-hooks/*`,
`max-lines`, `max-lines-per-function` and `sonarjs/no-duplicate-string` — but
**not** `no-identical-functions`. It cannot fire across files, because ESLint sees
one file at a time. So: copy helpers across files freely; **never write two copies
of the same helper inside one file**, because `pnpm --filter @kandev/web lint`
runs `eslint --max-warnings 0`.

**3. E3's GitHub PR comes from the association call, not the catalogue call.**
Use `apiClient.mockGitHubReset()`, then `apiClient.mockGitHubSetUser("test-user")`,
then `apiClient.mockGitHubAssociateTaskPR({ task_id, owner: "testorg",
repo: "testrepo", pr_number, pr_url, pr_title, head_branch, base_branch,
author_login: "test-user", state: "open" })`. `state` is optional and is passed
explicitly so the seeded PR's state is not left implicit.

**`mockGitHubAddPRs()` is the wrong call and must not be used.** It posts to
`POST /api/v1/github/mock/prs`, which seeds the mock provider's PR *catalogue*
(what pickers and listings read) and creates **no task-to-PR association**. A
scenario using it leaves `taskPRs.byTaskId[taskId]` empty, so `PRTaskIcon` renders
nothing and E3's PR-before-MR assertion can never run.
`mockGitHubAssociateTaskPR()` posts to `POST /api/v1/github/mock/task-prs` and is
what populates the association the badge reads; no catalogue call is needed
alongside it.

The reference is `mr-task-card-badge.spec.ts`'s own test
`"AC30/AC37: a linked PR and MR both render, PR before MR, and the badge tooltip
names a non-open state"`. Reading that file does not modify it, so AC16 stays
literally true.

**4. Scope every `mr-task-icon-*` accessor to a container, and never use
`.first()`.** One task id can be emitted by two mounted rows on one route
(sidebar plus board). Scope to `app-sidebar`, `tasks-list-row`, or
`task-card-<id>`. `.first()` papers over the duplicate instead of resolving it.
The identical hazard already exists for `pr-task-icon-*` and is recorded in
`e2e/tests/pr/pr-status-badge.spec.ts`. (AC19)

## Page object

Extend `apps/web/e2e/pages/app-sidebar-page.ts` with a row-and-badge accessor.
That class is already anchored at `page.getByTestId("app-sidebar")`, which is
the container AC19 requires. **Do not add a parallel sidebar page object.**
`apps/web/e2e/pages/sidebar-tasks-page.ts` is the shape to follow: it exposes a
`row(taskId)` accessor that scopes into `sidebar-task-item` and is careful about
which sidebar root it anchors to.

Note the sidebar row's own id attribute is `data-task-row-id` (set on the
`sidebar-task-item` element in `task-item.tsx`), which is what E1 locates by.

## Acceptance

1. `apps/web/e2e/tests/gitlab/mr-sidebar-badge.spec.ts` exists and E1, E2 and E3
   pass.
2. `apps/web/e2e/pages/app-sidebar-page.ts` gains the row-and-badge accessor; no
   new sidebar page object is created.
3. `e2e/tests/gitlab/mr-task-card-badge.spec.ts` is **unmodified** and passes
   (AC16); no file under `apps/web/components/github/` is modified (AC14); and
   `apps/web/components/gitlab/mr-task-icon.tsx` is unmodified (AC15).

## Verification

```
cd apps && pnpm install --frozen-lockfile \
  && cd web && pnpm e2e:raw -- e2e/tests/gitlab/mr-sidebar-badge.spec.ts \
  && pnpm e2e:raw -- e2e/tests/gitlab/mr-task-card-badge.spec.ts
```

The second command is the AC16 non-regression check and must pass with that file
untouched. Then:

```
cd apps && pnpm --filter @kandev/web lint
```

`--max-warnings 0`, so a duplicated helper **within** the new spec file fails the
build.

## Files likely touched

- `apps/web/e2e/tests/gitlab/mr-sidebar-badge.spec.ts` (new)
- `apps/web/e2e/pages/app-sidebar-page.ts`

Explicitly **not** touched: `e2e/tests/gitlab/mr-task-card-badge.spec.ts`,
`e2e/tests/pr/pr-status-badge.spec.ts`, `e2e/helpers/gitlab.ts` (imported from,
not edited).

## Dependencies

Task 01. The sidebar badge must render before these scenarios can pass.

## Parallelism

`parallel-safe` with task 05 once both their dependencies have landed: disjoint
spec files, disjoint page objects.

## Inputs

- Spec: "E2E (Playwright, required)" in full, including "Where the seed helpers
  come from" and "Where the mock GitHub PR comes from"; "Accessibility and
  duplicate mounts"; AC1, AC2, AC6, AC16, AC19
- Plan: E2E Tests section
- Patterns: `e2e/tests/gitlab/mr-task-card-badge.spec.ts` (read, do not edit),
  `e2e/tests/gitlab/mobile-mr-task-card-badge.spec.ts` (the local-copy
  precedent), `e2e/pages/sidebar-tasks-page.ts` (page-object shape)

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
