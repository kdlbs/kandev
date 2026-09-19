---
spec: docs/specs/integrations/requirements/gitlab-mr-task-list-badges.md
created: 2026-08-13
status: draft
---

# Implementation Plan: GitLab MR Badge on the Sidebar and Tasks-List Rows

> **Amendment:** The later
> [rich task title preview plan](../rich-task-title-previews/plan.md) extends
> this plan. It owns rich badge content and shared hydration behavior.

## Overview

`MRTaskIcon` ships today with exactly one call site (the Kanban card). This plan
adds it to the two remaining task-row surfaces: the app sidebar row
(`components/task/task-item.tsx`) and the `/tasks` rich row
(`app/tasks/rich-task-list-row.tsx`), and gives `/tasks` its own unconditional MR
hydration call so its own row has data to render. Frontend only: no backend, no
API, no store slice, no new component, no new user-facing copy.

The three production edits are on disjoint files and can land in any order; the
three E2E specs follow them. The order below is dependency order, not risk order:
each surface is independently observable, so the product is working after every
task.

---

## Backend

None. The spec introduces no persistent state, no API, no WS action, and no
change to `TaskStatusSummary`. The GitLab MR rows the badge reads are already
cached at `taskMRs.byWorkspaceId[workspaceId][taskId]`.

---

## Frontend

### 1. App sidebar row — `apps/web/components/task/task-item.tsx`

The local `TaskPRIcon` component (currently at ~line 314) is two chained early
returns:

```tsx
const hasStorePR = useAppStore((s) => !!taskId && (s.taskPRs.byTaskId[taskId]?.length ?? 0) > 0);
if (hasStorePR) return <PRTaskIcon taskId={taskId!} />;
if (!prInfo) return null;
// ... prInfo fallback badge
```

Both `return`s exit the component, so anything appended after them is
unreachable for a task that has a PR **and** for a task that has neither a PR nor
`prInfo` — the GitLab-only task this feature exists to serve.

Restructure so the PR branch and the MR branch are siblings. Rename the component
to reflect that it now renders both contribution badges, and have it return a
fragment:

```tsx
function TaskContributionIcons({ taskId, prInfo }: { taskId?: string; prInfo?: PrInfo }) {
  const hasStorePR = useAppStore((s) => !!taskId && (s.taskPRs.byTaskId[taskId]?.length ?? 0) > 0);
  return (
    <>
      <TaskPRBadge taskId={taskId} prInfo={prInfo} hasStorePR={hasStorePR} />
      {taskId ? <MRTaskIcon taskId={taskId} /> : null}
    </>
  );
}
```

Three rules the restructure must hold, each with its own AC:

- The MR branch needs **its own `taskId` truthiness guard**. `hasStorePR` is not
  usable: it is `false` both when `taskId` is absent and when `taskId` is present
  with no PRs. `<MRTaskIcon taskId={taskId!} />` (the non-null-assertion shape the
  PR branch beside it uses) would invoke the component with `undefined`, add a
  `useTaskMRs` subscription, and violate AC5 and AC18 while still rendering
  nothing. The guard must prevent the call, not just the output. (AC5, AC18)
- The call site inside `TaskItemContent`'s title row keeps its position: the MR
  badge lands immediately after the PR badge and **before** `IssueTaskIcon`.
  Full row order: title, autopilot, pinned, **PR, MR**, issue, agent-error,
  remote-cloud, archived chip. (AC2, AC22)
- The sidebar hydrates nothing. No file under `components/task/` or
  `components/app-sidebar/` may import `useWorkspaceMRs`. (AC7)

`task-item.tsx` measures **591 effective lines** against the 600-line
`max-lines` limit (`skipBlankLines`, `skipComments`, `--max-warnings 0`). The
restructure above fits inline with margin; the one-new-file allowance in the
spec's Constraints is a fallback, not the expected shape.

### 2. `/tasks` rich row — `apps/web/app/tasks/rich-task-list-row.tsx`

In `PrimaryTaskLine`:

- Rename the prop `showPullRequest: boolean` to `showContributions: boolean`.
  It is file-local (declared and passed only within this file: `true` from
  `RichTaskContent` at ~line 129, `false` from `TaskListRowPrimaryContent`'s
  compact path at ~line 174). One flag gates both badges; no second flag, no
  per-provider toggle, and no default value. No `showPullRequest` identifier may
  remain in the file. (AC10, AC11)
- Add `<MRTaskIcon taskId={task.id} />` **inside the existing**
  propagation-stopping `<span>` that already wraps `PRTaskIcon`, after it. Not a
  second wrapper: one wrapper, two children, so the badges cannot drift apart on
  interaction behaviour. (AC9)
- That wrapper carries **no `className` today** because it has only ever had one
  child. Two children would render with no separation at all. Give it
  `inline-flex items-center gap-1` — the same `gap-1` the Kanban card's
  `kanban-card-title-row` and the sidebar title row already use. Apply it
  **unconditionally**, so it adds no element and reserves no space in the no-MR
  case. (AC23, and this is why AC23 and AC6 are not in tension)
- Do not add `shrink-0` (both badge roots already carry it) and do not touch the
  row's existing `gap-2`.

### 3. `/tasks` hydration — `apps/web/app/tasks/tasks-page-client.tsx`

The page already owns its GitHub hydration, gated on the same setting that gates
the badge (line 529):

```ts
useWorkspacePRs(showTaskDetails ? s.activeWorkspaceId : null);
```

**Mirroring that expression for GitLab is a cross-surface data-loss bug.** The
two hooks do not have the same null behaviour: `useWorkspacePRs(null)` clears a
local ref and touches no store state, while `useWorkspaceMRs(null)` calls
`resetTaskMRs()` **with no argument**, which assigns `taskMRs.byWorkspaceId = {}`
— every workspace, wiped. `AppSidebar` is mounted in the root layout and is
therefore on screen on `/tasks`, so the gated form would blank the sidebar's MR
badges for the whole time a user sits on `/tasks` with task details off.

Add, unconditionally:

```ts
import { useWorkspaceMRs } from "@/hooks/domains/gitlab/use-task-mr";
// ...
useWorkspaceMRs(s.activeWorkspaceId);
```

matching `kanban-board.tsx:321`'s existing unconditional call rather than
`useWorkspacePRs`'s gated one. (AC12)

Passing `null` because `s.activeWorkspaceId` is itself `null` is permitted and
is not the forbidden case: with no active workspace `useTaskMRs` returns
`EMPTY_MRS` for every task, so the clear takes nothing away. (AC21)

**Check `max-lines` on this file before assuming the edit is free.** It measures
**597 effective lines of 600** — the tightest of the three, despite being the
smaller file by raw count (642 vs 660). The edit above is exactly two effective
lines, landing it at 599. That fits, but with one line to spare. If the actual
`eslint` run disagrees, the remedy is an extraction into a sibling under
`app/tasks/` (the spec's Constraints permit exactly one new file for this
purpose), **not** an `eslint-disable`: the file carries none today.

### API client

None.

### State

None. Reads go through the existing `useTaskMRs(taskId)`
(`hooks/domains/gitlab/use-task-mr.ts:60`), which resolves `workspaces.activeId`
itself and returns the module-level, referentially stable `EMPTY_MRS` when there
is nothing to show. No new selector may be introduced, and nothing added by this
feature may return a freshly allocated array or object — a fresh `[]` causes an
infinite re-render loop, as `EMPTY_MRS`' own comment records. (AC18)

`apps/web/components/gitlab/mr-task-icon.tsx` and everything under
`apps/web/components/github/` are consumed exactly as they ship and are not
modified. (AC14, AC15)

---

## Tests

Unit scenarios are `U1` to `U11` per the spec. Note the spec's numbering rule:
cite scenarios by prefixed id, never by bare number.

| # | What | File | How |
|---|---|---|---|
| U1 | Sidebar, PR and MR together, PR first in DOM order (AC2) | `components/task/task-item.test.tsx` | Render `TaskItem` under `StateProvider` with `initialState` seeding `taskPRs.byTaskId.t1`, `taskMRs.byWorkspaceId.ws1.t1`, `workspaces.activeId = "ws1"`; assert both testids present and `compareDocumentPosition` |
| U2 | Sidebar, MR only, no PR and no `prInfo` (AC3) | same | The case the early return makes impossible today |
| U3 | Sidebar, `prInfo` fallback plus MR, PR first (AC4) | same | `prInfo={{ number: 7, state: "Open" }}`, no store PR |
| U4 | Sidebar, no `taskId` (AC5) | same | **Two clauses, both required.** (a) no `mr-task-icon-*` element; (b) `MRTaskIcon` **not invoked at all** |
| U5 | Sidebar, no MRs, no `mr-task-icon-*` element (AC6) | same | PR only |
| U6 | `/tasks` row, contributions shown: both badges inside the single wrapper, PR first, wrapper carries `inline-flex items-center gap-1` (AC8, AC9, AC23) | `app/tasks/rich-task-list-row.test.tsx` (new) | Render the exported `TaskListRowPrimaryContent` with `showTaskDetails` true under `StateProvider`, following `tasks-list-view.test.tsx`'s no-mock shape |
| U7 | `/tasks` row, contributions hidden: neither badge (AC10) | same | Same store, compact path |
| U8 | `useWorkspaceMRs(null)` wipes every workspace (AC13) | `hooks/domains/gitlab/use-task-mr.test.ts` | Seed `byWorkspaceId["ws-a"]` and `["ws-b"]`, render the hook with `null`, assert both gone. Pins the existing behaviour AC12 exists to route around |
| U9 | `TasksPageClient` calls `useWorkspaceMRs` with the workspace id, not `null`, while `tasksListShowDetails` is false (AC12) | `app/tasks/tasks-page-client.mr-hydration.test.tsx` (new) | Mock `@/hooks/domains/gitlab/use-task-mr` with `vi.fn()`, assert the argument. See the risk note in task-03 |
| U10 | Sidebar, MR before the issue badge, run with a store PR present so it covers the full PR-MR-issue order (AC22) | `components/task/task-item.test.tsx` | `issueInfo={{ url, number: 42 }}` plus a seeded PR and MR |
| U11 | MR seeded but `workspaces.activeId = null`: no badge (AC21) | same | Observes AC21's second clause, which no other scenario reaches |

**U4 clause (b) needs a `vi.fn()`-backed component mock, and no in-repo
precedent exists — write it fresh.** `components/kanban-card-content.test.tsx` is
the precedent for **module-level interception only**: it replaces the module with
a plain stub (`MRTaskIcon: () => <span data-testid="mr-task-icon" />`), and a stub
records no calls, so clause (b) cannot be asserted against it. Copy that file's
`vi.mock` placement and factory shape; do **not** copy its stub body. The required
shape is:

```tsx
vi.mock("@/components/gitlab/mr-task-icon", () => ({ MRTaskIcon: vi.fn(() => null) }));
// ...
expect(MRTaskIcon).not.toHaveBeenCalled();
```

Clause (b) is the load-bearing one: an unguarded `<MRTaskIcon taskId={taskId!} />`
would pass clause (a) while breaking AC5.

---

## E2E Tests

Required: `apps/web/**` is touched and none of the files are on the Testing
step's exemption allowlist.

All GitLab scenarios seed through the GitLab mock provider and a task created
with **no agent**, for the reason `mr-task-card-badge.spec.ts`'s `seedBoardTask`
comment records: `createTaskWithAgent` auto-starts a session, the start step
carries `on_turn_complete: move_to_step review`, and the card leaves the column
mid-assertion.

**Seed helpers are copied, not imported.** `nextMRIID`, `seedMR`, `linkMR`,
`ensureGitLabConfigured` and `seedBoardTask` are file-local and **unexported** in
`mr-task-card-badge.spec.ts`. Each new spec defines its own local copies.
`mr-task-card-badge.spec.ts` is not edited to export them, which is what keeps
AC16 ("passes unmodified") literally true, and it matches the in-repo precedent:
`mobile-mr-task-card-badge.spec.ts` already writes its own `seedBoardTaskWithMR`.
Shared **constants** are different: import `GITLAB_HOST` and `GITLAB_PROJECT`
from `e2e/helpers/gitlab.ts`.

**Lint consequence the builder must respect.** The spec corrects an earlier draft
here, and the correct mechanism is narrower than it looks. In
`apps/web/eslint.config.mjs`, `sonarjs/no-identical-functions` sits in the global
block (no `files`, no `ignores`), so it **is live on `e2e/**`** — the
`e2e/**` entry in an `ignores` array belongs to the i18n guard block, and the
`files: ["e2e/**/*.ts"]` override disables `max-lines` and
`sonarjs/no-duplicate-string` but **not** `no-identical-functions`. It cannot fire
across files, because ESLint sees one file at a time. So: copy each helper
across files freely; **never write two copies of the same helper inside one
file**, because `pnpm --filter @kandev/web lint` runs `eslint --max-warnings 0`.

**The mock GitHub PR for E3, E6 and E7** comes from the existing mock-GitHub API
client methods: `apiClient.mockGitHubReset()`, `apiClient.mockGitHubSetUser()`,
then `apiClient.mockGitHubAssociateTaskPR({ task_id, owner, repo, pr_number,
pr_url, pr_title, head_branch, base_branch, author_login, state })` — `state` is
optional and is passed explicitly. **`mockGitHubAddPRs()` is the wrong call and
must not be used**: it seeds the mock provider's PR *catalogue* and creates no
task-to-PR association, so `taskPRs.byTaskId[taskId]` stays empty, `PRTaskIcon`
renders nothing, and the PR-before-MR assertion can never run. The reference is
`mr-task-card-badge.spec.ts`'s own `"AC30/AC37"` test; reading it does not modify
it.

**Every `mr-task-icon-*` accessor must be container-scoped** and may not use
`.first()`. One task id can be emitted by two mounted rows on one route (sidebar
plus board, or sidebar plus `/tasks` row). Scope to `app-sidebar`,
`tasks-list-row`, or `task-card-<id>`. (AC19)

`apps/web/e2e/tests/gitlab/mr-sidebar-badge.spec.ts` (new):

- **E1** — On `/` (the board mounts the hydration owner), a task with one linked
  open MR: inside `app-sidebar`, row located by `[data-task-row-id="<taskId>"]`,
  `mr-task-icon-<taskId>` visible with `data-mr-count="1"` and
  `data-mr-state="open"`. (AC1, AC19)
- **E2** — Same route, task with no linked MR: inside `app-sidebar`,
  `mr-task-icon-<taskId>` has count 0. (AC6)
- **E3** — Task with both a mock GitHub PR and a linked MR: inside the sidebar
  row, both visible, PR precedes MR by `compareDocumentPosition`. (AC2)

`apps/web/e2e/tests/gitlab/mr-tasks-list-badge.spec.ts` (new):

- **E4** — Navigate to `/tasks`, enable "Show task details" through the
  `display-button` menu (the pattern in `task-listing-view-preferences.spec.ts`:
  `getByTestId("display-button").click()` then
  `getByText("Show task details", { exact: true }).click()`), locate the
  `tasks-list-row` by title (the row carries **no task-id attribute**; only
  `tasks-list-row-title` identifies it), assert `mr-task-icon-<taskId>` inside
  that row. (AC8, AC19)
- **E5** — Same task, details off: inside the row, both `pr-task-icon-<taskId>`
  and `mr-task-icon-<taskId>` have count 0. (AC10)
- **E6** — Task with both: inside the row, PR precedes MR. (AC9)

`apps/web/e2e/tests/gitlab/mobile-mr-tasks-list-badge.spec.ts` (new):

- **E7** — Mobile viewport, details on, task with both badges: the MR badge is
  visible and `assertNoDocumentHorizontalOverflow` passes. Mirrors
  `mobile-mr-task-card-badge.spec.ts`. (AC20)

All seven scenarios run on `/` or `/tasks`. Neither route mounts
`TaskPageContent`, so none is exposed to the two accepted hydration holes the
spec records (an archived task at `/t/:taskId`, and
`use-external-vcs-file-link.ts`'s already-shipped gated `useWorkspaceMRs(null)`).
The builder must not attempt to work around either hole from the three production
files in scope; in particular the sidebar must not gain its own `useWorkspaceMRs`
call, which AC7 forbids.

**Page object.** Extend `e2e/pages/app-sidebar-page.ts` (already anchored at
`app-sidebar`) with a row-and-badge accessor. Do not add a parallel sidebar page
object; `e2e/pages/sidebar-tasks-page.ts` is the shape to follow.

---

## Verification Results

Pending. On completion, synchronize this section with each task's `## Results`:
record exact commands and outcomes/counts, generated artifact paths, and
cleanup/teardown evidence.

---

## Implementation Waves And Parallel Candidates

The three production tasks touch disjoint files and share no schema, migration,
generated contract, lockfile, or package-wide config, so they are genuine
parallel candidates. **The default is sequential execution in the primary
conversation.** Waves do not authorize subagents; only the user may ask for them
after selecting the implementation model.

```
Wave 1 (parallel candidates — user authorization required):
- [ ] [task-01-sidebar-mr-badge](task-01-sidebar-mr-badge.md)
- [ ] [task-02-tasks-row-mr-badge](task-02-tasks-row-mr-badge.md)
- [ ] [task-03-tasks-page-mr-hydration](task-03-tasks-page-mr-hydration.md)

Wave 2:
- [ ] [task-04-e2e-sidebar-badge](task-04-e2e-sidebar-badge.md)      (needs 01)
- [ ] [task-05-e2e-tasks-list-badge](task-05-e2e-tasks-list-badge.md) (needs 02, 03)
```

Before any task, in a fresh worktree: `cd apps && pnpm install --frozen-lockfile`.
`apps/node_modules/` is not shared across worktrees, and without it `vitest`,
`eslint` and the commit hooks all fail.

---

## Open Questions

None. The spec settles badge ordering (PR then MR, DOM order, all three
surfaces), the `/tasks` flag question (one flag, renamed `showContributions`),
coarse-pointer behaviour (the Radix tooltip is inherited unchanged), and the
sidebar `prInfo` question (deliberately GitHub-only; an MR analogue is a backend
projection change and is out of scope).
