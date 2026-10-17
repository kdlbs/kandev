---
id: "05-fixture-e2e"
title: "Prove facets end to end from workspace user state"
status: done
wave: 3
depends_on:
  - "02-preference-shape"
  - "03-selection-resolution"
  - "04-grouping-sections"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-TASKLIST-FACETS-001
  - REQ-PLUGINS-TASKLIST-FACETS-002
  - REQ-PLUGINS-TASKLIST-FACETS-003
acceptance_criteria:
  - AC-PLUGINS-TASKLIST-FACETS-001.1
  - AC-PLUGINS-TASKLIST-FACETS-001.2
  - AC-PLUGINS-TASKLIST-FACETS-001.4
  - AC-PLUGINS-TASKLIST-FACETS-001.5
  - AC-PLUGINS-TASKLIST-FACETS-002.2
  - AC-PLUGINS-TASKLIST-FACETS-002.3
  - AC-PLUGINS-TASKLIST-FACETS-002.5
  - AC-PLUGINS-TASKLIST-FACETS-002.7
  - AC-PLUGINS-TASKLIST-FACETS-003.1
  - AC-PLUGINS-TASKLIST-FACETS-003.3
system_design:
  - ../../specs/plugins/system-design/task-list-facets.md
---

# Task 05: Prove Facets End To End From Workspace User State

## Summary

Make the fixture facet read its values from a workspace-scoped user-state
document that a spec can drive through the real API, and prove the desktop and
phone outcomes against the running app: repeated multi-label rows, the trailing
unassigned bucket, a live re-section, the deep-link round trip, and the
unavailable-plugin fallback.

## In scope

- Edit the fixture source
  `apps/web/e2e/fixtures/plugins/prompt-history-plugin/bundle.js` so the facet's
  values come from a workspace user-state document instead of
  `window.__e2eFacetValues`:
  - hydrate a synchronous cache from
    `host.storage.get("workspace", workspaceId, "facet-values")`, following the
    existing Notes panel hydration pattern in the same file;
  - refresh the cache from
    `host.storage.subscribe({ scope: "workspace", scopeId: workspaceId }, ...)`;
  - keep `subscribe(listener)` notifying registered listeners whenever the
    cached values change, so the host re-reads and re-sections;
  - keep the failure-injection hook, and drive it from the rewritten desktop
    spec: set `window.__e2eFacetValues.__throwFor` for one task — the marker the
    fixture exposes at
    `apps/web/e2e/fixtures/plugins/prompt-history-plugin/bundle.js` — and assert
    that task still renders in the unassigned bucket while the other rows keep
    their sections. The spec already drives the facet through `page.evaluate`
    plus the fixture's notify hook, so this is the same mechanism with the throw
    marker set. That makes the documented `getValues` failure path observable at
    the browser boundary instead of only at unit level;
- Regenerate the packaged copy and tarball with
  `make -C apps/backend e2e-plugin-package` (it invokes
  `apps/web/scripts/build-e2e-plugin.mjs` with the required `--outDir`; the bare
  `pnpm run build:e2e-plugin` fails without one). Do not hand-edit
  `apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js`; it is a
  byte-identical copy of the fixture source.
- Extend `apps/web/e2e/tests/plugins/task-list-facet.spec.ts`:
  - drive values with `apiClient.rawRequest` against
    `PUT /api/plugins/<PLUGIN_ID>/user-state/workspace/<workspaceId>/facet-values`;
  - assert the built-in Sort and Group options precede the facet entry;
  - group by the facet and assert a task carrying two values has one row in each
    of two sections, that a task with no value appears in the trailing
    unassigned section labelled with the `sidebar:groupUnassigned` copy
    ("Unassigned", not "Ungrouped"), and that the unassigned section is last;
  - change the document again and assert the sections change live, with no
    reload;
  - sort by the facet and assert the rendered ordering, then assert the
    browser-side half of `AC-PLUGINS-TASKLIST-FACETS-002.7`: force a task-list
    request the spec owns (change the search query or the page), assert at least
    one intercepted task-list request happened, and fail on any `sort` value
    whose trimmed form starts with `facet:` — trimming matters, because a padded
    facet identifier starts with a space and a bare prefix test lets it through.
    The count assertion is load-bearing: `shouldSkipInitialTasksFetch` exempts
    only the untouched first page (`pageIndex === 0`, empty query, archived
    hidden), so a plain navigation issues no task-list request and the sort check
    would pass vacuously. The search or page change is what issues the request
    this assertion intercepts. Do not assert the first load this way.
    Playwright drives a Go-served SPA, so the first page's tasks come from the
    boot handler server side and the browser issues no task-list request for
    them; the document
    request must keep `?sort=facet:...` per `AC-PLUGINS-TASKLIST-FACETS-003.1`,
    and the first-load half is proved by the backend boot test in task 02;
  - open `/tasks?group=facet:<pluginId>:<facetId>` directly and assert the facet
    is the bound selection and its sections render;
  - disable or uninstall the plugin after storing the facet and assert three
    separate things, keeping query and storage apart: the list and control show
    the built-in default; the `/tasks` query still holds the facet; and the
    stored setting still holds it, read back through the user-settings API
    (`tasks_list_group` / `tasks_list_sort` = `facet:<pluginId>:<facetId>`). The
    storage assertion is the discriminating one: a regression that writes the
    built-in fallback into the setting while leaving the query untouched passes
    the first two, and the restoration below can then resolve from the query
    alone;
  - reload `/tasks` while the plugin is still unavailable and re-assert the
    stored setting, so the retention is proven across a fresh read rather than
    from in-memory state;
  - then re-enable and assert the facet selection returns, and that the setting
    was never rewritten in between.
- Extend `apps/web/e2e/tests/plugins/mobile-task-list-facet.spec.ts` for the
  same user-state-driven values, the facet options in the Display Options sheet,
  the multi-value and unassigned sections at the phone viewport, and zero
  document horizontal overflow.

## Out of scope

- Changing the released tags plugin or any other plugin's bundle. The
  released-Tags-plugin acceptance line is proved here by a shape-equivalent
  registration (the fixture facet uses the same synchronous context-only
  `getValues` plus `subscribe` signature, and the SDK change is type-only); the
  real bundle is not installed, because it ships a Go server and this harness
  stays hermetic. The check that closes that line is a defined release-time
  procedure, recorded in the plan: install the released package from a released
  tag into a build of this branch and confirm `Tag` appears after the built-in
  options, that grouping renders one section per tag with a multi-tagged task in
  each and leaves the unassigned bucket last, and that **Sort = Tag** orders the
  tasks by their alphabetically first tag label with an untagged task last. Do
  not describe it as an unspecified follow-up, and do not claim the automated
  suite covers it.
- New fixture plugin capabilities: `user_state: true` is already declared in
  `manifest.yaml`.
- Unit and component coverage (tasks 01, 03, and 04).

## Acceptance

- The desktop spec asserts, against the running app: the built-in options come
  first, the repeated multi-label rows, the trailing unassigned bucket under the
  `sidebar:groupUnassigned` copy, a task whose `getValues` throws rendering in
  that bucket while the other rows keep their sections, a live re-section after
  the workspace user-state document changes, the deep-link round trip (the
  document request
  keeps `?sort=facet:...` while the app's own task-list requests carry a
  built-in sort), and the unavailable-plugin fallback: the list and control show
  the built-in default, the query keeps the facet, and the stored setting still
  reads back as the facet before and after a reload with the plugin absent, so
  retention is proven from storage rather than from the query.
- The phone spec asserts the same facet in the Display Options sheet, the
  multi-value and unassigned sections at a phone viewport, and no document
  horizontal overflow.
- Both specs pass against artifacts rebuilt from this branch.

## Verification

```bash
make build-backend
cd apps/web && pnpm run build:e2e
make -C apps/backend e2e-plugin-package
cd apps/web && pnpm e2e:run --no-build tests/plugins/task-list-facet.spec.ts
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/plugins/mobile-task-list-facet.spec.ts
```

## Files likely touched

- `apps/web/e2e/fixtures/plugins/prompt-history-plugin/bundle.js`
- `apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js` (regenerated)
- `apps/web/e2e/tests/plugins/task-list-facet.spec.ts`
- `apps/web/e2e/tests/plugins/mobile-task-list-facet.spec.ts`

## Dependencies

Tasks 02, 03, and 04. The persistence round trip must be able to store the facet
before the deep-link and fallback assertions can mean anything.

## Risks

- The harness serves the SPA from the Go backend, so there is no browser-side
  task-list request for the first page and no Next server in the e2e path. An
  assertion that intercepts the first load cannot pass; the first-load half of
  `AC-PLUGINS-TASKLIST-FACETS-002.7` belongs to the backend boot test.
- `getValues` is synchronous, so the document must be hydrated before the first
  resolution and kept current by the subscription. A wait-for-first-hydration
  race shows up as a spurious unassigned bucket.
- The user-state change reaches the page over the
  `plugin.user-state.updated` WS action. A spec that asserts the re-section
  immediately after the PUT needs the app's own assertion retry rather than a
  fixed sleep.
- Re-enabling the plugin reloads the bundle; the restored selection must come
  from the stored preference, not from in-memory state.
- The fixture is hashed and copied by the build; editing the packaged copy
  instead of the source is silently reverted on the next package run.

## Parallelism

`sequential`; this work order is the package's end-to-end evidence.

## Inputs

- `REQ-PLUGINS-TASKLIST-FACETS-001`, `REQ-PLUGINS-TASKLIST-FACETS-002`,
  `REQ-PLUGINS-TASKLIST-FACETS-003`
- `docs/specs/plugins/system-design/task-list-facets.md`
- `docs/decisions/2026-09-22-plugin-facet-selection-persistence.md`
- `apps/web/e2e/helpers/plugin-fixture.ts`, `apps/web/e2e/helpers/api-client.ts`

## Results

Ported onto the current `main` base (`02ff057835`) and re-verified here against
the rebuilt artifacts. The reference branch recorded the RED observation (the
spec wrote a workspace user-state document while the fixture still read only
`window.__e2eFacetValues`); this port carries the same fixture change and specs.

- Fixture: `apps/web/e2e/fixtures/plugins/prompt-history-plugin/bundle.js` now
  hydrates a synchronous per-workspace cache with `host.storage.get` and
  refreshes it on `host.storage.subscribe`, notifying the registered facet
  listeners. `make -C apps/backend e2e-plugin-package` regenerated
  `apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js`; `cmp` confirms
  the packaged copy is byte-identical to the fixture source.
- Build: `make -C apps/backend build`, `pnpm --dir apps/web run build:e2e`, and
  `make -C apps/backend e2e-plugin-package` all passed.
- `pnpm e2e:run --no-build tests/plugins/task-list-facet.spec.ts` (apps/web):
  2 passed in 30.7s, no flaky retries, host mode, one worker.
- `pnpm e2e:run --no-build --project mobile-chrome
  tests/plugins/mobile-task-list-facet.spec.ts` (apps/web): 1 passed in 13.8s.
- Coverage: desktop covers built-in option precedence, facet sort/section
  ordering, a repeated multi-label row, the trailing `Unassigned` bucket, a
  throwing `getValues`, a live document update re-sectioning the list, the
  deep-link selection, a real task-list request whose `sort` stays built-in, and
  the stored preference/query surviving disable, reload, and re-enable. Phone
  covers the Display Options sort and group pickers, the multi-value rows, the
  trailing bucket, and zero document horizontal overflow.
- The released Tags package still needs the manual released-package procedure in
  the plan; the automated fixture is shape-equivalent, not that bundle.
