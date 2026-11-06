---
id: "04-grouping-sections"
title: "Section by value with a trailing unassigned bucket"
status: done
wave: 2
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-TASKLIST-FACETS-001
  - REQ-PLUGINS-TASKLIST-FACETS-002
acceptance_criteria:
  - AC-PLUGINS-TASKLIST-FACETS-001.2
  - AC-PLUGINS-TASKLIST-FACETS-001.3
  - AC-PLUGINS-TASKLIST-FACETS-001.4
  - AC-PLUGINS-TASKLIST-FACETS-002.1
  - AC-PLUGINS-TASKLIST-FACETS-002.2
  - AC-PLUGINS-TASKLIST-FACETS-002.3
  - AC-PLUGINS-TASKLIST-FACETS-002.4
  - AC-PLUGINS-TASKLIST-FACETS-002.8
system_design:
  - ../../specs/plugins/system-design/task-list-facets.md
---

# Task 04: Section By Value With A Trailing Unassigned Bucket

## Summary

Extract the task-list section builder into a unit-testable module, place the
unassigned bucket after every value section using the shared unassigned copy,
and update the phone group help copy so plugin-provided values are named as a
grouping dimension in every shipped locale.

## In scope

- Move `TaskTreeNode`, `TaskListSection`, `buildTaskTree`, `buildTaskSections`,
  `groupForTask`, `compareSection`, `flattenTaskTree`, and the section-key
  helpers (`facetValueSectionKey`, `UNGROUPED_FACET_SECTION_KEY`) from
  `apps/web/app/tasks/tasks-list-view.tsx` into
  `apps/web/lib/tasks/tasks-list-sections.ts`, with the host copy passed in as a
  translate function so the module is testable without rendering. `groupForTask`
  is the other consumer of module-scope `t()` (it resolves `tasks:noWorkflow` and
  `tasks:noRepository`), so it takes the injected translate too. Naming the whole
  set matters: `buildTaskSections` calls `groupForTask` and `compareSection`, and
  the section header count comes from `flattenTaskTree`.
- For a facet grouping value: one bucket per value returned by at least one
  loaded task, keyed `facet:value:<value>`; sort the value buckets by label with
  a case-insensitive comparison; append the tasks that returned no value as one
  final bucket keyed `facet:host:ungrouped`, titled with
  `sidebar:groupUnassigned`; rebuild the tree inside each bucket from that
  bucket's tasks.
- Do not add a dedupe layer: a repeated value for one task already yields one
  row and a count of one, because the per-bucket tree rebuild drops an
  already-visited task id. Keep the duplicated-value case in the sections test as
  the regression guard for that guard.
- Apply the empty-label rule in the builder as well: ignore a values-map entry
  whose label is empty, so the view cannot render a section titled with the empty
  string when it is handed raw values (the view takes `facetValues` as a prop,
  and its tests pass raw maps). Per entry, so a non-empty sibling value is
  unaffected (`AC-PLUGINS-TASKLIST-FACETS-002.8`); a task with only empty-label
  entries lands in the unassigned bucket.
- Keep the color swatch, and keep the keys that let a plugin value named
  `untagged` or `__host_ungrouped__` stay distinct from the host bucket.
- Leave the built-in `state`, `workflow`, `repository`, and `none` branches with
  their current keys and ordering.
- Remove `tasks:ungrouped` from every catalog now that it has no caller, and
  change `kanban:groupTasksIntoSectionsByState` in `en`, `pt-pt`, `zh-cn`, and
  `ja` to name plugin-provided values; regenerate `zh-hk` and `zh-tw` with
  `pnpm run i18n:zh-hant` and regenerate `pseudo` with `pnpm run i18n:pseudo`
  after the `en` edit. `pseudo` is generated from `en` by
  `scripts/generate-pseudo-locale.mjs`, so never hand-edit it: a hand-typed
  accented string passes `i18n:check` (which gates pseudo on key parity only) and
  the next legitimate regeneration silently changes it.
- Add the label-verbatim assertion for `AC-PLUGINS-TASKLIST-FACETS-001.2` in both
  surfaces, using a facet label that is also a real catalog key: register the
  facet with `label: "tasks:groupByState"` (and a value label of the same shape)
  and assert both the desktop Sort/Group listbox and the phone option row render
  that string verbatim. A translation-stable label such as "Tag" cannot catch a
  regression that routes the label through `t()`: the catalog has no such key,
  `returnNull: false` renders a missing key as the key itself, and the rendered
  string is identical either way. Create
  `app/tasks/tasks-list-controls.test.tsx` for the desktop half; create
  `components/kanban/mobile-menu-task-list-options.test.tsx` for the phone half
  (neither file exists today).
- Tests: a new `lib/tasks/tasks-list-sections.test.ts` for multi-membership,
  values no task carries, the trailing unassigned bucket, per-section trees,
  label ordering pinned with a same-base pair inserted in the discriminating
  order (`Beta` before `beta`: `sensitivity: "base"` compares them equal so the
  incoming order must survive, while the default comparison would reverse them
  and fail the case; `beta` before `Beta` is vacuous because both comparisons
  agree), swatch color, a duplicated value for
  one task (one row, not two), and an empty-label entry that must not produce a
  section (`AC-PLUGINS-TASKLIST-FACETS-002.8`); update
  `app/tasks/tasks-list-view.test.tsx` so the rendered section assertions keep
  covering the same outcomes; create
  `app/tasks/tasks-list-controls.test.tsx` for the desktop listbox, and create
  `components/kanban/mobile-menu-task-list-options.test.tsx` (neither exists
  today, so phone-selector unit coverage and the group help copy currently have
  no test home) for the phone selector, the updated help copy, the key-shaped
  label assertion, the locale guard below, and a zero-facet case asserting both
  option lists contain only the built-in values (`AC-PLUGINS-TASKLIST-FACETS-001.3`
  at the rendering boundary). The mobile options test also carries the
  fail-closed locale guard for `AC-PLUGINS-TASKLIST-FACETS-001.4`: it loads all
  seven `kanban.json` catalogs and asserts each
  `groupTasksIntoSectionsByState` value contains that locale's plugin-values
  dimension, from an explicit per-locale expected-substring table kept in the
  test. Key presence alone cannot catch a catalog left at the old
  state/workflow/repository/none wording, so a partial edit must fail the test.
  The table has three shapes, because one of the seven catalogs is English
  itself and the no-overlap discriminant cannot apply to it:
  - `en`: the expected phrase is the plugin-values dimension the change adds, and
    the guard additionally asserts the value is no longer the shipped pre-change
    sentence `"Group tasks into sections by state, workflow, repository, or
    none."`, so a stale English catalog fails.
  - `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`, `ja`: a locale-specific translated phrase
    (`valores de plugin`, `プラグイン`, `插件`, `外掛`) that does not appear in the
    English value, so a token of the English sentence cannot satisfy it.
  - `pseudo`: generated, so no hand-pinned substring (below).
  For `pseudo`, whose catalog is generated, do not pin a hand-typed
  accented substring and do not settle for "differs from English and is
  non-ASCII": a value generated from the *old* English text still differs from
  the new English text and is still non-ASCII, so that check passes on stale
  copy, which is exactly what this guard exists to catch. Assert instead that the
  pseudo value equals the generator's transform of the current English value,
  importing the transform from `apps/web/scripts/lib/pseudo-locale.mjs`. That
  module does not exist yet: extract `MAP`, `pseudolocalize`, and `transform`
  from `scripts/generate-pseudo-locale.mjs` into it and have both the CLI
  generator and this test import it (the repo already pairs `scripts/lib/*.mjs`
  with a co-located `.test.ts`). Never re-implement the mapping in the test, and
  never import the CLI itself: it has no exports and writes catalog files at
  import time.

## Out of scope

- Selection resolution and persistence (task 03).
- Copy for languages that are not gated catalogs.
- The E2E specs (task 05).

## Acceptance

- One section appears per value carried by the loaded tasks, ordered by label
  case-insensitively, with one row per carrying task and no section for a value
  no task carries; a two-value task appears once in each of its sections, a
  parent/child pair renders indented inside its section, and a value's color
  renders as the section swatch.
- Tasks with no value appear together in the last section, after every value
  section, labelled with `sidebar:groupUnassigned`, even when a plugin value's
  label sorts after that copy.
- `pnpm run i18n:check` passes with `kanban:groupTasksIntoSectionsByState`
  naming plugin-provided values in every catalog, `tasks:ungrouped` absent from
  all seven catalogs, and the seven-catalog locale guard test passing. A facet
  label that is also a catalog key renders verbatim in the desktop listbox and
  in the phone option row. The
  removal check counts how many catalogs report zero matches and requires seven,
  so a missing or renamed catalog fails it instead of reading as success; a bare
  `! grep` cannot make that distinction. Do not run
  `check-i18n-keys.mjs --strict-orphans`: the tree already carries 308
  unreferenced entries, so an absolute-zero orphan assertion cannot pass and
  would report a pre-existing backlog this work order neither caused nor owns.

## Verification

```bash
cd apps && pnpm --filter @kandev/web exec vitest run lib/tasks/tasks-list-sections.test.ts app/tasks/tasks-list-view.test.tsx app/tasks/tasks-list-controls.test.tsx components/kanban/mobile-menu-task-list-options.test.tsx
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps/web && node scripts/generate-pseudo-locale.mjs
cd apps/web && pnpm run i18n:check
cd apps/web && for l in en pt-pt zh-cn zh-hk zh-tw ja pseudo; do test -f "src/locales/$l/tasks.json" || exit 1; done; test "$(grep -c '"ungrouped"' src/locales/en/tasks.json src/locales/pt-pt/tasks.json src/locales/zh-cn/tasks.json src/locales/zh-hk/tasks.json src/locales/zh-tw/tasks.json src/locales/ja/tasks.json src/locales/pseudo/tasks.json | grep -c ':0$')" = "7"
cd apps/web && node scripts/check-i18n-keys.mjs
```

## Files likely touched

- `apps/web/lib/tasks/tasks-list-sections.ts`
- `apps/web/lib/tasks/tasks-list-sections.test.ts`
- `apps/web/app/tasks/tasks-list-view.tsx`
- `apps/web/app/tasks/tasks-list-view.test.tsx`
- `apps/web/app/tasks/tasks-list-controls.test.tsx`
- `apps/web/components/kanban/mobile-menu-task-list-options.tsx`
- `apps/web/components/kanban/mobile-menu-task-list-options.test.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,ja}/kanban.json`
- `apps/web/src/locales/{en,pt-pt,zh-cn,ja}/tasks.json`
- `apps/web/scripts/lib/pseudo-locale.mjs` (extracted transform, shared by the
  CLI generator and the locale guard test)
- `apps/web/src/locales/pseudo/*.json` (regenerated by `pnpm run i18n:pseudo`)
- `apps/web/src/locales/{zh-hk,zh-tw}/kanban.json`
- `apps/web/src/locales/{zh-hk,zh-tw}/tasks.json`

## UI reference

[UI-01 and UI-02](plan.md#ui-design-contract) in the plan own the section
structure and the option order. Reuse the shipped section header and swatch; do
not introduce a new header component.

### UI-01: `/tasks` grouped by an active facet (desktop, excerpt)

```text
│  ● ALPHA                                                            2 tasks  │  value section:
│    ▸ Multi value task                                                        │  swatch = value color
│      ▸ Child of multi value task                                             │  tree rebuilt inside
│  ● BETA                                                             1 task   │  the section
│    ▸ Multi value task                                                        │
│  ● UNASSIGNED                                                       1 task   │  host bucket, always
│    ▸ Unlabelled task                                                         │  last, sidebar copy
```

Structural: value sections ordered by label, one row per task per matching
value, unassigned last, child indented under its own parent. Illustrative:
spacing, swatch size, counts — the scenario is one multi-value task, its child,
and one unlabelled task, so the sections read 2/1/1 and the footer counts three
tasks while four rows render.

## Dependencies

None. The consumed interface is already in place: the view receives an effective
grouping value and `facetValues` keyed `<facetKey>:<taskId>`. Task 03 keeps that
key stable, so do not rename it.

## Risks

- Folding the unassigned bucket into the label sort lets a value label displace
  it. Sort the value buckets, then append the bucket.
- The built-in section keys are asserted by the existing waiting-for-input and
  destructive-action tests. Extraction must not change them.
- `useMemo` dependencies in the view currently include the language; keep the
  section rebuild reactive to `facetValues`, the grouping value, and the
  language after the extraction.
- Removing `tasks:ungrouped` from one catalog without the others fails the key
  parity check. Remove it everywhere in one change.

## Parallelism

`parallel-safe` with task 03.

## Inputs

- `REQ-PLUGINS-TASKLIST-FACETS-001`, `REQ-PLUGINS-TASKLIST-FACETS-002`
- `docs/specs/plugins/system-design/task-list-facets.md`
- `docs/specs/guide/plans-and-work-orders.md` ASCII preview contract

## Results

Ported onto the current `main` base (`02ff057835`) and re-verified here. The
reference branch recorded the RED observations (an empty-label task sorted before
Zulu and displaced the trailing bucket; the phone guard failed on the old help
text); this port carries the same assertions.

- `pnpm --filter @kandev/web exec vitest run lib/tasks/task-list-sections.test.ts
  app/tasks/tasks-list-view.test.tsx app/tasks/tasks-list-controls.test.tsx
  components/kanban/mobile-menu-task-list-options.test.tsx` (apps): passed; the
  whole focused run covers 10 files and 103 tests.
- `pnpm run typecheck` (apps/web) and `pnpm --filter @kandev/web lint` (apps):
  passed. Lint flagged the section builder (cyclomatic complexity) and the grown
  test `describe`; the facet branch is now its own `buildFacetSections` helper and
  the view test splits into grouping and rendering blocks.
- `pnpm run i18n:check` (apps/web): passed (9531 referenced keys, pseudo in sync,
  all eight gated catalogs complete). `tasks:ungrouped` is absent from all eight
  `tasks.json` catalogs, and `kanban:groupTasksIntoSectionsByWorkflowStep` names
  plugin-provided values in every one, `pseudo` regenerated by
  `node scripts/generate-pseudo-locale.mjs`.

Main-base adaptations (both recorded as deviations in the plan):

- Section building already lives in `apps/web/lib/tasks/task-list-sections.ts`
  (main's `#4108`) with the module-scope `t` import, so the port adds the facet
  branch and the extracted `buildFacetSections` helper there instead of creating
  `tasks-list-sections.ts` with an injected translate. `flattenTaskTree` still
  lives in `tasks-list-view.tsx`, so the sections test carries a local flatten
  helper.
- Main added the `ko` catalog and renamed the group help key from
  `groupTasksIntoSectionsByState` to `groupTasksIntoSectionsByWorkflowStep`, so
  eight catalogs (not seven) carry the new copy and the locale guard table.
