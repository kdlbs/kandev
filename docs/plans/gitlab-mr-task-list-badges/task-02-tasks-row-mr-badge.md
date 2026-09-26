---
id: "02-tasks-row-mr-badge"
title: "/tasks rich row renders the MR badge"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/gitlab-mr-task-list-badges.md"
---

# Task 02: `/tasks` rich row renders the MR badge

Add `MRTaskIcon` to `PrimaryTaskLine` in
`apps/web/app/tasks/rich-task-list-row.tsx`, and rename the flag that gates the
badge slot so it no longer names one provider.

## The three edits

**1. Rename `showPullRequest` to `showContributions`.** The prop is file-local:
declared on `PrimaryTaskLine` (~line 30) and passed only within this file —
`true` from `RichTaskContent` (~line 129), `false` from
`TaskListRowPrimaryContent`'s compact path (~line 174). One flag gates both
badges. There is no second flag and no independent per-provider toggle: a user
who turns on "Show task details" is asking for the row's remote contributions,
not for GitHub's specifically. It is a **required** prop and must not be given a
default value. No `showPullRequest` identifier may remain in the file. (AC10,
AC11)

**2. Mount the MR badge inside the existing wrapper.** The PR badge already sits
in a `<span>` that stops `click` and `pointerdown` propagation so tapping the
badge cannot navigate the row. Put `<MRTaskIcon taskId={task.id} />` inside that
**same** wrapper, after `PRTaskIcon` — not in a second copy of it, so the two
badges cannot drift apart on interaction behaviour. (AC8, AC9)

**3. Give that wrapper `inline-flex items-center gap-1`.** It carries **no
`className` at all today**, because it has only ever held a single child, so the
row's own `gap-2` was the only separation it needed. Two children inside it would
render with **no separation whatsoever**.

`gap-1` is not a new number: it is what both surfaces that already render badges
side by side use — the Kanban card's `kanban-card-title-row`
(`flex items-center gap-1 min-w-0`) and the sidebar title row
(`flex items-center gap-1 min-w-0`). Matching it keeps PR-to-MR separation
identical on all three surfaces.

Apply the class **unconditionally**. That is what keeps AC23 compatible with AC6
rather than in tension with it: an unconditional class adds no element and
reserves no space in the no-MR case, whereas a conditional one would. Do **not**
add `shrink-0` (both `PRTaskIcon` and `MRTaskIcon` already carry it on their own
roots), and do **not** alter the row's existing `gap-2` between the title and the
badge wrapper. (AC23)

AC23 is a separate criterion from AC9 for a reason: AC9 observes only which
elements sit inside the wrapper and in what order, so an implementation leaving
the wrapper unclassed would satisfy every other criterion while shipping two
touching badges on `/tasks` and separated badges on the other two surfaces.

## Acceptance

1. With contributions shown and at least one MR for the task in the active
   workspace, the row renders `MRTaskIcon`; with a PR as well, both render inside
   the single propagation-stopping wrapper, PR before MR in DOM order. (AC8, AC9)
2. That wrapper carries `inline-flex items-center gap-1` unconditionally —
   present whether or not an MR badge renders. (AC23)
3. On the compact path the row renders neither badge, the gating boolean is named
   `showContributions`, it has no default value, and no `showPullRequest`
   identifier remains in the file. (AC10, AC11)

## Verification

```
cd apps && pnpm install --frozen-lockfile \
  && pnpm --filter @kandev/web test -- app/tasks/rich-task-list-row.test.tsx \
  && pnpm --filter @kandev/web lint \
  && cd web && pnpm run typecheck && pnpm run i18n:check
```

Also confirm no `showPullRequest` identifier survives:

```
cd apps/web && grep -rn "showPullRequest" app/tasks/rich-task-list-row.tsx
```

(expects no output).

`rich-task-list-row.tsx` measures **168 effective lines** of the 600-line limit,
so size is not a constraint here.

`app/tasks/rich-task-list-row.tsx` **is** on `i18nGuardFiles` in
`apps/web/eslint.i18n.options.mjs`, so `i18next/no-literal-string` is a
whole-file **error** on it, not merely a changed-lines ratchet. This task adds no
string literal, so no allowlist entry is needed and none may be added. (AC17)

## Files likely touched

- `apps/web/app/tasks/rich-task-list-row.tsx`
- `apps/web/app/tasks/rich-task-list-row.test.tsx` (new test file)

## Dependencies

None.

## Parallelism

`parallel-safe` with task 01 and task 03. Disjoint files. Execution is still
sequential by default in the primary conversation.

## Inputs

- Spec: "Prop naming", "Responsive and coarse-pointer behaviour",
  "Nil, empty, and error behaviour", AC8 to AC11, AC23
- Plan: Frontend section 2; Tests table rows U6, U7
- `apps/web/components/gitlab/mr-task-icon.tsx` is **not** modified (AC15) and no
  file under `apps/web/components/github/` is modified (AC14).
- `MRTaskIcon` returns `null` when the task has no MRs, so it is safe to render
  unconditionally inside the gated wrapper.

### Unit scenarios to write (U6, U7)

`TaskListRowPrimaryContent` is exported from `rich-task-list-row.tsx`, so it can
be rendered directly. Follow `app/tasks/tasks-list-view.test.tsx`'s shape: it
renders under `StateProvider` + `TooltipProvider` with **no mocks**, and has local
`makeTask` / `props` builders. Seed MRs and PRs through `StateProvider`'s
`initialState` (`taskPRs.byTaskId`, `taskMRs.byWorkspaceId`,
`workspaces.activeId`).

- **U6** — Seeded PR and MR, `showTaskDetails` true (the rich path, which passes
  `showContributions` as `true`). Both badges render **inside the single
  wrapper**, PR first by `compareDocumentPosition`, and the wrapper carries all
  three of `inline-flex`, `items-center`, `gap-1`. Assert the wrapper's classes
  directly, not just the badge order — that is the part AC9 cannot observe.
  (AC8, AC9, AC23)
- **U7** — Same store, `showTaskDetails` false (the compact path). Neither
  `pr-task-icon-*` nor `mr-task-icon-*` is present. (AC10)

## Output contract

Report: summary; files changed (reconciled against the actual diff); tests run
with exact commands and pass/fail counts; blockers; risks. Set this task's
`status` to `in_progress` at start and `done` at finish, update `## Results`
below, and synchronize the Wave 1 checkbox and `## Verification Results` in
`plan.md`.

## Results

Pending. Before marking this task done, replace this with every exact command
actually run and its outcome/count, generated artifact paths, and cleanup or
teardown evidence. Record security/trust and external side-effect boundaries when
applicable, or explicitly state `None`.
