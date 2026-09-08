---
id: "03-tasks-page-mr-hydration"
title: "/tasks page hydrates workspace MRs unconditionally"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/gitlab-mr-task-list-badges.md"
---

# Task 03: `/tasks` page hydrates workspace MRs unconditionally

`/tasks` becomes an owner of GitLab MR hydration for the active workspace,
because it is the surface whose own row renders the badge (task 02). The sidebar
does **not** become one.

## The trap: do not mirror the GitHub line

`TasksPageClient` already owns its GitHub hydration, gated on the same setting
that gates the badge (`apps/web/app/tasks/tasks-page-client.tsx:529`):

```ts
useWorkspacePRs(showTaskDetails ? s.activeWorkspaceId : null);
```

**Mirroring that expression for GitLab is a cross-surface data-loss bug.** The
two hooks do not have the same null behaviour:

- `useWorkspacePRs(null)` clears a local ref and touches no store state.
- `useWorkspaceMRs(null)` calls `resetTaskMRs()` **with no argument**, and
  `resetTaskMRs` with no argument assigns `taskMRs.byWorkspaceId = {}` — every
  workspace, wiped. See `apps/web/hooks/domains/gitlab/use-task-mr.ts:23-32`.

`AppSidebar` is mounted in the root layout and is therefore on screen on
`/tasks`. The gated form would blank the sidebar's MR badges (and every other MR
consumer's data) for the entire time a user sits on `/tasks` with task details
switched off, and would re-wipe on every render pass that re-runs the effect. The
wipe is not confined to the page that caused it.

## The edit

Two effective lines:

```ts
import { useWorkspaceMRs } from "@/hooks/domains/gitlab/use-task-mr";
// ... inside TasksPageClient, beside the existing useWorkspacePRs call:
useWorkspaceMRs(s.activeWorkspaceId);
```

Unconditional, matching `apps/web/components/kanban-board.tsx:321`'s existing
unconditional call rather than `useWorkspacePRs`'s gated one. The cost is one
`GET` per workspace per effect run, which is not wasted even when task details
are off: the sidebar is mounted on that route and renders MR badges from the same
store.

Passing `null` because `s.activeWorkspaceId` is itself `null` is **permitted and
correct**, and is not the case the rule forbids: with no active workspace,
`useTaskMRs` returns `EMPTY_MRS` for every task and no surface can render an MR
badge, so the clear takes nothing away. `kanban-board.tsx` already does exactly
this. The forbidden form is different in kind — it passes `null` while a
workspace is active and other surfaces are displaying that workspace's MRs.
(AC21)

Do not write a request-count assertion anywhere. The app root renders inside
`<StrictMode>` (`apps/web/src/main.tsx`) and the hook's cleanup clears
`fetchedRef`, so React's development-only effect replay issues a **second** `GET`
on first mount. That is existing behaviour shared with all four current call
sites and does not occur in a production build.

## Check `max-lines` before assuming the edit is free

`tasks-page-client.tsx` measures **597 effective lines of the 600 allowed**
(`max-lines`, `skipBlankLines`, `skipComments`, under
`eslint --max-warnings 0`). It is the tightest of the three files despite being
the smaller by raw count (642 vs `task-item.tsx`'s 660). The edit above is
exactly two effective lines, landing it at 599. **That fits, with one line to
spare — verify it with an actual `eslint` run rather than trusting this figure.**

If it does go over, the remedy is an extraction into a sibling under
`app/tasks/`. The spec's Constraints permit at most **one** new file across this
task and task 01 combined; if both turn out to need one, that is a genuine second
finding and routes back to the spec.

**Do not resolve it with an `eslint-disable`.** The file carries none today, and
silencing a size warning to land a two-line hook call trades a real signal for
convenience.

## Acceptance

1. `TasksPageClient` calls `useWorkspaceMRs` with `s.activeWorkspaceId`
   unconditionally, with the argument gated on neither `tasksListShowDetails` nor
   any other setting. (AC12, AC21)
2. `useWorkspaceMRs(null)` clearing every workspace's entry under
   `taskMRs.byWorkspaceId` is pinned by a test that seeds two workspaces. This
   pins the existing behaviour AC12 exists to route around, so a future change to
   the hook cannot silently make the gated form look safe. (AC13)
3. `apps/web/app/tasks/tasks-page-client.tsx` gains no new lint warning, in
   particular no `max-lines` warning, and no `eslint-disable`.

## Verification

```
cd apps && pnpm install --frozen-lockfile \
  && pnpm --filter @kandev/web test -- hooks/domains/gitlab/use-task-mr.test.ts \
  && pnpm --filter @kandev/web lint \
  && cd web && pnpm run typecheck && pnpm run i18n:check
```

Plus, explicitly, the size check that this task's third acceptance condition
turns on:

```
cd apps/web && npx eslint app/tasks/tasks-page-client.tsx
```

(expects zero output; `--max-warnings 0` means any `max-lines` warning is a
failure).

Add the U9 test file to the `test` invocation once written.

`app/tasks/tasks-page-client.tsx` **is** on `i18nGuardFiles` in
`apps/web/eslint.i18n.options.mjs`, so `i18next/no-literal-string` is a
whole-file **error** on it. This task adds no string literal, so no allowlist
entry is needed and none may be added. (AC17)

## Files likely touched

- `apps/web/app/tasks/tasks-page-client.tsx`
- `apps/web/hooks/domains/gitlab/use-task-mr.test.ts` (U8)
- `apps/web/app/tasks/tasks-page-client.mr-hydration.test.tsx` (U9, new — see the
  risk note below)

## Dependencies

None. Task 02 renders the badge this hydration feeds, but neither task blocks the
other: task 03's tests observe the hook argument, not the badge.

## Parallelism

`parallel-safe` with task 01 and task 02. Disjoint files. Execution is still
sequential by default in the primary conversation.

## Inputs

- Spec: "Hydration ownership" in full, including "`/tasks` becomes an owner. The
  gated form is forbidden." and "The sidebar does not become an owner"; AC12,
  AC13, AC21
- Plan: Frontend section 3; Tests table rows U8, U9
- Reference: `apps/web/components/kanban-board.tsx:321` — the existing
  unconditional call this one matches.

### Unit scenarios to write (U8, U9)

- **U8 — `useWorkspaceMRs(null)` wipes every workspace (AC13).** Seed
  `taskMRs.byWorkspaceId["ws-a"]` and `["ws-b"]`, render the hook with `null`,
  assert **both** are gone. `hooks/domains/gitlab/use-task-mr.test.ts` already has
  a `describe("useWorkspaceMRs")` block with `renderHook` scaffolding at ~line 92
  and a cleanup block at ~line 212; add there if not already covered. This is the
  straightforward half of the task.

- **U9 — `TasksPageClient` hydration argument (AC12).** With
  `tasksListShowDetails` false and an active workspace, `useWorkspaceMRs` is
  called with the workspace id, **not** `null`.

  **This is the one genuinely uncertain test in the plan, and the estimate should
  not be trusted blindly.** `tasks-page-client.tsx` is a 642-line component with
  many hooks, and no existing test renders it: `app/tasks/tasks-page-fetch-policy.test.ts`
  is a pure-function test of `shouldSkipInitialTasksFetch`, and
  `app/tasks/tasks-list-view.test.tsx` renders the *view*, not the page client.
  So there is no established mocking baseline for this component to copy.

  Preferred approach: a new `app/tasks/tasks-page-client.mr-hydration.test.tsx`
  that mocks `@/hooks/domains/gitlab/use-task-mr` with `vi.fn()`, renders
  `TasksPageClient` under `StateProvider` with `userSettings.tasksListShowDetails`
  false and `workspaces.activeId` set, and asserts
  `expect(useWorkspaceMRs).toHaveBeenCalledWith("ws1")`. Expect to mock several
  further modules to get it to mount; that is the cost, and it is acceptable
  because AC12 is the criterion protecting against a real data-loss bug.

  If the render turns out to need so much mocking that the test asserts more about
  the mocks than the code, **stop and report it** rather than silently downgrading
  to a source-text assertion. The fallback the spec's Constraints already permit is
  an extraction: move the two-line hydration decision into a tiny sibling under
  `app/tasks/` and unit-test that directly — which also solves the `max-lines`
  pressure above, using the single new-file allowance for both purposes at once.
  That is a design choice worth surfacing, not making silently.

## Output contract

Report: summary; files changed (reconciled against the actual diff); tests run
with exact commands and pass/fail counts; the measured `eslint` result on
`tasks-page-client.tsx`; whether the U9 fallback was taken and why; blockers;
risks. Set this task's `status` to `in_progress` at start and `done` at finish,
update `## Results` below, and synchronize the Wave 1 checkbox and
`## Verification Results` in `plan.md`.

## Results

Pending. Before marking this task done, replace this with every exact command
actually run and its outcome/count, generated artifact paths, and cleanup or
teardown evidence. Record security/trust and external side-effect boundaries when
applicable, or explicitly state `None`.
