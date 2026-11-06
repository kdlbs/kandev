---
id: "03-selection-resolution"
title: "Resolve, persist, and restore the facet selection"
status: done
wave: 2
depends_on:
  - "01-facet-contract-docs"
  - "02-preference-shape"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-TASKLIST-FACETS-002
  - REQ-PLUGINS-TASKLIST-FACETS-003
acceptance_criteria:
  - AC-PLUGINS-TASKLIST-FACETS-002.5
  - AC-PLUGINS-TASKLIST-FACETS-002.6
  - AC-PLUGINS-TASKLIST-FACETS-002.7
  - AC-PLUGINS-TASKLIST-FACETS-003.1
  - AC-PLUGINS-TASKLIST-FACETS-003.2
  - AC-PLUGINS-TASKLIST-FACETS-003.3
  - AC-PLUGINS-TASKLIST-FACETS-003.4
  - AC-PLUGINS-TASKLIST-FACETS-002.8
system_design:
  - ../../specs/plugins/system-design/task-list-facets.md
---

# Task 03: Resolve, Persist, And Restore The Facet Selection

## Summary

Let a facet selection live in the same place as a built-in selection: encoded in
the `tasks_list_sort` / `tasks_list_group` preference and the `/tasks` query,
restored on load, and replaced by the built-in default in the rendered list and
controls while the owning plugin is not active, without overwriting the stored
value.

## In scope

- Add facet option encoding and decoding to
  `apps/web/lib/tasks/tasks-list-options.ts`:
  `encodeTaskListFacetOption({ pluginId, id })`, `parseTaskListFacetOption`, and
  the shape rule the backend mirrors. Widen `parseTasksListSort` and
  `parseTasksListGroup` to return a built-in value or a facet option, and add
  `resolveTasksListApiSort` (a facet resolves to `TasksListSortDefault`).
- Trim surrounding whitespace before matching in both widened parsers. The
  server already trims, and `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.3` names
  `parseTasksListSort` as the side that must gain trimming; leaving the
  asymmetry in place fails an in-force criterion, and a padded facet value would
  resolve to the facet on the boot payload and to the built-in default in the
  client.
- Widen `UserSettingsState.tasksListSort` / `tasksListGroup` in
  `apps/web/lib/state/slices/settings/types.ts`, and the state and props in
  `apps/web/app/tasks/tasks-page-client.tsx` and `apps/web/app/tasks/page.tsx`,
  to the widened preference type. Built-in reads keep working unchanged.
- Introduce the requested-versus-effective split in
  `apps/web/hooks/use-task-list-facet-selection.ts`: the requested value comes
  from the query or preference; a facet is effective only while its key is in
  the resolved facet list, otherwise the effective value is
  `DEFAULT_TASKS_LIST_SORT` / `DEFAULT_TASKS_LIST_GROUP`. Change handlers write
  the requested value, and the hook exposes the resolved facet key used for
  sectioning and ordering.
- Resolve the query sort at the client query site: `useTaskOperations` requests
  `resolveTasksListApiSort(requestedSort)`, so a facet-sorted list never sends a
  facet identifier to the list API. The other two live sites — the Go boot
  handler that serves the first page and the task-list HTTP endpoint's `sort`
  gate — are task 02's.
- Widen `apps/web/app/tasks/page.tsx` only for type compatibility. It is a
  non-executing Next-era twin: nothing imports it, the package has no `next`
  dependency, and the live `/tasks` route is `apps/web/src/spa-routes.tsx`
  rendering `TasksPageClient` from the boot payload. Do not add behavior tests
  for it, and do not treat it as a query site.
- Trim before matching in the client predicate and in
  `resolveTasksListApiSort`, mirroring the two parsers and the Go predicate: a
  bare `isTaskListFacetOption` is `value.startsWith("facet:")`, so
  `resolveTasksListApiSort(raw)` written as
  `isTaskListFacetOption(raw) ? default : parse(raw)` returns the padded facet
  for `" facet:plugin:tags "`. That would both send a facet identifier to the
  list API (`AC-PLUGINS-TASKLIST-FACETS-002.7`) and resolve a padded value
  differently from the server (`AC-PLUGINS-TASKLIST-FACETS-003.4`, with
  `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.3` binding the client side). The e2e
  guard rejects any value whose *trimmed* form starts with `facet:`, so a missing
  trim is browser-visible too, not only unit-visible.
- Return the trimmed token, not the raw input, from both widened parsers and
  from `resolveTasksListApiSort`. The Go side already returns the trimmed value
  (`NormalizeTasksListSort`), while the shipped parsers compare exactly, so a
  padded built-in token fails the match, returns the built-in default, and the
  query-param sync then persists that default over the stored value:
  `?sort=%20title_asc%20` renders as the default on the client while the boot
  payload resolves `title_asc`, and the user's choice is overwritten. Trimming
  for the match alone is not enough; the returned token must be trimmed. Add the
  padded built-in case next to the padded facet to the API-sort-resolution test.
- Add the tie case for `sortTasksByFacet` to `lib/tasks/tasks-list-options.test.ts`
  with the pinned same-base pair (`Beta` before `beta`), asserting the incoming
  order survives. Describe it as pinning `AC-PLUGINS-TASKLIST-FACETS-002.6`'s
  case-insensitive comparison: it fails if the comparator degrades to the default
  sensitivity, which would reverse the pair. Do not claim it guards the
  `|| a.index - b.index` tie-break; `Array.prototype.sort` is stable, so that
  tie-break is a restatement of stability and no input can distinguish it. The
  shipped "keeps ties stable" case uses three distinct labels and asserts no tie
  at all, which is why the pair is needed for the comparison itself.
- Make a facet selection write the requested value through the same path a
  built-in selection uses (so `AC-PLUGINS-TASKLIST-FACETS-003.1` needs no second
  persistence trigger) while **skipping that path's local re-sort**:
  `handleSortChange` runs `setTasks((prev) => sortTasksForList(prev, value))`,
  and with a facet value that call re-orders the loaded rows to `updated_desc`
  before `sortTasksByFacet` seeds from them, which changes the order that
  `AC-PLUGINS-TASKLIST-FACETS-002.6` says equal labels must keep. Guard the
  re-sort on the value being a built-in option. Cover the transition in
  `app/tasks/tasks-page-client.facet-selection.test.tsx`: with a non-default
  built-in sort loaded (`title_asc`) and two rows whose labels compare equal,
  selecting the facet sort must keep the loaded order for those rows while the
  query and the stored preference receive the facet.
- Make `firstFacetLabel` skip empty labels so `sortTasksByFacet` satisfies
  `AC-PLUGINS-TASKLIST-FACETS-002.8` on its own input: today it sorts the labels
  and takes the first, so `["", "Alpha"]` returns the empty string and the task
  sorts last even though it carries a usable value. Assert it in
  `lib/tasks/tasks-list-options.test.ts` with an empty-label sibling and the
  non-empty sibling's label winning.
- Leave the built-in sorter's facet behavior as documented, do not test it:
  `sortTasksForList` / `compareTasksForList` fall to their shared
  `updated_desc` default body for a facet value, which is the order
  `resolveTasksListApiSort` asks the API for, so a facet value cannot disturb the
  incoming order that `sortTasksByFacet` depends on for tie stability. An
  assertion here is true by construction; `resolveTasksListApiSort` covers the
  behavior that matters.
- Keep the facet-notification path: a `subscribe` notification re-resolves
  values and re-sections the list without a reload.
- Discard a value whose label is empty inside `resolveTaskFacetValues`, per
  entry, next to the existing malformed-entry filter. The facet sorter tests the
  label for falsiness, so an empty-label entry would already order as "no value"
  while the section builder titled a section with the empty string. The rule is
  per entry, so a task returning a non-empty value alongside an empty-label one
  keeps the non-empty value; only a task whose entries are all empty-label counts
  as returning no value (`AC-PLUGINS-TASKLIST-FACETS-002.8`). Cover it in
  `hooks/use-task-list-facets.test.tsx`; task 04 covers the same rule at the
  section builder, which receives raw values through the view's props.
- Tests: `lib/tasks/tasks-list-options.test.ts` for encoding, decoding, widened
  parsing including a whitespace-padded value, API sort resolution, and the
  shared accepted/rejected shape examples task 02 also uses;
  `hooks/use-task-list-facet-selection.test.tsx` for the requested
  versus effective split, fallback while the facet is absent, restoration when
  the facet appears, and that the fallback path never invokes the preference
  writer, so the requested facet stays the value that would be persisted (assert
  the writer was not called with the built-in default, not only that the
  effective value changed);
  the facet appears, built-in change behavior, and the requested value surviving
  a change to a different built-in option; `hooks/use-task-list-facets.test.tsx`
  for the revision bump; a new
  `app/tasks/tasks-page-client.facet-selection.test.tsx` for the query round
  trip.

## Out of scope

- Section rendering and host copy (task 04).
- The backend shape rule (task 02).
- Playwright coverage (task 05).

## Acceptance

- Selecting a facet writes `facet:<pluginId>:<facetId>` to the stored preference
  and the query, and loading `/tasks` with that query binds the facet in both
  controls and applies its grouping or ordering, on the boot-served first load
  and after hydration.
- With the facet absent from the registry, the controls and the list show the
  built-in default while the stored value and the query keep the facet, and the
  selection reappears when the facet registers without a reload; a built-in
  selection and an unknown stored value behave exactly as before.
- A facet sort reorders the loaded rows by first value label with unassigned
  last and stable ties, a whitespace-padded stored or query value resolves
  identically on the server and the client, and no request site carries a facet
  identifier.

## UI reference

[UI-03](plan.md#ui-03-an-unavailable-facet-falls-back-without-losing-the-selection)
in the plan owns the fallback state this work order produces.

### UI-03: An unavailable facet falls back without losing the selection

```text
  URL:      /tasks?group=facet:my-plugin:tags          ← unchanged, still holds the facet

  before (plugin active)      Group [ Fixture tag ▾ ]   sections: ALPHA | BETA | UNASSIGNED
  after  (plugin unavailable) Group [ State       ▾ ]   sections: state groups
                              ▲ built-in default, no write-back to the URL or the stored preference

  plugin becomes active again → Group [ Fixture tag ▾ ] without user action, no reload
```

Structural: the control binds the effective value while the query and the stored
preference keep the requested value, and the return needs no user action.
Illustrative: the specific group names. The control is in the fixed toolbar; the
section list is the single page scroll. On phones it is the Group selector in
the Display Options sheet, which scrolls internally; the fallback and the return
behave identically on both viewports.

## Verification

```bash
cd apps && pnpm --filter @kandev/web exec vitest run lib/tasks/tasks-list-options.test.ts hooks/use-task-list-facet-selection.test.tsx hooks/use-task-list-facets.test.tsx app/tasks/tasks-page-client.facet-selection.test.tsx
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
```

## Files likely touched

- `apps/web/lib/tasks/tasks-list-options.ts`
- `apps/web/lib/tasks/tasks-list-options.test.ts`
- `apps/web/lib/state/slices/settings/types.ts`
- `apps/web/app/tasks/tasks-page-client.tsx`
- `apps/web/app/tasks/tasks-page-client.facet-selection.test.tsx`
- `apps/web/app/tasks/page.tsx`
- `apps/web/hooks/use-task-list-facet-selection.ts`
- `apps/web/hooks/use-task-list-facet-selection.test.tsx`
- `apps/web/hooks/use-task-list-facets.ts`
- `apps/web/hooks/use-task-list-facets.test.tsx`

## Dependencies

Task 01 (option key helpers and the named facet context) and task 02 (the
backend accepts the encoded form, so the round trip is real rather than
client-only).

The interface task 04 consumes is fixed here: the list view receives an
effective `tasksListGroup` value and `facetValues` keyed `<facetKey>:<taskId>`,
where `<facetKey>` is `facet:<pluginId>:<facetId>`. Do not rename that key.

## Risks

- The fallback must not write the built-in default back into the preference or
  rewrite the query. Persisting the fallback destroys the user's choice, and a
  test that asserts only the rendered/effective value cannot see the difference:
  a regression that persists the fallback while leaving the query intact still
  renders the built-in default. Assert the writer call, and in task 05 assert the
  stored setting itself.
- `useTasksListPreferenceSync` currently persists whatever the query holds; it
  must persist the requested value, not the effective one.
- The widened preference type reaches the store slice and the boot payload.
  Keep the built-in branch typed so existing consumers do not need a cast.
- The registry version changes during plugin load. The hook must re-resolve on
  version changes rather than caching the first resolution.
- `apps/web/app/tasks/page.tsx` is a non-executing Next-era twin: nothing
  imports it, there is no `next` dependency, and the live `/tasks` route is
  `apps/web/src/spa-routes.tsx`. Keep it compiling with the widened types, add no
  behavior test for it, and do not count it as a query site.
- Trimming must land in both widened parsers, not only the group one.
  `parseTasksListSort` is named by an in-force criterion, and a padded value
  otherwise resolves two different ways in one page load.

## Parallelism

`parallel-safe` with task 04; the shared frontend contract is stated above.

## Inputs

- `REQ-PLUGINS-TASKLIST-FACETS-002`, `REQ-PLUGINS-TASKLIST-FACETS-003`
- `docs/specs/plugins/system-design/task-list-facets.md`
- `docs/decisions/2026-09-22-plugin-facet-selection-persistence.md`

## Results

Ported onto the current `main` base (`02ff057835`) and re-verified here. The
reference branch recorded the RED observations (facet parsed to the built-in
default, empty labels sorted first, unavailable facets cleared the request, no
requested/effective wiring); this port carries the same tests.

- `pnpm --filter @kandev/web exec vitest run lib/tasks/tasks-list-options.test.ts
  hooks/use-task-list-facet-selection.test.tsx hooks/use-task-list-facets.test.tsx
  app/tasks/tasks-page-client.facet-selection.test.tsx` (apps): passed (part of
  the focused 10-file, 103-test run).
- `pnpm run typecheck` (apps/web): passed, including the non-executing Next-era
  `app/tasks/page.tsx` twin, the SPA boot projection in `src/spa-routes.tsx`, and
  the widened `UserSettingsState` preference fields.
- `pnpm --filter @kandev/web lint` (apps): passed with zero warnings.

Main-base adaptation: the built-in group default here is `workflow_step` (not
`state`), so the fallback assertions in the hook, page-client, and Playwright
specs use `Workflow step`; `resolveTasksListApiSort` resolves a facet to
`updated_desc` on both client query sites and the boot handler.
