---
id: "01-sidebar-mr-badge"
title: "Sidebar task row renders the MR badge"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/gitlab-mr-task-list-badges.md"
---

# Task 01: Sidebar task row renders the MR badge

Restructure the sidebar row's badge slot so the PR badge and the MR badge are
siblings rather than alternatives, and mount `MRTaskIcon` after the PR badge and
before the issue badge.

## Why the current code cannot just be appended to

`TaskPRIcon` in `apps/web/components/task/task-item.tsx` (~line 314) is two
chained early returns:

```tsx
const hasStorePR = useAppStore((s) => !!taskId && (s.taskPRs.byTaskId[taskId]?.length ?? 0) > 0);
if (hasStorePR) return <PRTaskIcon taskId={taskId!} />;
if (!prInfo) return null;
// ... prInfo fallback badge
```

Both `return`s exit the component. An `MRTaskIcon` appended below is unreachable
for a task that has a PR, and unreachable for a task with neither a PR nor
`prInfo` — which is exactly the GitLab-only task this feature exists to serve.

## The `taskId` guard is load-bearing

`taskId` is optional on this component (`taskId?: string`), so `hasStorePR` is
**not** a usable guard for the MR branch: it is `false` both when there is no
task id and when there is a task id with no PRs. The MR branch needs its own
`taskId` truthiness guard.

Writing `<MRTaskIcon taskId={taskId!} />` — the same non-null-assertion shape the
PR branch beside it already uses — would invoke the component with `undefined`,
hit `useTaskMRs`' internal guard, render `null`, and **pass a
"no element present" assertion while breaking AC5** and adding the store
subscription AC18 exists to prevent. The guard must prevent the *call*.

## Acceptance

1. A sidebar row with a `taskId` whose task has at least one MR in the active
   workspace renders `MRTaskIcon`; with a PR as well, both render, PR first in
   DOM order, and the MR badge precedes `IssueTaskIcon`. Full title-row order:
   title, autopilot, pinned, **PR, MR**, issue, agent-error, remote-cloud,
   archived chip. (AC1, AC2, AC3, AC4, AC22)
2. A row rendered without a `taskId` renders no MR badge **and does not invoke
   `MRTaskIcon` at all**. A row whose task has no MRs contains no element whose
   `data-testid` starts with `mr-task-icon-`. (AC5, AC6)
3. No file under `apps/web/components/task/` or
   `apps/web/components/app-sidebar/` imports `useWorkspaceMRs`; MR reads go
   through `useTaskMRs` and nothing added here allocates a fresh array or object.
   (AC7, AC18)

## Verification

```
cd apps && pnpm install --frozen-lockfile \
  && pnpm --filter @kandev/web test -- components/task/task-item.test.tsx \
  && pnpm --filter @kandev/web lint \
  && cd web && pnpm run typecheck && pnpm run i18n:check
```

`lint` runs `eslint --max-warnings 0`. `task-item.tsx` measures **591 effective
lines** against the 600-line `max-lines` limit (`skipBlankLines`,
`skipComments`), so the restructure fits inline with margin. Confirm the file
gains no new warning; it already carries one `max-lines-per-function` disable on
`TaskItem` and must not gain a second.

## Files likely touched

- `apps/web/components/task/task-item.tsx`
- `apps/web/components/task/task-item.test.tsx`

Optionally, only if the inline shape pushes `max-lines` over: **one** new sibling
under `apps/web/components/task/` for the badge-pair extraction. The spec's
Constraints permit at most one new file in total across tasks 01 and 03; if both
this task and task 03 turn out to need one, that is a genuine second finding and
routes back to the spec rather than being absorbed by a second file. Any new file
inherits AC7 and AC18.

## Dependencies

None.

## Parallelism

`parallel-safe` with task 02 and task 03. Disjoint files; no shared schema,
migration, generated contract, lockfile, or package-wide config. Execution is
still sequential by default in the primary conversation.

## Inputs

- Spec: "The sidebar early-return trap", "Selection and ordering",
  "Nil, empty, and error behaviour", AC1 to AC7, AC18, AC22
- Plan: Frontend section 1; Tests table rows U1 to U5, U10, U11
- Reference shape: `apps/web/components/kanban-card-content.tsx:144-152` — the
  already-correct `PRTaskIcon` then `MRTaskIcon` pair inside
  `kanban-card-title-row` (`flex items-center gap-1 min-w-0`). It does **not**
  change (AC16).
- `MRTaskIcon` is consumed exactly as it ships and
  `apps/web/components/gitlab/mr-task-icon.tsx` is **not** modified (AC15); no
  file under `apps/web/components/github/` is modified (AC14).

### Unit scenarios to write (U1 to U5, U10, U11)

`components/task/task-item.test.tsx` already renders `TaskItem` inside
`StateProvider` + `TooltipProvider` via a local `renderTaskItem` helper.
`StateProvider` accepts an `initialState` prop, so seed the store through it:
`taskPRs.byTaskId`, `taskMRs.byWorkspaceId`, and `workspaces.activeId`.

- **U1** — PR and MR seeded for `t1`, `workspaces.activeId = "ws1"`. Both
  `pr-task-icon-t1` and `mr-task-icon-t1` present; `compareDocumentPosition` puts
  the PR first. (AC2)
- **U2** — MR only, no PR, no `prInfo`. `mr-task-icon-t1` present,
  `pr-task-icon-t1` absent. **This is the case the early return makes impossible
  today**, so it must fail before the fix. (AC3)
- **U3** — No store PR, `prInfo={{ number: 7, state: "Open" }}`, one MR. Both
  render, PR first. (AC4)
- **U4** — No `taskId`, MRs seeded for some other id. Assert **both** clauses;
  they are not equivalent and the weaker one alone is satisfied by code that
  violates the AC. (a) no `mr-task-icon-*` element. (b) **`MRTaskIcon` is not
  invoked**, via a `vi.fn()`-backed module mock. Clause (b) is load-bearing.

  **No in-repo precedent for clause (b) exists — write it fresh.**
  `components/kanban-card-content.test.tsx` is the precedent for **module-level
  interception only**: it replaces the module with a plain stub
  (`MRTaskIcon: () => <span data-testid="mr-task-icon" />`), and a stub records no
  calls. No test under `apps/web/components/` mocks a component module with
  `vi.fn()` and then asserts a call count. Copy that file's `vi.mock` placement
  and factory shape; do **not** copy its stub body. Required shape:

  ```tsx
  vi.mock("@/components/gitlab/mr-task-icon", () => ({ MRTaskIcon: vi.fn(() => null) }));
  // ...
  expect(MRTaskIcon).not.toHaveBeenCalled();
  ```

  Because this mock is module-scoped, U4 likely belongs in its own describe file
  or must be arranged so the other scenarios still render the real component.
  (AC5, AC18)
- **U5** — PR only, no MRs. No `mr-task-icon-*` element. (AC6)
- **U10** — One MR, a store PR, and `issueInfo={{ url: "…", number: 42 }}`.
  Assert the full **PR, MR, issue** order via `compareDocumentPosition`, not just
  the adjacent pair. (AC22)
- **U11** — MR seeded at `taskMRs.byWorkspaceId["ws1"]["t1"]` but
  `workspaces.activeId = null`. No `mr-task-icon-*` element. This observes AC21's
  second clause, which no other scenario reaches. (AC21)

Do not go looking for id-less rows in the live sidebar; **there are none**.
`TaskItem`'s only production importer is `components/task/task-switcher-row.tsx`,
whose single mount passes `taskId={task.id}`. AC5 is a defensive contract on the
optional prop, exercised synthetically by U4.

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
