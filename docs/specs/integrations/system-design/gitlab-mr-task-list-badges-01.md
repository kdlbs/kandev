---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITLAB-MR-TASK-LIST-BADGES-001
created: 2026-08-12
owners:
  - tbd
---
# GitLab MR Badge on the Sidebar and Tasks-List Rows System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-INTEGRATIONS-GITLAB-MR-TASK-LIST-BADGES-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-GITLAB-MR-TASK-LIST-BADGES-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

> **Amendment:** [Rich titles](../../tasks/requirements/rich-task-title-previews.md)
> extends this draft. It replaces the requirements that `MRTaskIcon` stay
> unmodified, add no copy, and have only one hydration owner. The badge
> placement and provider data-model requirements in this document still apply.

## Why

A task whose work lives on GitHub shows a pull-request badge on all three
task-row surfaces: the Kanban card, the app sidebar's task list, and the
`/tasks` page rows. A task whose work lives on GitLab shows a merge-request
badge on exactly one of them, the Kanban card. On the other two a GitLab-only
task looks like a task with no remote contribution at all.

This is not a regression. `docs/specs/integrations/requirements/gitlab-integration.md` scoped the MR
badge to the Kanban card and to nothing else, and the code matches that scope
exactly: `MRTaskIcon` has one call site,
`apps/web/components/kanban-card-content.tsx:152`. This spec **adds** the two
missing surfaces. It is a sibling of the shipped
[gitlab-mr-status-chip](../requirements/gitlab-mr-status-chip.md) card and belongs to the
same GitLab-parity family.

## What

- A task with at least one linked GitLab merge request SHALL render the existing
  `MRTaskIcon` badge on the app sidebar's task row and on the `/tasks` page's
  rich task row, in addition to the Kanban card it already renders on.
- The badge SHALL be the existing `MRTaskIcon` component, rendered unmodified.
  No new glyph, no new colour rule, no new status derivation, no new
  `data-testid`, and no new user-facing copy are introduced by this feature.
- On every surface that shows both, the pull-request badge SHALL precede the
  merge-request badge in DOM order.
- The `/tasks` page SHALL become an owner of GitLab MR hydration for the active
  workspace, because it is the surface whose own row renders the badge. The
  sidebar SHALL NOT become one; see Hydration ownership.
- No file under `apps/web/components/github/` changes.
  `apps/web/components/gitlab/mr-task-icon.tsx` does not change either.
  `PRTaskIcon`'s and `MRTaskIcon`'s rendered output SHALL be identical before and
  after this feature.

## Data model

No new persistent state, no new API, no new store slice, no new WS action.

The badge reads the `TaskMR` rows already cached at
`taskMRs.byWorkspaceId[workspaceId][taskId]`
(`apps/web/lib/state/slices/gitlab/gitlab-slice.ts`), typed at
`apps/web/lib/types/gitlab.ts`.

The two provider store shapes are **not** symmetric, and this asymmetry is the
first implementation trap:

| | GitHub | GitLab |
|---|---|---|
| store path | `taskPRs.byTaskId[taskId]` | `taskMRs.byWorkspaceId[workspaceId][taskId]` |
| accessor | direct `useAppStore` selector | `useTaskMRs(taskId)` |
| workspace scoping | none | active workspace only |

`useTaskMRs(taskId)` (`apps/web/hooks/domains/gitlab/use-task-mr.ts:60`) resolves
`workspaces.activeId` itself and returns the module-level, referentially stable
`EMPTY_MRS` when there is nothing to show. A hand-rolled selector that returns a
fresh `[]` causes an infinite re-render loop; the constant's own comment records
this.

There is a **second** GitHub-only source the sidebar row already uses and which
has no GitLab counterpart: `TaskItem`'s `prInfo` prop. It is not prototype-only
data. `task-session-sidebar-item.ts:109` derives it from the bounded task status
projection `TaskStatusSummary.pull_request`
(`apps/web/lib/types/task-status-summary.ts`), which carries `number`, `state`,
`aggregate_state` and `url` and **no `merge_request` field at all**.
`sidebar-mock-data.ts` supplies the same shape for the sidebar prototype. A
GitLab analogue of that path is therefore a backend projection change, not a
frontend one; see Out of scope.

## Surfaces and mount points

Exactly three **existing** production React files change, plus **at most one new
sibling component file** if the badge-pair extraction reads better out-of-line.
Constraints owns that allowance and states where the new file may live; this
section is not a second, tighter fence. Line numbers drift; the anchors are the
named symbols.

1. **App sidebar row** — `apps/web/components/task/task-item.tsx`, the local
   `TaskPRIcon` component and its single call site inside `TaskItemContent`'s
   title row.
2. **`/tasks` rich row** — `apps/web/app/tasks/rich-task-list-row.tsx`,
   `PrimaryTaskLine`.
3. **`/tasks` page hydration** — `apps/web/app/tasks/tasks-page-client.tsx`,
   `TasksPageClient`.

The Kanban card (`kanban-card-content.tsx`) is already correct and is the
reference shape. It does not change.

### The sidebar early-return trap

`TaskPRIcon` today is:

```tsx
const hasStorePR = useAppStore((s) => !!taskId && (s.taskPRs.byTaskId[taskId]?.length ?? 0) > 0);
if (hasStorePR) return <PRTaskIcon taskId={taskId!} />;
if (!prInfo) return null;
// ... summary-projection fallback badge
```

Both `return`s exit the component. An `MRTaskIcon` appended after them is
unreachable for any task that has a PR, and unreachable for any task that has
neither a PR nor `prInfo` — that is, for the GitLab-only task this feature
exists to serve. The component SHALL be restructured so the PR branch and the MR
branch are siblings, not alternatives.

`taskId` is optional on this component (`taskId?: string`), so `hasStorePR` is
not a usable guard for the MR branch: it is `false` both when there is no task id
and when there is a task id with no PRs. The MR branch needs its own `taskId`
truthiness guard.

## Selection and ordering

- **Badge order is fixed and named: pull request, then merge request.** It is
  DOM order within the row's title line, not visual order produced by CSS, and it
  is identical on all three surfaces. The Kanban card already ships this order
  (`PRTaskIcon` then `MRTaskIcon`), and `mr-task-card-badge.spec.ts` already
  asserts it there; the two new surfaces adopt it rather than inventing one.
- **Sidebar title row, full order:** title, autopilot icon, pinned icon,
  **PR badge, MR badge**, issue badge, agent-error icon, remote-cloud icon,
  archived chip. The MR badge sits immediately after the PR badge and before
  `IssueTaskIcon`: the two contribution badges group together, and the issue
  badge (a different entity class) follows them.
- **`/tasks` rich row, full order:** state icon, title, **PR badge, MR badge**,
  archived badge.
- **Which MR the badge represents, and the multi-MR count**, are decided by
  `MRTaskIcon` and are unchanged: one MR renders `SingleMRIcon`, two or more
  render `MultiMRIcon` with `aggregateMRStatusColor` and a count. That includes
  `aggregateMRStatusColor`'s first-in-array-order tie, which this feature freezes
  rather than touches (it is re-tuning the colour priority; see Out of scope).
  There is no per-surface selection rule and this spec SHALL NOT introduce one.

## Hydration ownership

`useWorkspaceMRs(workspaceId)` is the only thing that puts MR rows in the store.
Its four existing call sites are `kanban-board.tsx` (unconditional on
`workspaceState.activeId`), `mr-topbar-button.tsx`, `use-mr-key-to-tasks.ts` and
`use-external-vcs-file-link.ts`. Neither the sidebar nor `/tasks` is among them.
Consolidating these behind one provider-level owner is an already-declared
out-of-scope card in the sibling spec, and this feature does not take it on.

### `/tasks` becomes an owner. The gated form is forbidden.

`TasksPageClient` already owns its GitHub hydration, gated on the same setting
that gates the badge:

```ts
useWorkspacePRs(showTaskDetails ? s.activeWorkspaceId : null);
```

**Mirroring that expression for GitLab is a cross-surface data-loss bug**, and
this is the second implementation trap. The two hooks do not have the same
null behaviour:

- `useWorkspacePRs(null)` clears a local ref and touches no store state.
- `useWorkspaceMRs(null)` calls `resetTaskMRs()` **with no argument**, and
  `resetTaskMRs` with no argument assigns `taskMRs.byWorkspaceId = {}` — every
  workspace, wiped.

Because `AppSidebar` is mounted in the root layout (`app/layout.tsx`) and is
therefore on screen on `/tasks`, the gated form would blank the sidebar's MR
badges (and any other MR consumer's data) for the entire time a user sits on
`/tasks` with task details switched off, and would keep re-wiping on every
render pass that re-runs the effect. The wipe is not confined to the page that
caused it.

The `/tasks` page SHALL therefore call `useWorkspaceMRs(s.activeWorkspaceId)`
**unconditionally**, matching `kanban-board.tsx`'s existing unconditional call
rather than `useWorkspacePRs`'s gated one. The cost is one `GET` per workspace
per effect run, which is not wasted even when task details are off: the sidebar
is mounted on that route and renders MR badges from the same store.

Note "per effect run", not "per mount": the app root renders inside
`<StrictMode>` (`apps/web/src/main.tsx`), and the hook's cleanup clears
`fetchedRef`, so React's development-only effect replay issues a **second**
`GET` on first mount. That is existing behaviour shared with all four current
call sites, it does not occur in a production build, and no acceptance criterion
here asserts a request count. It is stated so nobody writes a call-count
assertion against the wrong number.

`s.activeWorkspaceId` can itself be `null`, and the unconditional form then
passes `null` and clears the cache. That is correct and is not the case this
rule forbids: with no active workspace, `useTaskMRs` returns `EMPTY_MRS` for
every task and no surface can render an MR badge, so there is nothing for the
clear to take away. It is also exactly what `kanban-board.tsx` already does. The
forbidden form is different in kind — it passes `null` **while a workspace is
active and other surfaces are displaying that workspace's MRs**.

### The sidebar does not become an owner

The sidebar SHALL read the store and hydrate nothing, exactly as its PR badge
does today. Consequently the sidebar MR badge is visible only when some other
mounted surface has hydrated the active workspace's MRs. That is an observation
about today's mounts, not a guarantee this spec pins, and the coverage is
**not** uniform across routes — see the two holes named below. No acceptance
criterion depends on an ambient hydration path: unit tests seed the store
directly, and the E2E scenarios below each name a route with a stated owner
(`/` via `kanban-board.tsx`, `/tasks` via this feature).

This is a deliberate parity choice. The GitHub badge on the sidebar has the same
property and the same zero owners; giving GitLab its own fifth call site would
make the two providers diverge in the one place a consolidation card has to
touch.

### Two routes where the sidebar MR badge does not render, and why both are accepted

Both are pre-existing properties of code this feature does not touch. They are
named here because a reader would otherwise assume the sidebar badge works
everywhere the sidebar does, and because the second one is a live instance of the
very pattern the previous section forbids.

**Hole 1 — an archived task opened directly.** `MRTopbarButton` is the hydration
owner on the task-detail routes, but on desktop it sits inside `{!isArchived &&
(…)}` in `components/task/task-top-bar.tsx`. An archived task opened at
`/t/:taskId` therefore mounts no MR hydration owner, and the sidebar shows no MR
badge for any task until the user navigates to a route that has one.

**Hole 2 — a task with no GitLab repository, on the task-detail route.**
`useExternalVcsFileLinkHydration`
(`hooks/domains/workspace/use-external-vcs-file-link.ts`) calls

```ts
useWorkspaceMRs(providers.has("gitlab") ? workspaceId : null);
```

and `components/task/task-page-content.tsx` invokes it unconditionally.
`TaskPageContent` is rendered by `KanbanTaskShell` for `/t/:taskId` and
`/tasks/:id`. For a task whose linked repositories carry no `gitlab` provider,
that expression passes `null` **while a workspace is active and the sidebar is
displaying that workspace's MRs** — which is exactly the shape "/tasks becomes an
owner" forbids, already shipped, on a sidebar-bearing route. It reaches
`resetTaskMRs()` with no argument and clears `taskMRs.byWorkspaceId` for every
workspace. `MRTopbarButton` on the same route calls `useWorkspaceMRs(workspaceId)`
and re-fetches, so the two race and the sidebar's MR badges are
**nondeterministic** on that route for a GitHub-only or repository-less task.

**Both are accepted for this card, and no acceptance criterion is weakened by
them.** Three reasons, stated so the decision is not re-litigated at Build:

1. Every AC that observes a badge is conditioned on the store already holding
   MRs — AC1 reads "**where** … the store holds at least one MR", not "wherever
   the task has an MR upstream". A wipe means the store does not hold them, so
   AC1 is satisfied vacuously rather than violated. The contract is about
   rendering what the store has, and that is what the builder must implement.
2. The fix for Hole 2 is not a fix to this feature's code. It is either scoping
   `useWorkspaceMRs`'s reset to the workspace it last fetched, or removing the
   gate at that call site — both changes to a hook four other call sites depend
   on, including ones that rely on the full clear at sign-out. That is already an
   explicit Out-of-scope entry below, and this section now names the call site
   that makes it urgent.
3. Neither hole is reachable from any scenario in this spec. Every E2E scenario
   (**E1 to E7**) runs on `/` or `/tasks`, both of which have an unconditional
   owner and neither of which mounts `TaskPageContent`. The unit scenarios
   (**U1 to U11**) render components directly and mount no route at all.

The builder SHALL NOT attempt to work around either hole from the three
production files in scope — in particular, the sidebar SHALL NOT gain its own
`useWorkspaceMRs` call to compensate, which AC7 already forbids.

## Nil, empty, and error behaviour

State each of these explicitly rather than by omission:

- **No linked MRs.** `useTaskMRs` returns `EMPTY_MRS`, `MRTaskIcon` returns
  `null`, and the row renders no MR element. The MR badge SHALL contribute **no
  element of its own** to the row in this case: no wrapper `<span>`, no
  placeholder, and no *conditional* spacing that exists only to reserve room for
  the absent badge. This is the observable form of the requirement, and AC6 is
  its test. (An earlier draft said "byte-identical layout", which named no
  baseline and no mechanism; no snapshot baseline is introduced by this feature.)

  **This rule bounds what the MR badge adds when there are no MRs. It does not
  forbid fixed spacing on the badge container.** The `/tasks` badge wrapper gains
  an unconditional `gap` class (specified under Responsive and coarse-pointer
  behaviour) so that two present badges are separated. That class is applied
  whether or not an MR badge renders, so in the no-MR case it changes nothing
  and adds no element. AC6 and AC23 are therefore not in tension, and a builder
  SHALL NOT read AC6 as a reason to leave two badges unseparated. AC23 requires
  that class unconditionally precisely so that satisfying it can never add an
  element in the no-MR case that AC6 forbids.
- **No `taskId`.** The sidebar's `TaskItem` accepts `taskId?: string`. When it is
  absent the MR branch renders nothing and SHALL NOT call `MRTaskIcon` with an
  empty-string or `undefined` id.
- **No active workspace** (`workspaces.activeId === null`). `useTaskMRs` returns
  `EMPTY_MRS`; no badge, no error, no fetch.
- **Task outside the active workspace.** `taskMRs` is workspace-keyed, so a row
  for a task belonging to another workspace renders no MR badge. This is
  inherited from `useTaskMRs` and is not corrected here.
- **Corrupted store entry** (a non-array value under `byWorkspaceId[ws][taskId]`,
  e.g. from a partial hydration). `MRTaskIcon`'s existing `Array.isArray` guard
  returns `null`. The new call sites SHALL NOT add a second, differently-shaped
  guard in front of it.
- **Hydration fetch fails.** `useWorkspaceMRs` swallows the error and clears its
  `fetchedRef` so a later workspace switch can retry. No badge, no toast, no
  error affordance, no retry loop is added by this feature.
- **Workspace switch.** `useWorkspaceMRs` calls `resetTaskMRs(workspaceId)` for
  the *incoming* workspace before fetching, so between the switch and the
  response the new workspace's rows show no MR badge. That transient blank is
  existing behaviour and is accepted, not fixed.
- **Task has a PR but no MR, or an MR but no PR.** Exactly one badge renders. The
  row SHALL NOT reserve space for the absent one.

### Boundaries and defaults

- **Zero / one / two-or-more MRs.** Zero renders nothing. One renders
  `SingleMRIcon` with `data-mr-count="1"` and `data-mr-state`. Two or more render
  `MultiMRIcon` with `data-mr-count="<n>"`, the aggregate colour and a numeric
  suffix, and no `data-mr-state`. Inherited from `MRTaskIcon`; no per-surface cap
  on the count and no overflow chip is introduced.
- **Archived tasks.** The sidebar's archived rows and the `/tasks` list with
  "Show archived" on both render through the same components, and the MR badge
  SHALL behave there exactly as the store-backed PR badge does today: present
  when the store holds MRs for the task, absent otherwise. Archival is not a
  suppression condition for either badge. (Note the asymmetry this inherits: the
  archived sidebar item builder sets `prInfo: undefined`, so the *summary
  fallback* PR badge is suppressed on archived rows while the store-backed
  badges are not. This feature neither copies nor corrects that.)
- **Terminal MRs.** A task whose only MR is merged, closed or locked still shows
  the badge, coloured by `getMRStatusColor`. Unlike the status chip, the badge
  has no open-only filter, and this feature does not add one.
- **Rows with no `taskId`** — `taskId` is optional on `TaskItem`
  (`taskId?: string`), and AC5 is grounded in that optionality, not in a specific
  caller. Stated precisely, because it is easy to assume otherwise: **every
  production caller today passes a `taskId`.** `TaskItem`'s only production
  importer is `components/task/task-switcher-row.tsx`, whose single mount passes
  `taskId={task.id}`. The sidebar's primary-nav and new-task rows render
  `AppSidebarNavItem` and `AppSidebarNewTaskItem`, which are different components
  and do not render `TaskItem` at all. So AC5 is a **defensive** contract on the
  optional prop rather than a rule about a live surface: it keeps the MR branch
  from being written in a way that breaks the moment an id-less caller appears,
  and U4 exercises it synthetically. Do not go looking for id-less rows in the
  sidebar; there are none, and their absence is not a missed surface.
- **`showContributions` has no default.** It is a required prop on
  `PrimaryTaskLine`, passed explicitly `true` by the rich path and `false` by the
  compact path. It SHALL NOT be given a default value.

## Concurrency and idempotency

- **Two surfaces, one task, same route.** The sidebar and the board (or the
  `/tasks` row) both mount a badge for the same task id. Both read the same store
  entry, so they always agree. Both emit the same
  `data-testid="mr-task-icon-<taskId>"`; see Duplicate mounts below.
- **Two hydration owners mounted at once** (board plus `/tasks` is not reachable
  today, but sidebar-bearing routes each mount one and `mr-topbar-button` can
  co-mount with `use-external-vcs-file-link`). Each `useWorkspaceMRs` instance
  keeps its own `fetchedRef`, so each issues its own `GET`. `setTaskMRs` replaces
  the whole per-workspace map, the request is idempotent, and the outcome
  converges to the last response regardless of arrival order. No coordination is
  added.
- **A store write while a row is mounted** (`setTaskMR` from a link, or
  `removeTaskMR` from an unlink) re-renders every mounted badge for that task in
  the same commit. There is no per-surface cache and therefore no staleness
  window between surfaces.
- **Re-render cost.** The badge adds exactly one `useTaskMRs` subscription per
  rendered row. Rows for tasks with no MRs SHALL NOT re-render on unrelated
  `taskMRs` writes, which is what the stable `EMPTY_MRS` identity buys. Any
  selector introduced by this feature SHALL NOT allocate a new array or object.

## Responsive and coarse-pointer behaviour

- `MRTaskIcon` uses a plain Radix `Tooltip` on both its single-MR and multi-MR
  variants. That disclosure behaviour is inherited **unchanged**: no
  `useTouchDrawer` variant, no `useHoverPopover` wiring, no tap-to-open handling
  is added on the new surfaces. The badge already ships this exact behaviour on
  the Kanban card, including on a mobile viewport
  (`e2e/tests/gitlab/mobile-mr-task-card-badge.spec.ts`), and this feature
  changes neither the component nor the trade-off. Upgrading the badge's
  coarse-pointer disclosure is its own card; see Out of scope.
- The badge SHALL NOT be gated on any Tailwind width class. It renders at every
  viewport width on both new surfaces, exactly as the PR badge beside it does.
- On a mobile viewport the added badge SHALL NOT introduce horizontal document
  overflow on the `/tasks` list. Both new badges sit in `min-w-0` flex rows whose
  siblings already truncate.
- The `/tasks` row is a click target. The PR badge is already wrapped in a
  `<span>` that stops `click` and `pointerdown` propagation so hovering or
  tapping the badge cannot navigate the row. The MR badge SHALL sit inside that
  **same** wrapper rather than get a second copy of it, so the two badges cannot
  drift apart on interaction behaviour.
- **That wrapper carries no `className` at all today**, because it has only ever
  held a single child, so the row's own `gap-2` was the only separation it needed.
  Two children inside it would render with **no separation whatsoever**. The
  wrapper SHALL therefore be given `inline-flex items-center gap-1`.
  `gap-1` is not a new number: it is what both surfaces that already render badges
  side by side use — the Kanban card's `kanban-card-title-row`
  (`flex items-center gap-1 min-w-0`, where `PRTaskIcon` and `MRTaskIcon` are
  direct siblings) and the sidebar title row (`flex items-center gap-1 min-w-0`).
  Matching it keeps PR-to-MR separation identical on all three surfaces.
  Two things this deliberately does NOT do: it does not add `shrink-0` (both
  `PRTaskIcon` and `MRTaskIcon` already carry `shrink-0` on their own roots), and
  it does not alter the row's existing `gap-2` between the title and the badge
  wrapper. The class is unconditional, so it reserves no space for an absent
  badge; see Nil, empty, and error behaviour for why that satisfies AC6.
  **AC23 is this rule's test.** It is a separate criterion rather than a clause on
  AC9 because AC9 observes only which elements sit inside the wrapper and in what
  order, so it cannot fail on an unclassed wrapper.
- The sidebar row's badges are not wrapped today and SHALL NOT become wrapped:
  clicking a sidebar row selects the task, which is the desired outcome from
  anywhere on the row including the badges.

## Accessibility and duplicate mounts

- `MRTaskIcon` carries no `aria-label` and this feature adds none. Doing so would
  be new user-facing copy on a component whose Kanban mount would then disagree
  with its two new mounts, and the state is already named in the tooltip text
  (`getMRTooltip`), not conveyed by colour alone.
- `MRTaskIcon`'s `data-testid` is `mr-task-icon-<taskId>`, and the same id is now
  emitted by up to two mounted rows for one task on one route (sidebar plus
  board, or sidebar plus `/tasks` row). Every Playwright accessor for it
  therefore SHALL be scoped to a container: the sidebar root
  (`app-sidebar`) or the specific row (`tasks-list-row`, `task-card-<id>`). No
  accessor may resolve `mr-task-icon-*` globally, and none may paper over the
  duplicate with `.first()`. The identical hazard already exists for
  `pr-task-icon-*` and is recorded in `e2e/tests/pr/pr-status-badge.spec.ts`.
- The two badges SHALL remain separate elements with separate tooltips. They are
  not merged into a combined contribution badge.

## Prop naming

`PrimaryTaskLine`'s existing `showPullRequest: boolean` gates the badge slot on
the `/tasks` row. It is file-local (declared and passed only within
`rich-task-list-row.tsx`) and is passed `true` from the rich content path and
`false` from the compact path.

**One flag SHALL gate both badges**, and it SHALL be renamed to
`showContributions`. There is no second flag and no independent per-provider
toggle: a user who turns on "Show task details" is asking for the row's remote
contributions, not for GitHub's specifically. Leaving a flag named
`showPullRequest` in control of a merge-request badge is the kind of stale name
that the next reader has to disprove.
