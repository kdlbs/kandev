---
created: 2026-10-10
status: implemented
requirements:
  - REQ-UI-TASK-LAYOUT-PROFILES-001
  - REQ-UI-TASK-LAYOUT-PROFILES-002
system_design:
  - ../../specs/ui/system-design/task-layout-profiles.md
  - ../../specs/ui/system-design/task-layout-plugin-panels.md
legacy_specs: []
---

# Implementation Plan: Plugin Task Panels in Reusable Layout Profiles

## Overview

Make installed plugin task panels available through Add panel at
`/settings/preferences/layouts`, then save, reload, and apply their placement
through the existing reusable profile paths. Establish registry-independent
eligibility first, because a menu-only change would leave valid additions
rejected by the profile validator. Follow with the complete settings/runtime
slice, including disposable real Prompt History browser proof.

Implemented 2026-10-18 after the task owner's explicit implementation request;
both work orders are done and carry their own RED/GREEN and gate results.

## Scope

### In scope

- Reactive discovery of every registered plugin task panel, localized titles,
  whole-layout duplicate exclusion, and a usable plugin-only Add panel menu.
- Generic plugin definitions added through existing split/action capture paths.
- Structural validation, including malformed definitions before retirement
  filtering, duplicate checks, Agent invariants, and nested split trees.
- Built-in overrides and custom profiles saved/reloaded through `saved_layouts`,
  task application, canonical title capture, and unchanged runtime lifecycle.
- Focused consumer-behavior unit/component and desktop/mobile Playwright proof.
- Real Prompt History package smoke using a disposable host/data directory.
- Existing public documentation updates and user-requested verification/commit.

### Out of scope

- Plugin-specific allowlists, the retired built-in Prompt History component,
  plugin repository changes, plugin API redesign, new flags, new storage, or
  deployment/restarting the user's primary instance.
- Automatic reappearance of revoked live tabs, mobile/tablet task-layout
  customization, or rewriting existing task-specific layouts.

## Confirmed intent and evidence

The user reports Kandev `0.97.1-nightly.sha3fed5570cec5`, active Prompt History
`1.4.1`, and a browser registration for
`plugin:kandev-plugin-prompt-history:prompt-history`. Their screenshot and
isolated shipped-validator reproduction establish the gap; do not rerun that
reported failure simply to confirm it. Current worktree source independently
shows static-only menu discovery and `unsupported-panel` for plugin IDs.
No material product choice is unresolved.

UI owns this extension because the durable contract is a reusable presentation
preference. Plugins owns registration and live lifecycle. Extend the existing
requirement document; the technical extension is split into a bounded companion
design rather than creating a second feature owner.

## Technical approach

### Profile eligibility

In `apps/web/lib/layout/layout-profiles.ts`, recognize the generic definition
using existing constants, `parsePluginPanelId`, and `pluginPanelId`. Require
matching ID/component/tab/params without consulting registry readiness.
Prevalidate plugin-intended definitions before component filtering and Agent
normalization so a malformed component cannot be silently removed or turned
into Agent. Preserve retirement filtering for unrelated obsolete components.
Keep tree-authoritative traversal and global duplicate counting.

Use the existing profile compatibility/default and immutable profile operations;
no backend change is expected. Validate input immutability, built-in override
and custom-profile round trips, nested structure, and missing/duplicate Agent.

### Editor discovery and actions

`layout-editor-toolbar.tsx` subscribes with `usePluginRegistry`, appends
`getTaskPanels()` to current built-in choices, derives canonical IDs with
`pluginPanelId`, resolves display via `resolveTaskPanelTitle`, and removes
already-present identities. Use the combined list for disabled state.
Do not evaluate task-specific `visible` or `mobileEnabled` in profile settings.

Extend `addReusablePanel` in `layout-editor-actions.ts` to resolve built-in or
currently registered plugin definitions. Re-resolve at selection time; retain
duplicate prevention, selected/fallback group behavior, and boolean mutation
result. Preserve controlled click-open touch behavior and `state.perform`
capture. Do not call task-workbench store actions from settings.

No new preview renderer: `layout-editor.tsx` already provides generic plugin
placeholders and editor tabs. Preserve them and prove the plugin Component is
never mounted there. `useLayoutSettings.updateLayout` and shared floating save
must accept plugin-containing drafts via the corrected validator.

### Serialization, task application, and lifecycle

Reuse `panelTitle`, `canonicalPluginPanelTitle`, `toSerializedDockview`,
`fromDockviewApi`, and generic structural retention, but do not treat the
live-registry title lookup as durable fallback evidence. Task 02 adds an
editor-owned canonical title map, seeded from the input draft and canonical
registrations at addition, retained across registration loss and locale
changes. Capture replaces localized fallback titles from this map in tree
leaves and their flat projection. Mounted preview fallback/title reconciliation
must not dirty or move the layout; no plugin params, schema, or store is added.
Prove both registered-then-revoked and initially-unavailable editor orderings.

Task 01 preserves unchanged `LayoutState` metadata in
`session-panels.ts`'s `rewriteReusableChatPanels` and `serializer.ts`'s
`filterEphemeral`; both currently return only columns and lose root orientation.
Normalization, Agent materialization, and workbench reusable capture must retain
explicit `rootOrientation`. It also owns root-axis serialization and sizing:
VERTICAL uses available height and orthogonal width, ignores horizontal pin
overrides/caps (including old pinned flags), and capture does not infer width
pinning from vertically placed Files/Changes.

Task 02 owns the orientation guards in custom/default store geometry,
applier, post-restore builders/fixups, environment-switch sizing, state pin
enforcement, and mounted task setup. Guard width constraints, root resize,
target refresh, sash classification/persistence, and width bookkeeping;
vertical extents cannot become horizontal targets or manual right widths.
Retain existing nested-tree weights and HORIZONTAL/legacy width policy.
Prove a non-square VERTICAL-root plugin profile with Files/Changes and
historical pinned metadata through override/custom save/reload, explicit
apply, fresh default, Reset Layout, task reload, and container resize. Assert
root-axis proportions, nested widths, tree/flat projection, order/selection
and placement, not just enum retention. These local fixes are required scope.

Task 02 disables `canUseFastEnvSwitch` when either current or incoming layout
contains generic plugin panels: inspect saved serialized definitions, or the
effective default for an unsaved environment, without registry lookups. Use
existing slow restore/default build and orientation-safe fixups so exact IDs,
params, nested structure, order and selections are restored. Do not redesign
comparators or make plugins ephemeral. Plugin-free optimization remains intact;
plugin-bearing switches pay the normal restore cost rather than reusing a
component-only skeleton that can represent another plugin or placement.

The new slow fresh-default path must preserve horizontal reusable geometry:
carry `performBuildDefault`'s resolved targets through its final
`applyLayoutFixups`, subsequent enforcement and mounted automatic refresh.
Actual manual width wins, then capped reusable targets, then stock defaults
when no reusable target exists. Reuse existing in-memory geometry/resolver
paths, scope ownership to the active build/environment, and do not fabricate
manual preferences or write portable settings. Keep saved-environment,
global-sidebar and code-preset policies unchanged.

Keep profile storage distinct from environment-scoped runtime layouts.
Registry changes remove menu choices, never portable placement. The existing
`useCloseRevokedPluginPanels` retains loading/failed states and closes definitive
revocations. It must not be mounted in settings. Re-enable plus explicit
profile reapplication can restore saved placement; automatic reopening is out
of scope. Existing customized tasks keep precedence over default changes.

### Compatibility matrix

| Source/state | Identity | Intended behavior | Evidence | Unsupported fallback |
| --- | --- | --- | --- | --- |
| Existing built-in panel | Static reusable ID | Existing add, Agent, and placement rules | Existing profile/unit/browser regressions | Existing legacy handling |
| Registered plugin, including titleKey | Generic matching ID/component/tab/params | Offer once; add placeholder; render live only in task | Registry component tests, packaged fixture E2E | Revoke-before-selection adds nothing |
| Loading/reloading/failed plugin | Structurally valid saved reference | Profile remains editable; task lifecycle unchanged | Registry-independent domain and lifecycle regressions | Saved title and placeholder |
| Disabled/uninstalled/ready-omitted panel | Structurally valid saved reference | Hide option; retain portable placement; close live tab per existing lifecycle | Save/reload and revocation browser flow | Explicit reapply after availability returns |
| Malformed plugin definition | Mismatched generic shape | Reject before sanitization or normalization hides it | Table-driven domain failures | Existing invalid/legacy UI, no silent rewrite |
| Real Prompt History 1.4.1 or recorded release | Same generic shape | Add/save/reload/apply actual package | Separate disposable smoke | Report package acquisition/runtime blocker; fixture does not substitute |

## ASCII UI preview

### UI-01: Desktop Add panel and saved placement

Entry: Preferences > Layouts, built-in or custom profile selected. Expanded Add
panel state; all built-ins may already be present.

```text
Layouts                      [Profiles] [Sidebar]
[Default  Customized]        Preview (placeholders only)
[Plan Mode]                  +--------------------------+
[Preview Mode]               | Agent | Prompt History    |
[VS Code]                    | generic preview          |
[Custom: Review workspace]   +--------------------------+
                             [ + Add panel ]
                             +--------------------------+
                             | <missing built-ins>      |
                             | Prompt History           |
                             | <other plugin panels>    |
                             +--------------------------+
                                      [Save changes]
```

The menu omits panels already placed anywhere; its Prompt History row is shown
only before that panel is added. Plugin-only choices keep the trigger enabled.
If nothing is missing, the existing disabled state remains. Loading/revocation
updates choices without deleting placeholders or marking the profile dirty.
A save error retains the draft and existing page-level error behavior.
The selected split determines placement; the drawing is not a new split preset.

### UI-02: Phone settings touch selection

Entry: Settings index > Layouts. Existing profile controls and preview remain;
this edits a desktop profile, not the phone task workbench.

```text
< Settings       Layouts
[Profiles] [Sidebar]
[Default  v] [existing profile actions]
+------------------------------------+
| Agent | Files                      |
| placeholder preview                |
| [ + Add panel ]                    |
+------------------------------------+
                         [Save changes]

+------------------------------------+
| <missing built-ins>                |
| Prompt History                     |
| <other plugin panels>              |
+------------------------------------+
```

Reuse the current Radix inset bottom-sheet menu below 640px, touch rows at least
44px, and safe-area handling. The settings page owns ordinary vertical scroll;
a long menu owns its internal scroll. One tap opens, a separate tap chooses.
No horizontal document scroll, hover-only action, or alternate phone catalog.
Wider touch viewports retain the existing menu treatment.

UI-01/UI-02 cover AC-UI-TASK-LAYOUT-PROFILES-002.1-.3, .7-.8. Control order,
placeholder-only rendering, separate selection, and existing shared save are
required. Labels and spacing are illustrative and use existing localization.

## Tests

| Acceptance | File and planned behavior-level scenario |
| --- | --- |
| 001.5, 002.4-.5 | `lib/layout/layout-profiles.test.ts`: structurally valid plugin remains editable with empty/changing registry; malformed ID/component/tab/params fail before filtering; duplicates across groups/tree and Agent violations fail |
| 002.5-.6 | Same domain suite: override/custom compatibility, effective default, JSON round trip, nonmutation, nested geometry and selections |
| 002.1-.3, .7 | New `components/settings/layouts/layout-editor-toolbar.test.tsx`: registry changes update visible choices; plugin-only trigger works; occupied identities omitted; selection after revoke makes no addition |
| 002.3 | `layout-editor-actions.test.ts`: real resulting panel membership/placement and duplicate prevention, not only a mocked addPanel argument echo |
| 002.3, .5-.6 | `use-layout-settings.test.tsx`: plugin edit marks dirty, save adopts baseline, failure preserves draft, fresh hook/store hydration restores editable override/custom profile |
| 002.5-.6, .8 | Replace static `layout-editor.test.ts` mapping coverage with mounted `layout-editor.test.tsx`: pseudo-locale open/add, revoke without unmount, edit another tab, capture/save/reload, exact canonical title and fallback; symmetric initially-unavailable profile; no plugin Component mount or registry-only dirtying. `serializer.test.ts` covers resulting tree/projection/title payload |
| 002.6 | `session-panels.test.ts`, `serializer.test.ts`, and `layout-profiles.test.ts`: VERTICAL/HORIZONTAL/legacy orientation survives normalization, materialization, ephemeral filtering, validation, and profile round trip with nested selections/order/sizes |
| 002.6, 001.9-.10 | `sizing.test.ts`, `serializer.test.ts`, `applier.test.ts`, `dockview-store.test.ts`, `dockview-layout-builders-fixups.test.ts`, `dockview-pinned-enforce.test.ts`, and new `dockview-layout-setup.orientation.test.ts`: non-square VERTICAL root proportions/nested widths survive apply/restore/resize despite old pinned flags/width preferences; both enforcement boundaries and sash tracking preserve geometry/manual-width state; HORIZONTAL/legacy controls |
| 002.5-.6, 001.4/.9 | New `dockview-env-switch.plugin-layout.test.ts`: actual Dockview and real comparator/capture modules restore Notes -> Prompt History on fresh task, return to saved different identity, same IDs/different tree/order/selection, and one-sided plugin transitions; portable settings remain unchanged and plugin-free fast behavior is retained |
| 001.9-.10, 002.6 | Same new switch suite plus builders-fixups/enforcement/setup coverage: horizontal override/custom default, canonical Files/Changes/plugin right group, non-stock in-cap 20% ratio and no manual width; actual settled root extents retain resolved geometry after final fixups, rAF/enforcement and mounted refresh. Stock/manual/saved-environment controls and unchanged storage |

Use deterministic actual registry state with cleanup and real Dockview where
membership/serialization is the contract. Add only consumer-visible regression
assertions. Do not add static source-string, forwarding, or incidental-copy tests.

## E2E tests

Extend the existing settings files, using `helpers/plugin-fixture.ts` for
packaged registration and restoring baseline settings/plugin state after each
mutating scenario.

- `tests/settings/layout-profiles.spec.ts` (`chromium`): add the fixture panel
  into a selected non-Agent split, save built-in override, reload, and apply to a
  task. Duplicate/create a custom profile with an explicit VERTICAL root,
  Files/Changes/plugin child and nested placement on a non-square host.
  Save/reload, apply, open a fresh default task, Reset Layout, reload the task,
  and resize its container; prove actual fixture content, root-axis
  proportions, nested widths, tree order, and group/tab selection. Drag a
  vertical root sash and verify manual right-width state is unchanged.
  Include HORIZONTAL/legacy cap/manual-width compatibility controls.
  Show plugin-only Add panel and occupied-identity exclusion. Use pseudolocale
  display plus canonical payload assertions. Covers 002.1-.3, .6, .8.
- Same desktop file: disable/enable or unregister/reload fixture contribution;
  settings remains editable and reusable placement unchanged, while live task
  cleanup respects existing lifecycle. Covers 002.5 and existing 001.4/.9.
- Same desktop file: render a titleKey fixture in pseudo locale, revoke its
  registration while the editor remains mounted, reorder another tab and save,
  inspect exact canonical title/identity/params/tree, reload unavailable and
  check fallback. Cover opening initially unavailable as a separate ordering.
  Covers 002.5-.6/.8; navigating away before revocation does not exercise this.
- Same desktop file: use in-app task navigation, not `page.goto` or reload,
  from a saved Notes-bearing environment to a fresh task whose effective default
  has Prompt History in the same component skeleton, then back. Also switch
  between saved environments with the same plugin ID but different root/nested
  structure, tab order and selection. Assert actual content, canonical
  IDs/params and saved placement/selections. Covers 002.5-.6 and default versus
  existing-environment precedence; reload/direct-apply smoke alone is insufficient.
  Use the packaged fixture's existing `notes` and `prompt-history-plugin`
  registrations under `kandev-plugin-e2e`; no second production plugin or
  registration fabrication is needed. This deterministic fixture flow remains
  separate from the actual Prompt History package smoke below.
  For the fresh horizontal default, save a non-stock in-cap ratio with
  canonical Files/Changes/plugin right groups and no manual width. Measure
  settled task extents after post-build fixups and rAF/enforcement, not only
  identities or the initial serialized build. Prove both override/custom
  defaults and stock/manual/saved-environment precedence controls.
- `tests/settings/mobile-layout-profiles.spec.ts` (`mobile-chrome`): tap Add panel,
  assert no unsolicited insertion, choose the plugin row, save and reload the
  profile; prove touch target and no horizontal overflow. Covers 002.2-.3, .7-.8.
- Run existing `tests/plugins/plugin-task-panel.spec.ts`,
  `tests/task/task-default-layout.spec.ts`, and
  `tests/layout/saved-layout-session-isolation.spec.ts` with settings regressions
  to protect lifecycle and existing task precedence.

### Real Prompt History smoke

A packaged fixture is deterministic regression coverage, not acceptance of the
real plugin. In Task 02, use the managed E2E worker's disposable backend/database
and browser, or an equivalently isolated host under `/tmp`, never the primary
instance. Upload the real Prompt History package from the user's separately
cloned repository or a verified release. Record version and package checksum.
Use a temporary smoke spec with existing test-base fixtures when that is the
simplest way to get a fully isolated runtime; remove it after recording proof.

1. Prove isolated host revision, ports, data/config/plugin directories before
   any settings write; no live settings/database copy or primary restart.
2. Register real Prompt History; at the settings route choose it from Add panel.
   Check the preview is a placeholder, duplicate exclusion, and dirty/save state.
3. Save/reload a built-in override and custom profile with non-default/nested
   placement; inspect persisted identity/params and group/tab/size structure.
4. Open a disposable task/session with a distinct prompt; apply the profile and
   observe that prompt in actual Prompt History content in the saved group.
   Reload the task, repeat with the other profile, and prove structure persists.
5. Disable the package; verify menu removal and unchanged editable reusable
   payload. Re-enable, explicitly reapply, and verify saved placement/content.
6. Capture desktop/phone screenshots, record exact exercised paths and limits,
   close the browser, stop only owned processes, and remove disposable artifacts.

## Public documentation and companion records

After implementation smoke, update the existing how-to section in
`docs/public/sessions-and-review.md`: Add panel includes installed registered
plugin task panels, settings uses placeholders, save/reload/apply behavior, and
availability versus portable placement. Link from `docs/public/plugins.md`
only if it avoids duplicating instructions. Historical `CHANGELOG.md` is
generated release history; do not hand-edit it. The requested Conventional
Commit provides the generated changelog entry.

The original [layout package](../task-layout-profiles/plan.md) and
[PR Details package](../pr-details-layout-panel/plan.md) are completed historical
records; do not reopen their work orders or overwrite their old evidence. This
plan owns the new scenarios, statuses, and results. No system README or new ADR
is required because ownership/identity/lifecycle/storage remain unchanged.

## Work orders

- [x] [Task 01: Registry-independent profile eligibility](task-01-plugin-profile-validation.md)
- [x] [Task 02: Plugin add/save/reload/apply flow](task-02-plugin-layout-editor-flow.md)

Task 02 depends on Task 01. The design handoff required a later explicit
implementation request before delegation; the task owner has now authorized an
Opus implementation subtask. The earlier read-only adversarial review did not
itself authorize production edits or implementation workers.

## Verification results

Implementation results (2026-10-18): both work orders passed their acceptance
checks; exact RED/GREEN, commands, counts and limits are in
[Task 01](task-01-plugin-profile-validation.md#results) and
[Task 02](task-02-plugin-layout-editor-flow.md#results). Summary: focused unit and
integration suites pass (232 + 53 tests in the work-order sets), desktop
Playwright 24 passed and mobile 6 passed, and the real Prompt History 1.4.1 package
(SHA-256 `12f0ba12...c5d`) was exercised on a disposable host for add, save, reload,
apply, disable/re-enable and a phone menu. `make test` is not green in this
environment (shared Go cache volume full, Node 22 and missing `unzip`/Docker
bridge); see Task 02 for the classified failures. No contract acceptance is implied.

Implementation review closure (2026-10-25):

- Fourteen independent adversarial implementation reviews completed. Rounds
  1-11 produced actionable findings, addressed in the commits and detailed
  RED/GREEN/runtime records in Task 02 Results. The implementation worker
  initially used Sonnet rather than the requested Opus profile; this routing
  mistake was disclosed. Review round 1 used Opus; subsequent reviewers and
  delegated remediation used the exact Sol 6.1 profile.
- Rounds 12, 13 and 14 independently reported zero actionable findings against
  unchanged implementation `e3b1603af00a401a6dd3e20f792159a404241fc1`.
  Three-consecutive-clean stopping condition met; no round 15 scheduled.
- Final implementation evidence: 43 adjacent files / 649 tests, a complete
  29-test desktop aggregate on freshly rebuilt production artifacts, and
  8 mobile tests on those artifacts passed. Round 14 independently executed
  59 external consumer probes, not browser/backend tests.
- Historical full `make test` environment failures were not claimed resolved.
  Real Prompt History 1.4.1 disposable smoke predates the review fixes; no
  exact-final-head real-package re-smoke is claimed. The pre-existing review
  parameter serialization/reveal limitation and delayed-sash environment race
  remain outside this feature, as independently classified in review.
- Review closure is not contract-owner acceptance, deployment or publication.
  No push, PR, primary-instance mutation or workflow completion signal occurred.

Design-package evidence (2026-10-10):

- `python3 scripts/list-docs.py validate`: passed; 370 decisions and 1521
  specifications validated.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `validateCoverage` from `.github/scripts/pr-docs.cjs`, executed against the
  six current package files and a simulated planned toolbar source change:
  `ok: true`, `status: covered`, no errors, both work orders accepted with
  REQ-UI-TASK-LAYOUT-PROFILES-001/.002 and their corresponding design paths.
  This checks plan/requirement/design/AC links; it is not a production diff.
- Catalog discovery includes `task-layout-plugin-panels.md`. Both design files
  are below 32 KiB; requirements are below 20 KiB.
- `git diff --check -- docs/specs docs/plans/plugin-task-layout-profiles`:
  passed. Handoff status identifies the two modified existing specs, the new
  extension design, and the new plan directory; files remain uncommitted.

Round 1 adversarial design remediation (2026-10-11):

- Three actionable findings accepted: retain canonical titles across mounted
  registration loss; preserve root orientation in Chat rewrites and ephemeral
  filtering; keep the baseline requirement active with only 002 marked draft.
- The design and work orders now own those local fixes and deterministic
  registered/revoked and initially-unavailable title tests, plus nested
  VERTICAL/HORIZONTAL/legacy orientation tests and desktop application proof.
  These are implementation obligations, not executed product regressions.
- Re-ran catalog validation (370 decisions/1521 specifications), specification
  lint tests (36 passed), and full specification lint: passed.
- `python3 scripts/list-docs.py specs --system ui --kind requirement --status active --format paths`:
  includes `docs/specs/ui/requirements/task-layout-profiles.md`.
- Exported `validateCoverage` preflight using the six package documents and
  simulated editor, toolbar, session-panel, and serializer source changes:
  `ok: true`, `status: covered`, no errors; both work orders and requirement/design
  links accepted. This is not a production-change or runtime verification.
- Scoped `git diff --check` passed. Both designs remain below 32 KiB and the
  requirement below 20 KiB.

Round 2 adversarial design remediation (2026-10-11):

- Two P2 readiness findings accepted: orientation metadata alone does not
  correct width-axis geometry; component-only environment-switch eligibility
  can retain another plugin or another saved placement.
- Task 01 now owns serializer/sizing root-axis correctness and capture pin
  inference. Task 02 owns orientation guards across store geometry callers,
  applier, restore fixups, environment sizing, both enforcement boundaries, and
  sash/width bookkeeping. Non-square root proportions/nested widths are required
  through apply, fresh default, Reset Layout, reload and resize, with existing
  HORIZONTAL/legacy width behavior protected.
- Task 02 uses the existing slow restore/default build whenever either side
  contains generic plugin panels. New actual-Dockview integration uses real
  comparator/capture modules; desktop E2E now includes SPA navigation between
  different identities and same-identity/different-structure saved environments.
  Plugin-free fast behavior and revocation policy remain unchanged.
- Source tracing supports these obligations; no product regression or runtime
  counterexample was executed during design remediation.
- Catalog validation passed (370 decisions/1521 specifications), full
  specification lint passed, and all 36 linter tests passed.
- `validateCoverage` using six current documents plus simulated planned
  editor/toolbar, session, serializer/sizing, applier/store, builders, pin/setup,
  and environment-switch source paths: covered, `ok: true`, no errors, both
  work-order requirement/design links accepted. This is not an implementation
  diff. Both designs remain below 32 KiB; requirements remain below 20 KiB.
- Scoped whitespace checks passed. Real-package smoke, product tests, requested
  broad gates and commit remain pending implementation.

Round 3 adversarial design remediation (2026-10-11):

- One P2 finding accepted: the newly mandatory plugin slow fresh-default path
  builds resolved horizontal geometry, then stock-ratio fixups can overwrite
  that geometry. This is source-derived evidence, not an executed runtime case.
- Task 02 explicitly carries resolved reusable targets through final fixups,
  scheduled enforcement and mounted automatic refresh, scoped to the active
  build/environment. Keep actual manual precedence, caps, global sidebar,
  saved-environment and stock-preset policies; no fake manual preference or
  portable settings write.
- Actual-Dockview regression and desktop SPA navigation now require settled
  horizontal override/custom geometry with canonical Files/Changes/plugin
  groups and non-stock in-cap 20% right ratio. Builders/fixups/enforcement/
  refresh are real; initial map or mocked resize assertions are insufficient.
- Catalog validation passed (370 decisions/1521 specifications), full
  specification lint and scoped whitespace checks passed.
- Current six-document plus simulated affected-source `validateCoverage`
  preflight: covered, `ok: true`, no errors, both work orders accepted.
  Designs remain below 32 KiB and requirements below 20 KiB. Product tests,
  browser/runtime smoke, broad execution gates and commit remain pending.

Adversarial design review completion (2026-10-11):

| Round | Actionable findings |
| --- | --- |
| 1 | 3 |
| 2 | 2 |
| 3 | 1 |
| 4 | 0 |
| 5 | 0 |
| 6 | 0 |

- Six independent read-only reviews reported exact GPT 6.1 Sol runtime/profile
  evidence. Findings from rounds 1-3 were addressed in contract/work-order
  ownership and consumer verification schedules, not production implementation.
- Rounds 4-6 completed with no actionable findings. The three-consecutive-clean
  stopping condition is met; no round 7 is scheduled.
- Parent SHA-256 checks matched all six package documents to the final clean
  reviewed snapshot before adding this completion record. Requirements,
  designs and work-order contracts remain unchanged; only manifest evidence
  is appended. Reviewed manifest SHA-256:
  `f7c3e75652d8a5bb311ead36f89e7b1cc810f8a71c948198c21c912ee566d9a4`.
- Conversation histories record completed self-archive tool events for each
  reviewer after report delivery; the parent was not archived.
- Ready for a later explicit implementation request. This is not implementation,
  runtime, TDD, browser, or merge acceptance. Both work orders remain pending.

No production/permanent-test edits, dependency installation, product checks,
browser smoke, runtime/settings mutations, or commit ran in this design turn.

## Risks

- Filtering before validation can hide malformed plugin components or normalize
  them into Agent. Check raw authoritative plugin definitions first.
- Registry-dependent profile validity would turn a valid default into fallback
  during asynchronous plugin boot. Structural checks must remain independent.
- Tree and flat group projections describe the same panels; validating both as
  independent groups double-counts valid profiles.
- Localized display must not replace canonical titles in captured saved data.
- Touch menu pointerdown behavior is already a regression fix; preserve it.
- Successful live revocation can remove environment tabs, not saved profiles;
  do not promise automatic live reopening.
- The actual Prompt History tarball is an execution prerequisite for smoke;
  record acquisition failures rather than substituting the fixture.
- No TypeScript LSP is currently available. Reference lookup was attempted;
  before changing exported signatures, retry availability and map all callers
  with focused repository searches if still unavailable.
