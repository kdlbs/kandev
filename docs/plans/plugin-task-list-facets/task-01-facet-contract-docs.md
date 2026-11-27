---
id: "01-facet-contract-docs"
title: "Complete the facet contract and plugin docs"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-TASKLIST-FACETS-001
acceptance_criteria:
  - AC-PLUGINS-TASKLIST-FACETS-001.1
  - AC-PLUGINS-TASKLIST-FACETS-001.2
  - AC-PLUGINS-TASKLIST-FACETS-001.3
  - AC-PLUGINS-TASKLIST-FACETS-001.5
  - AC-PLUGINS-TASKLIST-FACETS-001.6
  - AC-PLUGINS-TASKLIST-FACETS-001.7
system_design:
  - ../../specs/plugins/system-design/task-list-facets.md
---

# Task 01: Complete the Facet Contract and Plugin Docs

## Summary

Name the facet callback context in the public SDK, and make the plugin
documentation describe the contract the host will ship: multi-value membership,
the trailing unassigned bucket, page-local ordering, and a selection that
persists in the host sort and group preference.

## In scope

- Add `TaskListFacetContext` to `apps/packages/plugin-sdk/src/index.ts` and use
  it as the `getValues` parameter type. The change is structural:
  `getValues({ taskId, workspaceId })` keeps working for an already released
  bundle.
- Keep the registry behavior that already satisfies the criteria, and close the
  two remaining test gaps: a registration shaped exactly like the released tags
  plugin's
  (`{ id: "tags", label: "Tag", getValues: ({ taskId, workspaceId }) => [...] }`
  with a `subscribe` that returns an unsubscribe), and the registration-lifecycle
  notification. Do not assert the private `totalCount()`: assert the observable
  bump instead — with facets registered, a registry subscriber fires on
  `unregisterPlugin`, because the registry notifies only when its registration
  count changes.
- Update `docs/plans/plugins/PLUGIN-API.md` and
  `docs/public/plugins-authoring.md` for the persistence, multi-membership, and
  unassigned-bucket outcomes: state the stored selection with both preference
  names and the encoded form, replace the denial in `PLUGIN-API.md`, add the
  facet selector to its registry selector list, rename the section for tasks
  without a value in both (both currently promise an `Ungrouped` section while
  task 04 renders `sidebar:groupUnassigned`, "Unassigned"), and state the
  empty-label rule (an entry whose label is empty means the task carries no value
  for that entry). Also narrow the authoring guide's "page-local Sort and Group
  choices" claim to the values, since the choice's selection is now stored. One
  editing pass must satisfy every assertion in the verification block below, per
  document.
- Link the ADR from the authoring guide's facet section.

## Out of scope

- Any change to the store, the API, or the list rendering.
- New contribution points or a facet filter.
- The fixture bundle (task 05).

## Acceptance

- A registered facet with a URL-safe slug id and a label becomes a Sort and a
  Group choice positioned after the built-in options, a non-slug id is rejected
  at registration, a registration shaped like the released plugin's is accepted,
  and with no facet registered both option lists contain only built-in values.
- The choice label is the plugin's label rendered without a host translation,
  and `unregisterPlugin` removes the facet from `getTaskListFacets()`, from the
  option lists, and observably from the registry: a registry subscriber fires on
  the removal.
- Both plugin documents state the persisted-selection contract instead of
  denying it, and state it with the encoded `facet:<pluginId>:<facetId>` form and
  the two preference names; `PLUGIN-API.md` lists the facet selector alongside
  the other registry selectors; and both use the `Unassigned` copy rather than
  `Ungrouped`.

## Verification

```bash
cd apps && pnpm --filter @kandev/web exec vitest run lib/plugins/registry.test.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
grep -q "2026-09-22-plugin-facet-selection-persistence.md" docs/public/plugins-authoring.md
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
```

The negative assertions have four properties, and each one is a trap this package
has already fallen into once:

- **Count, never `!`.** `test "$(grep -c …)" = "0"` cannot be satisfied by a grep
  error; a missing path yields an empty count and fails.
- **Normalized stream.** The denial in `PLUGIN-API.md` is wrapped across two
  lines, so a line-anchored pattern misses it.
- **Scoped to the denial's object, not to the word `facet`.** A `facet`-scoped
  pattern matches the two sentences the documents must keep: "A facet identifier
  is never sent to a task-list query" and "Facet values themselves are never
  persisted by the host" (the selection is persisted; values are not). Only the
  *selection* is the object of the denial, so the patterns are `no facet
  selection …`, `facet selection … (not|never) …`, and `(keeps|stores|holds|
  persists) no facet selection …`.
- **Cover the phrasings, not one wording.** The alternations carry the verb
  stems (`persist`, `write`, `reach`, `store`, `save`, `keep`, `retain`,
  `survive`) and `sent` only as `sent to the backend`, so "A facet selection is
  never sent to a task-list query" — a true sentence this same change must keep —
  no longer matches. Treat the negative patterns as a bounded net over the
  wordings we know, not as a completeness proof: a denial phrased as "the plugin
  cannot persist a facet selection" still escapes them, which is why the positive
  anchors, not the negatives, are what bind the criterion.
- **Paired with a positive.** A negative check alone is satisfied by deleting the
  sentence, so each document must also state the encoded form
  (`facet:<pluginId>:<facetId>`), the two preference names, the `Unassigned`
  copy (asserted with word boundaries, because the identifier
  `sidebar:groupUnassigned` contains the copy as a substring), and the empty-label
  rule. The positives are what bind the authoring guide: its only pre-existing
  page-locality sentences describe *values* ("apply only to the currently loaded
  page"), which stay true, so a page-local negative pattern would forbid a
  correct sentence. The one page-locality claim that does become false — the
  guide's "page-local Sort and Group choices" — is fixed here and asserted
  absent.

## Files likely touched

- `apps/packages/plugin-sdk/src/index.ts`
- `apps/web/lib/plugins/types.ts`
- `apps/web/lib/plugins/registry.ts`
- `apps/web/lib/plugins/registry.test.ts`
- `apps/web/lib/plugins/registry-registration-types.ts`
- `docs/plans/plugins/PLUGIN-API.md`
- `docs/public/plugins-authoring.md`

## Dependencies

None.

## Risks

- The SDK type is loaded by already released third-party bundles. Keep the
  change structurally compatible: no new required field, no renamed member.
- The docs statement being replaced is load-bearing for other plugins that
  assumed a page-local selection. State the fallback behavior explicitly so a
  plugin author understands that an unavailable plugin is not an error.

## Parallelism

`parallel-safe` with task 02.

## Inputs

- `REQ-PLUGINS-TASKLIST-FACETS-001`
- `docs/specs/plugins/system-design/task-list-facets.md`
- `docs/decisions/2026-09-22-plugin-facet-selection-persistence.md`
- Existing `registerTaskFilter` / `registerTaskListFacet` registry patterns.

## Results

Ported onto the current `main` base (`02ff057835`) from the reference
implementation on `feature/group-tasks-by-label-5xz`, then re-verified here:

- `TaskListFacetContext` added to `apps/packages/plugin-sdk/src/index.ts` and
  re-exported from `apps/web/lib/plugins/types.ts`; the released-plugin-shaped
  callback/lifecycle test moved into its own
  `lib/plugins/registry-task-list-facet-callback.test.ts` (isolating it from the
  slug/duplicate/lifecycle block in `registry.test.ts`).
- `docs/plans/plugins/PLUGIN-API.md` and `docs/public/plugins-authoring.md`
  describe the stored `facet:<pluginId>:<facetId>` selection, the trailing
  `Unassigned` section, the empty-label rule, and the page-local ordering rule.

Verification (apps = `apps`, apps/web as noted):

- `pnpm --filter @kandev/web exec vitest run lib/plugins/registry.test.ts
  lib/plugins/registry-task-list-facet-callback.test.ts`: passed (part of the
  10-file, 103-test focused run below).
- `pnpm run typecheck` (apps/web): passed. `pnpm --filter @kandev/web lint`
  (apps): passed with zero warnings.
- `python3 scripts/list-docs.py validate`: 371 decisions and 1521 specifications
  validated. `python3 scripts/lint-spec-files.py --all`: passed.
  `node scripts/validate-public-docs.mjs`: 47 published pages validated, which
  resolves the authoring guide's local ADR link.
- Documentation assertions: both documents carry `facet:<pluginId>:<facetId>`,
  `Unassigned`, and an empty-label rule; the whitespace-normalized persistence
  denials match zero times, `Ungrouped` appears zero times, and
  `page-local Sort and Group choices` is gone.
