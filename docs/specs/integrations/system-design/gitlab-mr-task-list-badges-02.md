---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITLAB-MR-TASK-LIST-BADGES-001
created: 2026-08-12
owners:
  - tbd
---
# GitLab MR Badge on the Sidebar and Tasks-List Rows System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-INTEGRATIONS-GITLAB-MR-TASK-LIST-BADGES-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-GITLAB-MR-TASK-LIST-BADGES-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Internationalization

This feature introduces **no** user-facing string literal. `MRTaskIcon` renders
MR-derived data (`getMRTooltip` builds from `mr_iid`, `mr_title`, `state`,
`approval_state`, `pipeline_state`), which the component already documents as
non-translatable domain data with the same precedent as GitHub's `getPRTooltip`.

Consequences, stated so they are not re-derived:

- `apps/web/src/locales/en/*.json` SHALL NOT gain a key.
- `apps/web/eslint.i18n.options.mjs` SHALL NOT gain an entry. Of the three
  touched files, **two are already on `i18nGuardFiles`** —
  `app/tasks/rich-task-list-row.tsx` and `app/tasks/tasks-page-client.tsx` — so
  `i18next/no-literal-string` is a whole-file **error** on both, not merely a
  changed-lines ratchet. `components/task/task-item.tsx` is **not** on the list,
  so only the ratchet's changed-lines judgement applies there. This feature adds
  no literal to any of the three, so the append-in-the-same-PR rule is not
  triggered on any of them. Adding an unrelated file to the list is a separate
  change and SHALL NOT be smuggled in here.
  (`components/kanban-card-content.tsx` is also on the list, but it is not one of
  the touched files — it does not change; see Surfaces and mount points.)
- `pnpm run i18n:check` and `pnpm run i18n:ratchet` SHALL pass, and the ratchet's
  changed-lines judgement SHALL find nothing on the lines this feature touches.

## Failure modes

| Failure | Observable behaviour | Contract |
|---|---|---|
| GitLab not configured for the workspace | `listWorkspaceTaskMRs` returns an empty map; no badge anywhere | No error UI, no banner, no console noise |
| MR hydration request fails | No MR badge; PR badge unaffected | Row renders, retry only on a later workspace switch or remount |
| Store holds a non-array for the task | No MR badge | `MRTaskIcon`'s existing guard; no crash |
| Task's only MR is unlinked while the row is mounted | Badge disappears in the same commit | `removeTaskMR` filters in place; no stale badge |
| Task's last MR merges | Badge stays, coloured purple (`merged`) | Terminal MRs remain visible on the badge, unlike the status chip |
| Sidebar mounted on a route with no MR hydration owner | No MR badge for a GitLab-only task | Stated, accepted, not an AC (see Hydration ownership) |
| Archived task opened at `/t/:taskId` (desktop) | No MR badge in the sidebar, because `MRTopbarButton` is inside `{!isArchived && …}` and no owner mounts | Hole 1: stated, accepted, not an AC; pre-existing, no file in scope changes it |
| Task with no `gitlab`-provider repository opened at `/t/:taskId` or `/tasks/:id` | Sidebar MR badges nondeterministic for every task: `useExternalVcsFileLinkHydration` passes `null` and wipes `byWorkspaceId` while `MRTopbarButton` re-fetches | Hole 2: stated, accepted, not an AC. AC1 is conditioned on the store holding MRs, so it is satisfied vacuously. Fix belongs to the `useWorkspaceMRs(null)` reset-scoping card (Out of scope) |

## Acceptance criteria

Sidebar row:

- **AC1** — Where a sidebar task row has a `taskId` and the store holds at least
  one MR for that task in the active workspace, the row SHALL render
  `MRTaskIcon` for that task id.
- **AC2** — Where a sidebar task row's task has both a store PR and at least one
  MR, the row SHALL render both badges, with the PR badge preceding the MR badge
  in DOM order.
- **AC3** — Where a sidebar task row's task has an MR and no PR (neither a store
  PR nor a `prInfo` summary projection), the row SHALL render the MR badge.
- **AC4** — Where a sidebar task row's task has a `prInfo` summary projection, no
  store PR, and at least one MR, the row SHALL render the `prInfo` fallback PR
  badge followed by the MR badge.
- **AC5** — When a sidebar task row is rendered without a `taskId`, the row SHALL
  render no MR badge and SHALL NOT invoke `MRTaskIcon`.
- **AC6** — Where a sidebar task row's task has no MRs, the row's rendered output
  SHALL contain no element whose `data-testid` starts with `mr-task-icon-`.
- **AC7** — The sidebar SHALL NOT call `useWorkspaceMRs`, and no file under
  `apps/web/components/task/` or `apps/web/components/app-sidebar/` SHALL import
  it.

`/tasks` rows:

- **AC8** — Where the `/tasks` row is rendered with contributions shown and the
  store holds at least one MR for the task in the active workspace, the row SHALL
  render `MRTaskIcon` for that task id.
- **AC9** — Where the `/tasks` row is rendered with contributions shown and the
  task has both a PR and an MR, both badges SHALL render, PR before MR in DOM
  order, and both SHALL be inside the single propagation-stopping wrapper.
- **AC10** — While the `/tasks` row is rendered with contributions hidden (the
  compact path), the row SHALL render neither the PR badge nor the MR badge.
- **AC11** — The single boolean that gates both badges SHALL be named
  `showContributions`; no `showPullRequest` identifier SHALL remain in
  `apps/web/app/tasks/rich-task-list-row.tsx`.
- **AC23** — Where the `/tasks` row is rendered with contributions shown, the single
  propagation-stopping wrapper holding the badges SHALL carry the classes
  `inline-flex items-center gap-1`, and SHALL carry them unconditionally: the classes
  are present whether or not an MR badge renders, and SHALL NOT be applied only when
  two badges are present. This pins the wrapper class stated in Responsive and
  coarse-pointer behaviour, which AC9 alone does not observe: AC9 constrains only which
  elements sit inside the wrapper and in what order, so an implementation leaving the
  wrapper unclassed would satisfy every other criterion while shipping two touching
  badges on `/tasks` and separated badges on the other two surfaces. The
  unconditionality is what keeps this compatible with AC6 rather than in tension with
  it; see Nil, empty, and error behaviour.

`/tasks` hydration:

- **AC12** — When `TasksPageClient` renders with an active workspace, it SHALL
  call `useWorkspaceMRs` with that workspace id, unconditionally, and SHALL NOT
  gate the argument on `tasksListShowDetails` or on any other setting.
- **AC13** — If `useWorkspaceMRs` is invoked with `null` while another
  workspace's MRs are cached, then every workspace's entry under
  `taskMRs.byWorkspaceId` SHALL be cleared. This is the existing behaviour AC12
  exists to avoid triggering, and SHALL be pinned by a test that seeds two
  workspaces.
- **AC21** — Where `TasksPageClient` renders with no active workspace, passing
  `null` to `useWorkspaceMRs` is permitted, and no badge SHALL render on any
  surface for the duration.

Non-regression:

- **AC14** — No file under `apps/web/components/github/` SHALL be modified.
- **AC15** — `apps/web/components/gitlab/mr-task-icon.tsx` SHALL NOT be
  modified. The badge is consumed exactly as it ships.
- **AC16** — The Kanban card's rendered badge output SHALL be unchanged, and
  `e2e/tests/gitlab/mr-task-card-badge.spec.ts` SHALL pass unmodified.
- **AC17** — No `apps/web/src/locales/**` file and no `eslint.i18n.options.mjs`
  entry SHALL change; `pnpm run i18n:check` and `pnpm run i18n:ratchet` SHALL
  pass.
- **AC18** — No selector or hook added by this feature SHALL return a freshly
  allocated array or object; MR reads SHALL go through `useTaskMRs`.

Cross-cutting:

- **AC19** — Every Playwright accessor introduced for `mr-task-icon-*` SHALL be
  scoped to `app-sidebar`, `tasks-list-row`, or `task-card-<id>`, and SHALL NOT
  use `.first()` to disambiguate duplicates.
- **AC20** — On a mobile viewport, a `/tasks` row carrying both badges SHALL NOT
  cause horizontal document overflow.
- **AC22** — Where a sidebar task row's task has at least one MR and the row also
  renders the issue badge (`issueInfo` present), the MR badge SHALL precede the
  issue badge in DOM order. This pins the "PR, MR, issue" grouping stated in
  Selection and ordering, which AC2 alone does not observe: AC2 constrains only
  PR-versus-MR, so an implementation appending the MR badge after
  `IssueTaskIcon` would satisfy every other criterion while violating the stated
  row order.

## Scenarios

Scenarios are numbered `U<n>` for unit and `E<n>` for E2E, in two independent
sequences. The two kinds are deliberately not numbered in one run: an earlier
draft numbered them continuously, and inserting a unit scenario silently shifted
every E2E number and left a stale cross-reference behind. Cite scenarios by these
prefixed ids, never by bare number.

### Unit (vitest) — U1 to U11

- **U1 — Sidebar, PR and MR together** — seed `taskPRs.byTaskId["t1"]` with one PR
  and `taskMRs.byWorkspaceId["ws1"]["t1"]` with one MR, `workspaces.activeId =
  "ws1"`; render `TaskItem` with `taskId="t1"`. Both `pr-task-icon-t1` and
  `mr-task-icon-t1` are present, and `compareDocumentPosition` puts the PR
  first. (AC2)
- **U2 — Sidebar, MR only** — no PR, no `prInfo`, one MR. `mr-task-icon-t1`
  present, `pr-task-icon-t1` absent. This is the case the early return currently
  makes impossible. (AC3)
- **U3 — Sidebar, `prInfo` fallback plus MR** — no store PR, `prInfo={{number: 7,
  state: "Open"}}`, one MR. Both render, PR first. (AC4)
- **U4 — Sidebar, no `taskId`** — render `TaskItem` without `taskId`, with MRs
  seeded for some other id. Assert **both** of AC5's clauses, because they are not
  equivalent and the weaker one alone is satisfied by code that violates the AC:
  (a) no `mr-task-icon-*` element is present, and (b) **`MRTaskIcon` is not
  invoked at all**, asserted by mocking the module and checking zero calls.
  Clause (b) is the load-bearing one. An unguarded `<MRTaskIcon taskId={taskId!} />`
  — the same non-null-assertion shape the PR branch beside it already uses — would
  invoke the component with `undefined`, hit `useTaskMRs`' internal guard, render
  `null`, and **pass clause (a) while breaking AC5** and adding the store
  subscription AC18 exists to prevent.
  **Clause (b) needs a spy, and no in-repo precedent for one exists — write it
  fresh.** `components/kanban-card-content.test.tsx` is the precedent for
  **module-level interception only**: it replaces the module with a plain stub
  (`MRTaskIcon: () => <span data-testid="mr-task-icon" />`), and a stub records no
  calls, so clause (b) cannot be asserted against it. No test under
  `apps/web/components/` mocks a component module with `vi.fn()` and then asserts a
  call count, so there is nothing to copy for the assertion itself. The required
  shape is a `vi.fn()`-backed component mock plus a zero-call assertion — mock the
  module with something of the form
  `vi.mock("@/components/gitlab/mr-task-icon", () => ({ MRTaskIcon: vi.fn(() => null) }))`,
  import the mocked symbol, and assert `expect(MRTaskIcon).not.toHaveBeenCalled()`.
  Copy `kanban-card-content.test.tsx` for the `vi.mock` placement and factory shape;
  do NOT copy its stub body, which is what makes clause (b) unassertable. (AC5)
- **U5 — Sidebar, no MRs** — PR only. No `mr-task-icon-*` element. (AC6)
- **U6 — `/tasks` row, contributions shown** — seeded PR and MR,
  `showContributions` true. Both badges render inside the single wrapper, PR
  first, and the wrapper carries `inline-flex items-center gap-1`. (AC8, AC9, AC23)
- **U7 — `/tasks` row, contributions hidden** — same store, compact path. Neither
  badge renders. (AC10)
- **U8 — `useWorkspaceMRs(null)` wipes every workspace** — seed
  `byWorkspaceId["ws-a"]` and `byWorkspaceId["ws-b"]`, render the hook with
  `null`, assert both are gone. Add to
  `apps/web/hooks/domains/gitlab/use-task-mr.test.ts` if not already covered
  there. (AC13)
- **U9 — `TasksPageClient` hydration argument** — with `tasksListShowDetails`
  false and an active workspace, `useWorkspaceMRs` is called with the workspace
  id, not `null`. (AC12)
- **U10 — Sidebar, MR before the issue badge** — one MR seeded and
  `issueInfo={{url: "…", number: 42}}` passed. Both `mr-task-icon-t1` and the
  issue badge render, and `compareDocumentPosition` puts the MR badge first.
  Run it with a store PR present as well, so the assertion covers the full
  "PR, MR, issue" order rather than just the adjacent pair. (AC22)
- **U11 — No active workspace, no badge** — seed
  `taskMRs.byWorkspaceId["ws1"]["t1"]` with one MR but set
  `workspaces.activeId = null`; render `TaskItem` with `taskId="t1"`. No
  `mr-task-icon-*` element is present. This observes AC21's second clause, which
  no other scenario reaches: AC21 is otherwise contract text resting on
  `useTaskMRs` returning `EMPTY_MRS` for a null workspace, with nothing pinning
  it. (AC21)

### E2E (Playwright, required)

`apps/web/**` is touched and none of the files are on the Testing step's
exemption allowlist, so E2E is required. All GitLab scenarios seed through the
GitLab mock provider and a task created with **no agent**, for the reason
`mr-task-card-badge.spec.ts`'s `seedBoardTask` comment records: an auto-started
session moves the card out from under the assertion.

**Where the seed helpers come from, stated so it is not invented at build time.**
`nextMRIID`, `seedMR`, `linkMR`, `ensureGitLabConfigured` and `seedBoardTask` are
file-local and **unexported** in `mr-task-card-badge.spec.ts`. Each new spec file
SHALL therefore define its own local copies, seeded from that file's versions,
and SHALL NOT import them. `mr-task-card-badge.spec.ts` is not edited to export
them, which is what keeps AC16 ("passes unmodified") literally true. Three
reasons this is duplication rather than extraction:

- It is the established in-repo precedent: `mobile-mr-task-card-badge.spec.ts`
  already writes its own `seedBoardTaskWithMR` and imports only `GITLAB_HOST` and
  `GITLAB_PROJECT` from `e2e/helpers/gitlab.ts`.
- It does not trip lint, **because ESLint rules are per-file**. An earlier draft
  justified this by claiming `e2e/**` sits in the `ignores` of the rule block
  carrying `sonarjs/no-identical-functions`; that is **wrong**, and the correct
  mechanism matters because it is narrower. In `apps/web/eslint.config.mjs`:
  the block carrying `"sonarjs/no-identical-functions": "warn"` has no `files` and
  no `ignores` (it is the global block, so it applies to `e2e/**` too); the
  `ignores: ["**/*.test.ts", "**/*.test.tsx", "e2e/**"]` belongs to the **i18n
  guard block** (`files: i18nGuardFiles`, rule `i18next/no-literal-string`); and
  the `files: ["e2e/**/*.ts"]` override disables `react-hooks/*`, `max-lines`,
  `max-lines-per-function` and `sonarjs/no-duplicate-string` — but **not**
  `sonarjs/no-identical-functions`. The rule is therefore live on e2e files. It
  still cannot fire on this duplication, because ESLint only ever sees one file at
  a time and so cannot compare a helper in one spec file with its copy in another.
  **Consequence the builder must respect:** two identical functions **within a
  single** spec file DO warn, and `pnpm --filter @kandev/web lint` runs
  `eslint --max-warnings 0`, so that fails the build. Copy each helper across
  files freely; do not write two copies of the same helper inside one file.
- Extracting them into `e2e/helpers/gitlab.ts` would edit
  `mr-task-card-badge.spec.ts`'s imports, which AC16 forbids, and consolidating
  the GitLab e2e seed helpers is a separate cleanup; see Out of scope.

Shared *constants* are a different matter: `GITLAB_HOST` and `GITLAB_PROJECT`
SHALL be imported from `e2e/helpers/gitlab.ts` as the existing specs do, not
re-declared.

**Where the mock GitHub PR comes from.** Three scenarios (E3, E6, E7) need a task
carrying a PR *and* an MR, and the GitHub half has its own seeding path that is not
covered by the GitLab helpers above. It SHALL come from the existing mock-GitHub
API-client methods, not from a new fixture: `apiClient.mockGitHubReset()` and
`apiClient.mockGitHubSetUser()` to put the provider in a known state, then
`apiClient.mockGitHubAssociateTaskPR()` to attach a PR **to the task**.

**`mockGitHubAddPRs()` is the wrong call and SHALL NOT be used for these three
scenarios.** It is named here only so the mistake is not re-derived: it posts to
`POST /api/v1/github/mock/prs`, which seeds the mock provider's PR *catalogue* (what
pickers and listings read) and creates **no task-to-PR association**. A scenario that
uses it leaves `taskPRs.byTaskId[taskId]` empty, so `PRTaskIcon` renders nothing and
the PR-before-MR assertion that is the entire point of E3, E6 and E7 can never run.
`mockGitHubAssociateTaskPR()` posts to `POST /api/v1/github/mock/task-prs` and is the
call that populates the association the badge reads. No catalogue call is needed
alongside it; the association alone is sufficient.

Its required arguments are `task_id`, `owner`, `repo`, `pr_number`, `pr_url`,
`pr_title`, `head_branch`, `base_branch` and `author_login`; `state` is optional and
SHALL be passed explicitly so the seeded PR's state is not left implicit.

The reference is `e2e/tests/gitlab/mr-task-card-badge.spec.ts`'s own test
`"AC30/AC37: a linked PR and MR both render, PR before MR, and the badge tooltip names
a non-open state"`. It is the exact precedent for all three scenarios: it calls
`mockGitHubReset()`, then `mockGitHubSetUser("test-user")`, then
`mockGitHubAssociateTaskPR({ task_id, owner: "testorg", repo: "testrepo",
pr_number: 900, pr_url, pr_title: "Companion PR", head_branch: "feat/companion",
base_branch: "main", author_login: "test-user", state: "open" })`, and asserts
PR-before-MR order by `compareDocumentPosition` on the Kanban card. Reading that file
does not modify it, so AC16 stays literally true.

Neither `mr-task-card-badge.spec.ts` nor `e2e/tests/pr/pr-status-badge.spec.ts` is
edited or imported from — the same local-copy rule as the GitLab helpers applies, and
for the same AC16 reason. This is stated because the spec is otherwise precise about
GitLab seeding and would otherwise leave three of seven E2E scenarios half-specified.

New file `apps/web/e2e/tests/gitlab/mr-sidebar-badge.spec.ts`:

- **E1 — Sidebar shows the badge** — on `/` (board mounts the hydration owner), a
  task with one linked open MR. Scoped to `app-sidebar`, the row located by
  `[data-task-row-id="<taskId>"]`, `mr-task-icon-<taskId>` is visible with
  `data-mr-count="1"` and `data-mr-state="open"`. (AC1, AC19)
- **E2 — Sidebar shows no badge without an MR** — same route, a task with no
  linked MR. Inside `app-sidebar`, `mr-task-icon-<taskId>` has count 0. (AC6)
- **E3 — Sidebar shows PR before MR** — a task with both a mock GitHub PR (seeded
  per "Where the mock GitHub PR comes from" above) and a linked MR; inside the
  sidebar row, both badges visible and PR precedes MR by
  `compareDocumentPosition`. (AC2)

New file `apps/web/e2e/tests/gitlab/mr-tasks-list-badge.spec.ts`:

- **E4 — `/tasks` rich row shows the badge** — navigate to `/tasks`, enable
  "Show task details" through the `display-button` menu as
  `task-listing-view-preferences.spec.ts` does, locate the `tasks-list-row` by
  title, assert `mr-task-icon-<taskId>` inside that row. (AC8, AC19)
- **E5 — `/tasks` compact row shows neither badge** — same task, details off:
  inside the row, both `pr-task-icon-<taskId>` and `mr-task-icon-<taskId>` have
  count 0. (AC10)
- **E6 — `/tasks` row shows PR before MR** — task with both (mock GitHub PR seeded
  as above); inside the row, PR precedes MR. (AC9)

New file `apps/web/e2e/tests/gitlab/mobile-mr-tasks-list-badge.spec.ts`:

- **E7 — Mobile `/tasks` row** — mobile viewport, details on, task with both
  badges (mock GitHub PR seeded as above): the MR badge is visible and
  `assertNoDocumentHorizontalOverflow` passes. Mirrors
  `mobile-mr-task-card-badge.spec.ts`. (AC20)

All seven E2E scenarios (E1 to E7) run on `/` or `/tasks`. None mounts
`TaskPageContent`, so none is exposed to either hydration hole named above.

## Out of scope

Each is a deliberate exclusion, not an oversight.

- **A `merge_request` field on `TaskStatusSummary`, and an MR analogue of the
  sidebar's `prInfo` fallback.** The summary projection is produced by the
  backend and carries `pull_request` only. Adding a GitLab counterpart is a
  backend contract change (projection, WS delivery, bounded-summary spec) that
  cannot be tested from `apps/web/` alone. Consequence, stated plainly: on a
  route with no MR hydration owner the sidebar can show a PR badge from the
  summary while showing no MR badge for the equivalent GitLab task. That gap is
  this card's, and it is left open on purpose.
- **An MR analogue of `TaskItemStatsRow`'s `#<number>` text.** Same backend
  dependency, same reason.
- **Giving GitLab MR hydration a single provider-level owner.** Already an
  out-of-scope card on [gitlab-mr-status-chip](../requirements/gitlab-mr-status-chip.md);
  this feature adds one page-level owner where its own badge needs it and
  consolidates nothing.
- **Making `useWorkspaceMRs(null)` scope its reset to the workspace it last
  fetched.** That is the correct long-term fix for the trap AC12 routes around,
  but it changes a hook four other call sites depend on, including two that rely
  on the full clear at sign-out. It needs a card that can test all of them.
  That card also owns **Hole 2** above: the already-shipped gated call in
  `hooks/domains/workspace/use-external-vcs-file-link.ts`
  (`useWorkspaceMRs(providers.has("gitlab") ? workspaceId : null)`), which makes
  the sidebar MR badge nondeterministic on `/t/:taskId` and `/tasks/:id` for a
  task with no GitLab repository. Two candidate fixes exist — scope the reset, or
  drop the gate at that call site and let the hook no-op for a workspace with no
  MRs — and choosing between them requires testing all four call sites, so it is
  not decided here.
- **Fixing Hole 1** (desktop `MRTopbarButton` sitting inside `{!isArchived && …}`
  in `components/task/task-top-bar.tsx`, so an archived task's detail route
  mounts no MR hydration owner). Changing what the archived top bar renders is a
  task-detail-surface decision with its own parity questions on the GitHub side.
- **Consolidating the GitLab e2e seed helpers** (`seedMR`, `linkMR`,
  `ensureGitLabConfigured`, `seedBoardTask`) into `e2e/helpers/gitlab.ts`. It is
  worth doing, and it would edit `mr-task-card-badge.spec.ts`, which AC16
  forbids here. It is a test-only cleanup card covering every GitLab spec at
  once, not a rider on this one.
- **Upgrading `MRTaskIcon`'s coarse-pointer disclosure** to a drawer or a
  tap-to-open popover. The Radix tooltip is inherited as-is, and changing it
  would change the shipped Kanban badge on every viewport.
- **Re-tuning GitLab MR status colours, the multi-MR aggregate, or
  `aggregateMRStatusColor`'s array-order tie.** Frozen verbatim.
- **Merging the PR and MR badges into one combined contribution badge, or
  collapsing them behind a count.** Two providers, two badges.
- **The Azure DevOps row badge.** `components/azure-devops/` is not touched, and
  the DOM-order ACs name only PR and MR.
- **Office task surfaces and any other task row that renders no `PRTaskIcon`
  today.** Only the two surfaces named in Surfaces and mount points gain a badge.
  (The sidebar's archived rows are *not* excluded — they render through the same
  `TaskItem` and are covered by the boundary rule above.)
- **Any change to which tasks appear in the sidebar or on `/tasks`**, to their
  ordering, grouping, filtering, or pagination.
