---
created: 2026-09-22
status: done
requirements:
  - REQ-PLUGINS-TASKLIST-FACETS-001
  - REQ-PLUGINS-TASKLIST-FACETS-002
  - REQ-PLUGINS-TASKLIST-FACETS-003
system_design:
  - ../../specs/plugins/system-design/task-list-facets.md
legacy_specs: []
---

# Implementation Plan: Plugin Task List Facets

## Overview

Complete the host half of plugin task-list facets so a registered facet is a
first-class `/tasks` Sort and Group dimension: selectable on desktop and phone,
sectioned per value with a trailing unassigned bucket, ordered page-locally, and
persisted in the existing sort and group preference so the choice survives a
reload, a shared link, and a plugin that is temporarily inactive.

The design package is
[`docs/specs/plugins/system-design/task-list-facets.md`](../../specs/plugins/system-design/task-list-facets.md)
and
[`docs/specs/plugins/requirements/task-list-facets.md`](../../specs/plugins/requirements/task-list-facets.md).
The persistence boundary is recorded in
[ADR-2026-09-22-plugin-facet-selection-persistence](../../decisions/2026-09-22-plugin-facet-selection-persistence.md).

## Continuation context: what already ships

The contribution point was merged for `kdlbs/kandev` in
`feat(plugins): plugin task list facets for sort and group (#2932)`. Re-verified
against the current tree:

- **Contribution contract and registry** - `TaskListFacetValue` and
  `TaskListFacetRegistration` exist in `apps/packages/plugin-sdk/src/index.ts`;
  `apps/web/lib/plugins/registry.ts` holds `taskListFacets`, rejects a non-slug
  and duplicate id, exposes `getTaskListFacets()` and the `forPlugin` scoped
  `registerTaskListFacet`, cleans up in `unregisterPlugin`, and counts facets in
  the private `totalCount()` that gates its change notification. `registry.test.ts`
  covers the id rules, the owner pairing, `getTaskListFacets()`, and teardown, but
  no test asserts that a facet registration or removal triggers the notification;
  task 01 closes that gap through the public subscriber, not through the private
  count.
- **Options** - `tasks-list-controls.tsx` and
  `mobile-menu-task-list-options.tsx` append facet options after the built-in
  ones and render the plugin label verbatim.
- **Grouping and sorting** - `tasks-list-view.tsx` builds one section per value,
  keeps a value a task does not carry out of the list, rebuilds the tree per
  section, renders the color swatch, keeps a plugin value named `untagged`
  separate from the host bucket, and `sortTasksByFacet` orders by the first value
  label with unassigned last.
- **Live updates** - `useTaskListFacets` subscribes, catches throwing callbacks,
  and re-resolves values on a revision bump.

The remaining gaps, each verified in the current tree:

1. The facet value the host stores is not a shape the backend accepts.
   `internal/user/models/tasks_list_preferences.go` validates both enums against
   a closed list, so `?group=facet:...` normalizes to `state`, the store's
   `Normalize*` calls discard it, and `service.applyTasksListPreferences`
   rejects a facet write with a 400.
2. `parseTasksListSort` / `parseTasksListGroup` reject the facet form, so a
   deep link and the round-trip through the user-settings API cannot carry a
   facet selection.
3. `useTaskListFacetSelection` has no persistence path at all: the selection
   lives only in component state (`useState<string | null>`), so nothing
   restores a stored facet, and its absence check nulls the state rather than
   falling back. The new persistence must survive a facet key that is
   temporarily unresolvable instead of being cleared by that check.
4. The unassigned bucket is sorted among the value sections and labelled with
   `tasks:ungrouped`, not placed last with `sidebar:groupUnassigned`.
5. The mobile group help copy
   (`kanban:groupTasksIntoSectionsByState`) still enumerates only state,
   workflow, repository, and none.
6. The published contract contradicts the request:
   `docs/plans/plugins/PLUGIN-API.md` states that "no facet selection is
   persisted or sent to the backend", and neither it nor
   `plugins-authoring.md` mentions the persisted-selection outcome. The
   capability also had no specification before this package, which is why one is
   created here rather than as a work order.
7. Section building lives inside `tasks-list-view.tsx` and is not directly
   unit-testable; the rendered unassigned bucket is already asserted by two
   component tests, but its label and its trailing position are not, and no test
   covers a value that no loaded task carries or a per-section tree shape.
8. The fixture facet reads `window.__e2eFacetValues`, and the two e2e specs
   assert a single value and a single section. Neither the repeated multi-label
   row, the unassigned bucket, nor a live re-section is covered.

## Scope

### In scope

- Complete the public facet contract with a named context type, and keep
  `PLUGIN-API.md` and `plugins-authoring.md` describing the shipped behavior and
  their selector lists current.
- Accept `facet:<pluginId>:<facetId>` in the stored sort and group preference by
  shape, segment by segment, in the model, the service, the store, and the
  boot-state route.
- Resolve every live task-list query to a built-in sort: the browser refetch, the
  task-list HTTP endpoint's `sort` gate (which currently normalizes through the
  widened predicate and would forward a facet), and the Go boot handler, which
  must split its one resolved value into a built-in sort for the first-page query
  and the requested value for the boot payload.
- Separate the requested selection from the effective selection in the frontend
  so an unavailable facet falls back without overwriting the stored value, and
  resolve the query sort to a built-in value at the client query site.
- Discard a returned value whose label is empty, per entry, at both layers that
  read it: `resolveTaskFacetValues` drops it so the sorter's falsy label test and
  the builder agree, `firstFacetLabel` skips it so `sortTasksByFacet` satisfies
  the rule on its own input, and `buildTaskSections` ignores it so a raw values
  map through the view's props cannot produce a section titled with the empty
  string. A task whose values are all empty-label counts as returning no value
  (`AC-PLUGINS-TASKLIST-FACETS-002.8`).
- Trim before matching in both widened client parsers, so the client resolves a
  padded value the same way the server does.
- Place the unassigned bucket last with the shared unassigned copy, and update
  the phone group help copy in every shipped locale.
- Extract section building so multi-membership, absent values, the unassigned
  bucket, and per-section trees are unit-testable.
- Drive the fixture facet from a workspace user-state document and prove the
  desktop and phone outcomes in Playwright.

### Out of scope

- Server-side filtering or ordering by a facet value.
- Facet contributions on the kanban board, the sidebar, or task rows.
- New storage for facet values; a consumer keeps its own values in plugin
  storage.
- Changes to the released tags plugin, its manifest, or its bundle.
- A backend enumeration of installed facets.

## Technical approach

### Preference shape (backend)

`tasks_list_preferences.go` gains the facet prefix and a shape check applied to
the two segments separately: the plugin-id segment follows the manifest identity
grammar (`^[a-z0-9][a-z0-9._-]*$`, so dots and underscores are accepted and a
colon is not), and the facet-id segment follows the registry's
`^[a-z0-9]+(?:-[a-z0-9]+)*$` rule. `IsValidTasksListSort` /
`IsValidTasksListGroup` accept a built-in value or the facet form;
`NormalizeTasksListSort` / `NormalizeTasksListGroup` keep an accepted facet value
and still return the built-in default for anything else. The service's rejection
message must name both accepted forms instead of listing only built-ins. The
package also gains an exported query-side resolver, `TasksListApiSort`, that
tests the facet-shaped predicate `IsTaskListFacetValue` (never
`IsValidTasksListSort`, which accepts facet values) and otherwise defers to
`NormalizeTasksListSort`; both the predicate and the resolver trim before
matching, because the shipped helpers trim and an untrimmed predicate would let a
padded facet value through as "not a facet, therefore normalized". The boot
handler's task query and the task-list HTTP
handler's `sort` gate use it, while the boot payload's `tasksListSort` field and
the two preference writes keep the requested value. That split matters because
`NormalizeTasksListSort` delegates to `IsValidTasksListSort`: widening the
predicate alone would let `?sort=facet:…` reach the order-by input on the list
endpoint. `store/sqlite.go` keeps calling the normalizers.

### Selection resolution (frontend)

`tasks-list-options.ts` gains `encodeTaskListFacetOption`,
`parseTaskListFacetOption`, widened `parseTasksListSort` / `parseTasksListGroup`,
and `resolveTasksListApiSort`. The widened parsers trim surrounding whitespace
before matching, closing the client/server asymmetry that
`AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.3` forbids. `useTaskListFacetSelection`
keeps the requested
value from the query or preference and derives the effective value: a facet is
effective only while its key is in the resolved facet list, otherwise the
effective value is the built-in default. The controls and the list bind the
effective value; the query and the stored preference keep the requested value.
`useTaskOperations` requests `resolveTasksListApiSort(requestedSort)`. Together
with the boot handler's split above, no live task-list query carries a facet
identifier. `apps/web/app/tasks/page.tsx` is a non-executing Next-era twin (no
importer, no `next` dependency, the live route is `apps/web/src/spa-routes.tsx`),
so it is only kept compiling; it is not a query site this plan constrains.

### Grouping and copy

`buildTaskSections`, `buildTaskTree`, `groupForTask`, `compareSection`,
`flattenTaskTree`, and the section-key helpers move to
`apps/web/lib/tasks/tasks-list-sections.ts` with a translate function injected
by the caller (both `buildTaskSections` and `groupForTask` read host copy). The facet branch sorts value buckets by label, then appends the
unassigned bucket last under `sidebar:groupUnassigned`; the existing
`facet:value:<value>` and `facet:host:ungrouped` keys are preserved so a plugin
value cannot collide with the host bucket. `tasks:ungrouped` disappears with its
last caller, and `kanban:groupTasksIntoSectionsByState` names plugin-provided
values in every catalog: `en`, `pt-pt`, `zh-cn`, and `ja` are hand-edited, `zh-hk`
and `zh-tw` come from `pnpm run i18n:zh-hant`, and `pseudo` comes from
`pnpm run i18n:pseudo` rather than a hand edit.

### Fixture and evidence

The fixture facet hydrates a workspace-scoped user-state document
(`host.storage.get("workspace", workspaceId, "facet-values")` plus
`host.storage.subscribe`) into a synchronous cache, following the existing Notes
panel pattern, and notifies its listeners on change. A spec drives it with the
authenticated user-state route, and the WS fan-out
(`plugin.user-state.updated`) produces the live re-section.

## UI design contract

- **Entry points:** `/tasks` toolbar on desktop; the top-bar page context opens
  the Display Options sheet on phones.
- **Mobile surface:** the existing Display Options sheet, an inset bottom
  drawer. No new surface.
- **Exemplar:** the shipped Sort and Group selectors in the same sheet; the
  facet adds option rows and a trailing section, not a new pattern.
- **Scroll owner:** the task list keeps the single page scroll; the sheet keeps
  its own internal scroll.
- **Shared state:** the requested/effective selection is shared; only
  presentation differs between the toolbar and the sheet.
- **Primary action:** choose a Group or Sort option, exactly as for a built-in
  option.
- **Mobile proof:** the existing `mobile-*.spec.ts` facet spec, extended to the
  multi-value and unassigned outcomes, plus a document-width overflow check.

### UI-01: `/tasks` grouped by an active facet (desktop)

Entry: workspace → Tasks. State: the fixture facet is active and grouping is
`facet:...`; three tasks load — one carries two values, one is its child, one
carries none.

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ Tasks                                                       [ + New Task ]   │
│                                                                              │
│        Show archived ☐   Sort [ Updated newest ▾ ]   Group [ Fixture tag ▾ ] │  toolbar (fixed)
├──────────────────────────────────────────────────────────────────────────────┤
│  ● ALPHA                                                            2 tasks  │  value section:
│    ▸ Multi value task                                                        │  swatch = value color
│      ▸ Child of multi value task                                             │  tree rebuilt inside
│  ● BETA                                                             1 task   │  the section
│    ▸ Multi value task                                                        │
│  ● UNASSIGNED                                                       1 task   │  host bucket, always
│    ▸ Unlabelled task                                                         │  last, sidebar copy
├──────────────────────────────────────────────────────────────────────────────┤
│ Rows per page [ 25 ▾ ]          1-3 of 3                  ‹  1  ›            │  footer (fixed)
└──────────────────────────────────────────────────────────────────────────────┘
```

Structural requirements: value sections ordered by label case-insensitively, the
unassigned bucket last, one row per task per matching value, the child indented
under its own parent inside its section. Illustrative: spacing, swatch size, task
counts. The scenario is one multi-value task, its child, and one unlabelled task,
which is why the sections read 2/1/1 and the footer counts three tasks while four
rows render — the footer's total comes from the API task count, not the rendered
row count. Maps to `AC-PLUGINS-TASKLIST-FACETS-002.1` through `.4`.

### UI-02: Group picker contents (desktop and phone)

State: the task list has one active facet. The built-in options come first, then
one entry per active facet, labelled by the plugin.

```text
  Group ▾                    phone: Group selector in the Display Options sheet
  ┌──────────────────┐       ┌──────────────────────────────┐
  │ State            │       │ State                        │
  │ Workflow         │       │ Workflow                     │
  │ Repository       │       │ Repository                   │
  │ None             │       │ None                         │
  │ Fixture tag      │  ← new│ Fixture tag                  │  ← same label, verbatim
  └──────────────────┘       └──────────────────────────────┘
                             help: "... by state, workflow,
                                    repository, plugin values,
                                    or none."
```

Maps to `AC-PLUGINS-TASKLIST-FACETS-001.1`, `.2`, `.4`. The help copy is
illustrative in English only; every shipped catalog carries its own translation.

### UI-03: An unavailable facet falls back without losing the selection

Entry: `/tasks` opened from a link whose query names a facet. State: the plugin
that owns it is disabled, uninstalled, or still loading, so the facet is not in
the registry. The control shows the built-in default; the query keeps the facet.

```text
  URL:      /tasks?group=facet:my-plugin:tags          ← unchanged, still holds the facet

  before (plugin active)     Group [ Fixture tag ▾ ]   sections: ALPHA | BETA | UNASSIGNED
  after  (plugin unavailable) Group [ State       ▾ ]   sections: state groups
                              ▲ built-in default, no write-back to the URL or the stored preference

  plugin becomes active again → Group [ Fixture tag ▾ ] without user action, no reload
```

Structural: the control binds the effective value while the query and the stored
preference keep the requested value; no user action is required for the return.
Illustrative: the specific group names. The Sort and Group controls sit in the
fixed `/tasks` toolbar; the section list below is the single page scroll. On
phones the same control is the Group selector inside the Display Options sheet,
which scrolls internally while the page keeps its own scroll; the fallback and
the return behave identically on both viewports. Maps to
`AC-PLUGINS-TASKLIST-FACETS-003.3` and is owned by task 03.

## Tests

- `lib/plugins/registry.test.ts` - slug rejection, duplicate rejection, owner
  pairing, `getTaskListFacets`, `unregisterPlugin` teardown through the
  observable notification bump (never the private `totalCount()`), and
  a registration shaped exactly like the released tags plugin
  (`{ id: "tags", label: "Tag", getValues: ({ taskId, workspaceId }) => [...] }`
  with a `subscribe` returning an unsubscribe).
- `lib/tasks/tasks-list-options.test.ts` - facet option encoding and parsing,
  widened preference parsing with the shared accepted/rejected examples
  including a whitespace-padded value, API sort resolution (padded facet *and*
  padded built-in), and facet ordering: first label, unassigned last, and the
  case-insensitive comparison pinned with a same-base pair inserted in the
  discriminating order — `Beta` before `beta`, where `sensitivity: "base"`
  compares equal and the default comparison would reverse them; `beta` before
  `Beta` is vacuous because both comparisons agree. The pin guards the
  comparison, not the index tie-break: `Array.prototype.sort` is stable, so the
  tie-break is unobservable and the shipped "keeps ties stable" case (three
  distinct labels) asserts no tie at all. The same pin belongs in
  the sections suite for the bucket ordering. Do not assert that a
  facet value leaves `sortTasksForList` unchanged: its default branch shares one
  body with `updated_desc`, so that assertion is true by construction. The
  invariant is documented in the design and the API-side resolution is covered by
  `resolveTasksListApiSort`. The same file covers `firstFacetLabel` skipping
  empty labels, so a task with an empty-label sibling and a usable value sorts by
  the usable value (`AC-PLUGINS-TASKLIST-FACETS-002.8`).
- `lib/tasks/tasks-list-sections.test.ts` - one bucket per carried value, absent
  values produce no bucket, multi-membership, trailing unassigned bucket with the
  `sidebar:groupUnassigned` copy, per-section trees, label ordering, swatch
  color, a duplicated value for one task (one row, not two), and an empty-label
  entry that must not produce a section (`AC-PLUGINS-TASKLIST-FACETS-002.8`).
- `hooks/use-task-list-facets.test.tsx` - key namespacing, subscription
  revision, malformed and throwing `getValues`, context arguments, and the
  per-entry discard of a value whose label is empty
  (`AC-PLUGINS-TASKLIST-FACETS-002.8`), including a task that keeps a non-empty
  sibling and one whose entries are all empty-label.
- `hooks/use-task-list-facet-selection.test.tsx` - requested versus effective
  selection, facet fallback while unavailable, restoration when the facet
  appears, built-in change path, and that a fallback never calls the preference
  writer with the built-in default, so the requested facet stays the value that
  would be persisted.
- `app/tasks/tasks-list-view.test.tsx` - rendered sections for multi-membership,
  the unassigned bucket, and the swatch.
- `app/tasks/tasks-page-client.facet-selection.test.tsx` - the facet query round
  trip, the requested value surviving a fallback, and the facet-selection
  transition: after a non-default built-in sort, selecting a facet must not
  locally re-order the loaded rows, so equal-label ties keep the loaded order
  (`AC-PLUGINS-TASKLIST-FACETS-002.6`).
- `components/kanban/mobile-menu-task-list-options.test.tsx` (new file - no
  phone-selector test exists today) - facet options in both phone selectors, the
  updated help copy, a facet label that is also a catalog key rendering verbatim,
  a zero-facet case that asserts only the built-in options appear
  (`AC-PLUGINS-TASKLIST-FACETS-001.3`), and a fail-closed locale guard
  that loads all seven `kanban.json` catalogs and asserts each
  `groupTasksIntoSectionsByState` value carries that locale's plugin-values
  dimension from an explicit expected-substring table. The table has three
  shapes because one of the seven catalogs is English itself: for `en` the entry
  is the plugin-values phrase the change adds, plus an assertion that the value
  is no longer the shipped `"Group tasks into sections by state, workflow,
  repository, or none."`; for the five hand-authored locales it is a
  locale-specific translated phrase that must not appear in the English value, so
  an English token cannot satisfy it; and `pseudo` is derived from the
  generator's transform (`AC-PLUGINS-TASKLIST-FACETS-001.4`): that catalog is
  generated by `pnpm run i18n:pseudo`, so its expectation must be produced by the
  generator's own transform of the current English value, imported from
  `apps/web/scripts/lib/pseudo-locale.mjs`
  (extract `MAP`/`pseudolocalize`/`transform` there from
  `scripts/generate-pseudo-locale.mjs`, which has no exports and writes files at
  import time). Neither a hand-typed accented substring nor a bare "differs from
  English and is non-ASCII" check works: the former breaks on the next
  regeneration, the latter passes on a pseudo value still generated from the old
  English text.
- `app/tasks/tasks-list-controls.test.tsx` (new file) - the desktop listbox
  renders a facet label that is also a catalog key verbatim, and lists only the
  built-in options when no facet is registered.
- Backend `internal/user/models/tasks_list_preferences_test.go` - facet
  acceptance for both enums, unknown-value normalization, built-in behavior.
- Backend `internal/user/service` tests - a facet preference is accepted and
  persisted; an unknown value is rejected, and the rejection error text names
  both accepted forms (a built-in value and `facet:<pluginId>:<facetId>`).
- Backend `internal/task/handlers` - `GET /api/v1/workspaces/:id/tasks` with
  `sort=facet:<pluginId>:<facetId>` (including a whitespace-padded value) reaches
  the service as a built-in sort rather than the facet identifier. This is the
  only place the rerouted gate can be proven, and the package has no `sort`
  assertion there today.
- Backend `internal/backendapp/boot_state_routes_test.go` - a facet query wins
  over the stored default; a stored facet survives boot; and the first-page task
  query receives a built-in sort while the boot payload field keeps the facet.
- Backend `internal/user/models` - `TasksListApiSort` maps a facet and an unknown
  value to the built-in default and passes a built-in value through.
- Backend `internal/task/repository/sqlite/task_order_test.go` - a facet sort
  string yields the default ordering.
- Documentation check for `AC-PLUGINS-TASKLIST-FACETS-001.6`: after task 01,
  neither document denies persistence and both describe the stored selection.
  The assertions in the verification block are deliberately fail-closed: counts
  instead of inverted greps (a grep error cannot read as success), a
  whitespace-normalized stream (the denial is wrapped across two lines), a
  pattern scoped to the denial's object (only the *selection* is denied
  persistence; "a facet identifier is never sent to a task-list query" and "facet
  values themselves are never persisted" are true sentences the documents must
  keep, and `sent` is matched only as `sent to the backend`), and positive
  anchors per document (the encoded `facet:<pluginId>:<facetId>` form, the
  `Unassigned` copy, and the empty-label rule), because a negative check alone is
  satisfied by deleting the sentence. The negative patterns are a bounded net over
  known wordings, not a completeness proof; the positive anchors are what bind the
  criterion.

## E2E tests

- `e2e/tests/plugins/task-list-facet.spec.ts` - group and sort by the fixture
  facet from a workspace user-state document; assert the built-in options
  precede it, the repeated multi-label rows, the trailing unassigned bucket
  carrying the `sidebar:groupUnassigned` copy, the deep-link round trip, and a
  live re-section after the document changes.
- Query-site evidence, split by what the harness can observe. Playwright drives a
  Go-served SPA, so the first page's tasks come from the boot handler and the
  browser issues no task-list request for them: the first-load half of
  `AC-PLUGINS-TASKLIST-FACETS-002.7` is proved by the backend boot test above,
  which records the sort the boot handler passes to the repository: the payload
  itself cannot show it, because the order-by default body matches `updated_desc`
  and an unresolved facet emits the same ORDER BY. The browser-side half is
  proved by forcing a request the spec owns (search or page
  change, since the initial-fetch skip exempts only the untouched first page),
  asserting at least one intercepted task-list request, and rejecting any `sort`
  whose trimmed form starts with `facet:`. The document request itself must keep
  `?sort=facet:...`, per `AC-PLUGINS-TASKLIST-FACETS-003.1`; a spec that
  intercepts the first load looks for a request that does not exist.
- `e2e/tests/plugins/mobile-task-list-facet.spec.ts` - the same facet in the
  Display Options sheet, the facet sections at a phone viewport, and zero
  document horizontal overflow.
- Plugin-state fallback: disable or uninstall the fixture plugin while a facet
  is stored; the list and control show the built-in default, the query keeps the
  facet, the stored setting still reads back as the facet through the
  user-settings API (checked before and after a reload with the plugin absent),
  and re-enabling restores it without any write in between.
- Compatibility with the released Tags plugin: the automated evidence in this
  package is a shape-equivalent registration (same synchronous context-only
  `getValues` and `subscribe` signature) plus a type-only SDK change, because the
  released plugin ships a Go server and the e2e harness stays hermetic. The
  acceptance line names the released plugin itself, so the check that closes it
  is defined here rather than left open: from a build of this branch, install the
  released package (`ghcr`/GitHub release asset, or `make package` in a checkout
  of `kandev-plugin-tags` pinned to a released tag), add it in Settings → Plugins
  or via `POST /api/plugins`, then confirm in `/tasks` that `Tag` appears after
  the built-in Sort and Group options; group by it and check that one section
  renders per tag with a multi-tagged task in each and the unassigned bucket last;
  and select **Sort = Tag** and check the actual task order — a multi-tagged task
  ordered by its alphabetically first tag label, an untagged task last, and ties
  keeping the loaded order — because the acceptance names group *and* sort.
  Owner: whoever verifies the host release; the package's automated suite does not
  cover it, and the acceptance line should be read with that boundary in mind.

## Work orders

- [x] [Task 01: Complete the facet contract and plugin docs](task-01-facet-contract-docs.md)
- [x] [Task 02: Accept facet selections in the stored preference](task-02-preference-shape.md)
- [x] [Task 03: Resolve, persist, and restore the facet selection](task-03-selection-resolution.md)
- [x] [Task 04: Section by value with a trailing unassigned bucket](task-04-grouping-sections.md)
- [x] [Task 05: Prove facets end to end from workspace user state](task-05-fixture-e2e.md)

## Dependency order

Task 01 and task 02 are independent and can run in parallel. Task 03 depends on
both: the encoded form must be accepted by the backend before the round trip is
real rather than client-only. Task 04 depends on nothing structural - it already
receives an effective grouping value and `facetValues` keyed
`<facetKey>:<taskId>`, a key task 03 keeps stable - so it is parallel-safe with
task 03. Task 05 depends on tasks 02, 03, and 04.

## Verification strategy

```bash
cd apps && pnpm --filter @kandev/web exec vitest run lib/plugins/registry.test.ts lib/plugins/registry-task-list-facet-callback.test.ts lib/tasks/tasks-list-options.test.ts lib/tasks/task-list-sections.test.ts hooks/use-task-list-facets.test.tsx hooks/use-task-list-facet-selection.test.tsx app/tasks/tasks-list-view.test.tsx app/tasks/tasks-list-controls.test.tsx app/tasks/tasks-page-client.facet-selection.test.tsx components/kanban/mobile-menu-task-list-options.test.tsx
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm run i18n:check
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
grep -q "2026-09-22-plugin-facet-selection-persistence.md" docs/public/plugins-authoring.md
cd apps/backend && go test ./internal/user/... ./internal/backendapp/... ./internal/task/repository/sqlite/... ./internal/task/handlers/...
make -C apps/backend lint
make build-backend
make -C apps/backend build-e2e-delivery-fixture
cd apps/web && pnpm run build:e2e
make -C apps/backend e2e-plugin-package
cd apps/web && pnpm e2e:run --no-build tests/plugins/task-list-facet.spec.ts
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/plugins/mobile-task-list-facet.spec.ts
test -f docs/plans/plugins/PLUGIN-API.md && test -f docs/public/plugins-authoring.md
grep -q "getTaskListFacets" docs/plans/plugins/PLUGIN-API.md
grep -q "tasks_list_sort" docs/plans/plugins/PLUGIN-API.md
grep -q "tasks_list_group" docs/plans/plugins/PLUGIN-API.md
grep -q "tasks_list_sort" docs/public/plugins-authoring.md
grep -q "tasks_list_group" docs/public/plugins-authoring.md
test "$(grep -c 'facet:<pluginId>:<facetId>' docs/plans/plugins/PLUGIN-API.md)" -ge 1
test "$(grep -c 'facet:<pluginId>:<facetId>' docs/public/plugins-authoring.md)" -ge 1
test "$(grep -Ec '\bUnassigned\b' docs/plans/plugins/PLUGIN-API.md)" -ge 1
test "$(grep -Ec '\bUnassigned\b' docs/public/plugins-authoring.md)" -ge 1
test "$(grep -Eic 'empty label' docs/plans/plugins/PLUGIN-API.md)" -ge 1
test "$(grep -Eic 'empty label' docs/public/plugins-authoring.md)" -ge 1
test "$(grep -c 'page-local Sort and Group choices' docs/public/plugins-authoring.md)" = "0"
test "$(tr -s '[:space:]' ' ' < docs/plans/plugins/PLUGIN-API.md | grep -Eic 'no facet selection[^.]{0,80}(persist|write|reach|store|save|keep|retain|surviv|sent to the backend)')" = "0"
test "$(tr -s '[:space:]' ' ' < docs/plans/plugins/PLUGIN-API.md | grep -Eic 'facet selection[^.]{0,60}(is|are|gets|does|do) (not|never) (persist|write|reach|store|save|keep|retain|surviv|sent to the backend)')" = "0"
test "$(tr -s '[:space:]' ' ' < docs/plans/plugins/PLUGIN-API.md | grep -Eic '(keeps|stores|holds|persists) no facet selection')" = "0"
test "$(tr -s '[:space:]' ' ' < docs/public/plugins-authoring.md | grep -Eic 'no facet selection[^.]{0,80}(persist|write|reach|store|save|keep|retain|surviv|sent to the backend)')" = "0"
test "$(tr -s '[:space:]' ' ' < docs/public/plugins-authoring.md | grep -Eic 'facet selection[^.]{0,60}(is|are|gets|does|do) (not|never) (persist|write|reach|store|save|keep|retain|surviv|sent to the backend)')" = "0"
test "$(tr -s '[:space:]' ' ' < docs/public/plugins-authoring.md | grep -Eic '(keeps|stores|holds|persists) no facet selection')" = "0"
test "$(grep -c 'Ungrouped' docs/plans/plugins/PLUGIN-API.md)" = "0"
test "$(grep -c 'Ungrouped' docs/public/plugins-authoring.md)" = "0"
make typecheck test lint
```

The three documentation gates are explicit because the package's own tooling does
not cover them: `make lint` runs `lint-spec-files.py --all` only, and `make test`
runs the public-doc validator's unit test, never the validator against the real
docs. The public-doc gate matters most for `AC-PLUGINS-TASKLIST-FACETS-001.6`:
task 01 adds a local link from the authoring guide to the ADR, and
`validate-public-docs.mjs` resolves local links with `fs.access`, so a wrong
relative depth passes every other command and fails always-on CI.

The final acceptance sweep for the package runs `make typecheck test lint` at the
repository root — the broader gate the acceptance criteria name, kept as the block's
last line because the targeted commands above are the per-work-order fast
checks — plus the backend test suite, `pnpm run i18n:check`, the three
documentation gates, and the two facet e2e specs against the rebuilt artifacts.
Running only the targeted commands would let a repository-wide regression pass.

## Risks

- **The boot handler is a live query site with two uses in one variable.** `tasksPageBootData` resolves one
  `tasksListSort` and uses it twice: as the first-page task query's sort and as
  the boot payload field the client binds. Only the query half must be resolved
  to a built-in; resolving the field would strip the facet selection on every
  first load, and resolving neither is invisible because the order-by default
  matches the default sort. Both halves need a test.
- **Fallback versus retention.** The fallback must never write the built-in
  default back into the preference or the query. A regression here silently
  destroys the user's choice and only shows up on the next plugin load.
- **Two parsers, one shape.** The Go and TypeScript shape rules are separate
  implementations of one contract; both need a test with the same accepted and
  rejected examples, including a whitespace-padded value and a plugin id with a
  dot or an underscore, or they will drift. A plugin-id rule narrower than the
  manifest's `^[a-z0-9][a-z0-9._-]*$` silently rejects a legally named plugin's
  preference write, which is indistinguishable from the user never choosing it.
- **Client/server trim asymmetry.** `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.3`
  already binds `parseTasksListSort`: the client, not the server, is the side
  that must gain trimming. Widening the parser without trimming ships a facet
  that the boot payload resolves and the client does not.
- **Locale completeness.** Two halves, only one of which `i18n:check` covers.
  Removing `tasks:ungrouped` is caught by key parity, and the guard's
  hand-authored entries cover `en`, `pt-pt`, `zh-cn`, `ja`, and the Traditional
  Chinese pair (from `pnpm run i18n:zh-hant`). The help-copy *value* half is not:
  `i18n:check` gates pseudo on key parity only, so a stale pseudo value passes
  it. That is why `pseudo` comes from `pnpm run i18n:pseudo` and why its guard
  expectation is derived instead of pinned.
- **Fixture provenance.** `apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js`
  is a byte-identical copy of
  `apps/web/e2e/fixtures/plugins/prompt-history-plugin/bundle.js`, staged by
  `make -C apps/backend e2e-plugin-package`, which invokes
  `apps/web/scripts/build-e2e-plugin.mjs` with the required `--outDir`. Run that
  make target; the bare `pnpm run build:e2e-plugin` script has no default output
  directory and fails without one. Edit the source and regenerate; a hand-edited
  copy is overwritten.
- **Unassigned bucket ordering.** Sorting the value buckets and appending the
  unassigned bucket are separate steps; folding the bucket into the same sort
  lets a value label displace it.
- **Task row reuse.** `TasksListView` renders sections for built-in groupings
  too. Extracting the builder must not change the built-in section keys, which
  the existing waiting-for-input and destructive-action tests depend on.

## Implementation results

Ported onto the current `main` base (`02ff057835`, which already carries
`feat(plugins): plugin task list facets for sort and group (#2932)`, `#4108`'s
section extraction, and the `ko` catalog) from the reference implementation on
`feature/group-tasks-by-label-5xz`, then re-verified here. All five work orders
are implemented.

Passing checks, as actually run:

- `pnpm --filter @kandev/web exec vitest run lib/plugins/registry.test.ts
  lib/plugins/registry-task-list-facet-callback.test.ts
  lib/tasks/tasks-list-options.test.ts lib/tasks/task-list-sections.test.ts
  hooks/use-task-list-facets.test.tsx hooks/use-task-list-facet-selection.test.tsx
  app/tasks/tasks-list-view.test.tsx app/tasks/tasks-list-controls.test.tsx
  app/tasks/tasks-page-client.facet-selection.test.tsx
  components/kanban/mobile-menu-task-list-options.test.tsx` (from `apps`):
  10 files, 103 tests passed.
- `pnpm --filter @kandev/web exec vitest run app/tasks lib/tasks lib/plugins
  lib/state/slices/settings components/kanban src/spa-routes` (from `apps`):
  108 files, 859 tests passed, covering every directory this change touches.
- `pnpm run typecheck` (apps/web) and `make typecheck` (root): passed.
- `pnpm --filter @kandev/web lint` (apps) and `make lint` (root: backend 0
  issues, web, 204 harness files, specifications, architecture): passed. Lint
  rejects `--max-warnings 0`, so the extracted `buildFacetSections` helper and
  the split view-test `describe` blocks are part of the port, not cosmetics.
- `pnpm run i18n:check` (apps/web): passed; pseudo in sync, all eight gated
  catalogs complete.
- `go test ./internal/user/... ./internal/task/repository/sqlite/...
  ./internal/task/handlers/...` and `go test ./internal/backendapp/...`
  (apps/backend): passed. `make -C apps/backend lint`: 0 issues.
- Docs: `python3 scripts/list-docs.py validate` (371 decisions, 1521
  specifications), `python3 scripts/lint-spec-files.py --all`, and
  `node scripts/validate-public-docs.mjs` (47 published pages): passed.
- `make -C apps/backend build`, `pnpm --dir apps/web run build:e2e`, and
  `make -C apps/backend e2e-plugin-package`: passed;
  `apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js` is byte-identical
  to the fixture source it is generated from.
- `pnpm e2e:run --no-build tests/plugins/task-list-facet.spec.ts` (apps/web):
  2 passed. `pnpm e2e:run --no-build --project mobile-chrome
  tests/plugins/mobile-task-list-facet.spec.ts`: 1 passed.

Not green, with the evidence that it is unrelated to this change:

- `make test-backend` (`go test -trimpath -tags fts5 ./...`) fails in
  `internal/agent/runtime/lifecycle`, `internal/agentctl/server/api`,
  `internal/agentctl/server/process`, `internal/common/config`,
  `internal/launcher`, and `internal/system/updates`. This change touches no file
  in those packages; the failing cases are HOME/config-discovery, process-capture,
  and install-channel tests that depend on this sandbox (its `HOME` carries a
  `.kandev` config and a launcher supervisor is running). Every package this
  change modifies passes.
- The full web suite (`pnpm --filter @kandev/web test`) reported 97 failing tests
  across 57 files while `make lint` and a Go suite ran concurrently; the failures
  are 5s-timeout cases, for example
  `components/settings/system/storage/storage-policy-card.test.tsx`, which passes
  in isolation (20 tests). The focused and blast-radius runs above cover every
  module this change touches.

Released-package boundary, unchanged from the plan: the released Tags package
still needs the manual install-and-check procedure below. Its shipped bundle
(`kandev-plugin-tags` 0.16.2, `ui/bundle.js`) was inspected for the contract: it
feature-detects `registry.registerTaskListFacet`, registers
`{ id: "tags", label: "Tag", getValues(context) {…}, subscribe(listener) {…} }`,
reads `context.workspaceId` and `context.taskId`, returns
`{ value: tag.id, label: tag.name, color }` per tag, and never touches
persistence. The host contract satisfies that bundle with no plugin-side change;
running the installed package in an isolated instance is not part of the
automated suite.

Rebase addendum (2026-10-11): the change was rebased onto the current main tip
`e8c1761cb5` (13 incoming commits) without conflicts, and re-verified on the
rebased commit: focused facet Vitest 10 files / 103 tests, `pnpm run typecheck`,
`pnpm --filter @kandev/web lint`, `make -C apps/backend build`, and both
Playwright specs (desktop 2 passed, mobile 1 passed) against a freshly rebuilt
backend, web bundle, and fixture package.

Two pre-existing failures at the new base, both independent of this change:
`internal/task/handlers` fails five repository-discovery cases
(`TestHTTPCreateDirectoryCreatesFolder`,
`TestHTTPInitializeLocalRepositoryCreatesRepository`, and siblings) with and
without this commit, confirmed by running them at `e8c1761cb5` in a scratch
worktree; and `internal/task/repository/sqlite` cannot finish because main's
`TestDeliveryRetentionStorage` (`#4407`, `SQLite/1024_bytes`) hangs past a
20-minute timeout. This change touches only `task.go` and `task_order_test.go` in
that package, and its order-by tests pass.

## Port deviations from the design package

- Section building stays in `apps/web/lib/tasks/task-list-sections.ts` (main's
  `#4108` extraction) with its module-scope `t` import; the facet branch became
  the extracted `buildFacetSections` helper, and `flattenTaskTree` stays in
  `tasks-list-view.tsx`. The package's `tasks-list-sections.ts` with an injected
  `translate` was not created: main already has one sections module with the same
  content, three importers, and a test file whose helpers the facet cases reuse.
- Eight shipped catalogs, not seven: main added `ko`. The phone help copy lives at
  `kanban:groupTasksIntoSectionsByWorkflowStep` (main renamed the key from
  `…ByState`), and the locale guard test covers `en`, `pt-pt`, `zh-cn`, `zh-hk`,
  `zh-tw`, `ja`, `ko`, and `pseudo`.
- The built-in group default is `workflow_step`, not `state`, so the fallback
  assertions read `Workflow step`.
- `docs/specs/INDEX.md` carries no catalog row: the page states "Do not add
  catalog rows to this page", and discovery is
  `python3 scripts/list-docs.py specs`.
- The facet service test lives in
  `apps/backend/internal/user/service/service_tasks_list_facets_test.go` rather
  than in `service_test.go`: that file is already 2350 lines, and `make lint`
  rejects any change to it through revive's `file-length-limit` under
  `issues.new: true`.
- Playwright global setup now also requires
  `apps/backend/bin/e2e-delivery-fixture` (main's delivery-retention work), which
  neither `make build-backend` nor the e2e README's prerequisite list builds.
  The verification block above adds
  `make -C apps/backend build-e2e-delivery-fixture`; without it every e2e run in
  this package aborts in global setup before any spec.
