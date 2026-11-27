---
created: 2026-10-06
status: complete
requirements:
  - REQ-UI-NAV-HIERARCHY-001
  - REQ-UI-NAV-HIERARCHY-003
  - REQ-UI-NAV-HIERARCHY-004
  - REQ-UI-CONTROL-SIZING-001
  - REQ-UI-SIDEBAR-CUSTOMIZATION-001
  - REQ-UI-SIDEBAR-CUSTOMIZATION-004
  - REQ-UI-SIDEBAR-CUSTOMIZATION-005
  - REQ-UI-SIDEBAR-CUSTOMIZATION-006
  - REQ-UI-SIDEBAR-CUSTOMIZATION-007
  - REQ-UI-NEEDS-YOU-INBOX-001
system_design:
  - ../../specs/ui/system-design/navigation-hierarchy.md
  - ../../specs/ui/system-design/control-sizing.md
  - ../../specs/ui/system-design/sidebar-customization.md
  - ../../specs/ui/system-design/needs-you-inbox-03.md
legacy_specs: []
---

# Configurable sidebar presentation

## Overview

Add independent sidebar preferences for fast action icons and New Task styling.
New users receive the simple design with smaller controls. Existing users retain
the compact row and section shortcuts. Task 01 delivers the preferences and
upgrade behavior. Task 02 adds direct sidebar customization using those settings
and the existing saved layout contract. Task 03 adds the persisted navigation/Tasks split. Execute them sequentially.

UI owns this reusable presentation contract. User settings retain persistence
ownership. Source baseline: `40dcfeacb2c98cf650132c7c787340114c1b91ad`, PR #4239.
The [previous placement package](../sidebar-action-placement/plan.md) remains
completed historical evidence. This revision replaces its unconditional placement
with account preferences; it does not invalidate its recorded test results.

Confirmed: icons default off for new users and on for existing users; the simple
creation style defaults for new users; the old compact style remains for existing
users; the settings live in Layout > Sidebar. The approved preview puts labelled
quick actions below New Task when icons are off and moves them beside it when on.
The latest screenshot confirms that the simple button and action bar need less height.
Routine implementation choices: user-wide scope, `simple`/`compact` style values,
28px simple controls, and the full preference-state matrix below. No unresolved
product decision blocks this package. Existing settings and navigation ADRs cover
the boundaries; no additional ADR is needed for these local presentation choices.

The subsequent user request adds a shared right-click customization menu and
direct drag reordering. The final correction is explicit: no drag handles or
hover grips; only change the cursor to `grabbing` during an active drag.
Visibility, order, and preference changes made here persist to the same database
settings as the Layout > Sidebar page. Direct actions save immediately, while
the settings page retains its draft and shared Save/Discard behavior.

## Scope

In scope:

- Persist both preferences through the existing settings API, boot payload, and WS updates.
- Backfill only missing preferences for existing users and seed new-user defaults explicitly.
- Add a Sidebar appearance settings group with shared Save/Discard participation.
- Apply preferences to expanded desktop creation and built-in section shortcuts.
- Reduce the simple creation button and labelled utility bar to ordinary desktop density.
- Preserve named child navigation, plugin ownership, saved layouts, phone behavior, and Stats placement.
- Localize new copy and update the public sidebar tutorial when the behavior exists.
- Add right-click visibility choices, preference controls, and a final settings link.
- Support handle-free top-level drag ordering, keyboard reorder, and phone customization.
- Include Inbox visibility/order in the same layout model and Settings editor.
- Preserve concurrent saves, hidden/ineligible nodes, workspace scope, and dirty Settings drafts.

Out of scope: runtime feature flags, task mutations, provider eligibility changes,
task-row dragging, nested shortcut reorganization from navigation, phone redesign,
plugin registration changes, commits,
pushes, or PR publication without separate authorization.

## Technical approach

Follow the paired [navigation design](../../specs/ui/system-design/navigation-hierarchy.md),
especially Saved sidebar preferences and Desktop composition.

1. Extend the settings model and optional PATCH DTO, response projection, store
   codec, service validation/application, and settings event. Invalid types,
   nulls, and unknown styles fail without partially saving a request.
2. Backfill missing JSON keys transactionally before `ensureDefaultUser` creates
   any new record. Preserve explicit values and unknown keys. Seed both new
   defaults in `CreateUser` and `ensureDefaultUser`. Verify SQLite and the existing
   PostgreSQL repository path. Migration errors roll back and fail initialization.
3. Extend client settings types, defaults, and shared SSR/WS mapping. Keep partial
   updates and stale-revision filtering. Server hydration establishes the initial layout.
4. Add an account-scoped preferences group in the Sidebar tab. Reuse the settings
   contributor and draft/rebase pattern. Do not couple these preferences to a
   workspace layout reset or add a second Save button.
5. Branch the creation presentation without replacing its dialog host or callbacks.
   Gate Canvases and Integrations header shortcuts with the same preference. Add
   a labelled canvas settings child when its optional header shortcut is hidden.
   Keep the integration child links and setup row in all preference states.
6. Add behavioral and rendered tests, translations, and documentation. Promote
   the draft requirements/design only after all three work orders pass their checks.
7. Task 02 follows the paired sidebar-customization design's Direct sidebar editing
   and Inbox layout entries sections. Extend default/saved projections and the
   Settings editor to materialize missing Inbox nodes without reordering user nodes.
   Gate eligibility separately from saved visibility, removing only Inbox from
   the protected entry set. Tasks, settings, and workspace switching remain fixed.
8. Add one navigation customization controller and ContextMenu entry point to
   primary rows and empty space. Visibility and drop operations use the existing
   layout operations and `sidebar_layout_state` CAS patch. Preferences use Task
   01's existing account settings keys. Serialize immediate saves, preserve dirty
   editor drafts, render acknowledged changes, and report conflicts.
9. Reuse installed dnd-kit sortable primitives for row labels with an activation
   threshold. Exclude independent quick actions, provider icons, and nested rows.
   Render no grip or drag handle in any state. Keep resting cursors unchanged;
   use `grabbing` while active and restore it after completion/cancellation.
   Suppress the drag's trailing click and show only an insertion marker.

| Creation style | Fast icons | Expanded desktop creation | Section headers |
| --- | --- | --- | --- |
| `simple` | off | Centered 28px New Task; labelled 28px utility bar below | Label and chevron |
| `simple` | on | Centered 28px New Task; 24px Terminal/Chat icons beside it | Eligible shortcuts before chevron |
| `compact` | off | Left-aligned 36px navigation row; labelled 28px utility bar below | Label and chevron |
| `compact` | on | Existing 36px navigation row; 24px Terminal/Chat icons beside it | Existing shortcuts before chevron |

Phone/coarse-pointer controls retain at least 44px targets. The collapsed rail
retains its current creation launcher instead of projecting the expanded bar.
Plugin workspace actions remain mounted with existing context and error boundaries;
they follow built-in actions and wrap when needed. The preference does not hide
custom groups, saved explicit shortcuts, or plugin actions.

## ASCII UI preview

ASCII spacing and icon abbreviations are illustrative. Control order, action
availability, separate hit targets, and measured size roles are requirements.
Views UI-01 to UI-03 cover AC-UI-NAV-HIERARCHY-004.1 through 004.6 and 001.1/3/4/7.
UI-04 covers 004.7 and the existing phone criteria.
UI-05 through UI-07 cover AC-UI-SIDEBAR-CUSTOMIZATION-006.1 through 006.8.

### UI-01: Expanded desktop, new-user defaults

Entry: normal sidebar, simple style, fast icons off. Header/footer remain fixed;
the existing task-list region owns scrolling. Current PR #4239 has a compact
creation row and header shortcuts. This view adds the alternative below.

```text
+-------------------------------------+
|             + New Task              | 28px desktop
+------------------+------------------+
|    Quick Chat    |     Terminal     | 28px desktop
+------------------+------------------+
  Home
  Automations                        >
  Canvases                           >
  Integrations                       >
  [Tasks and existing task scroller]
  Settings                 Stats Sun ...
```

When Canvases is expanded, its named settings child remains available even with
no active canvases or a failed canvas read. Integration settings remains a child
even with no eligible providers. No header shortcut implies a provider connection.

### UI-02: Expanded desktop, fast icons on

Entry: either style with fast icons enabled. The New Task height/alignment
follows its independent style. Existing users default to compact (36px);
the simple option remains 28px. Do not also mount a labelled utility bar.

```text
+---------------------------+----+----+
| + New Task                | >_ | QC |
+---------------------------+----+----+
  Home
  Automations                        >
  Canvases                       [C] >
  Integrations                GH LN  >
  [Tasks and existing task scroller]
  Settings                 Stats Sun ...
```

`[C]` opens canvas management. GH/LN represent eligible integrations, not a
fixed provider list. Compact style plus icons off uses UI-01's utility placement
with UI-02's left-aligned 36px creation control. Both settings remain independent.

### UI-03: Layout > Sidebar appearance settings

Entry: Sidebar tab on desktop or phone. New-user saved state is shown. This
group precedes the existing workspace layout editor and remains usable without
a workspace. The shared Settings save controls stay owned by the existing shell.

```text
Layout:  [Profiles] [Sidebar]
Sidebar appearance
  Show fast action icons                 [off]
  New Task button style          [New design v]
    choices: New design / Old compact design

Sidebar layout
  [Existing workspace navigation editor]

  [Localized save error, only after failure]
                      [Discard] [Save changes]
```

Changing a preference creates a settings draft. Failed saves keep the selection
and draft; retry uses the shared Save control. No second local save footer is added.
On phone, rows stack labels above controls if needed and retain 44px targets.

### UI-04: Phone navigation drawer, all preference states

Entry: existing hamburger below 768px, including narrow fine-pointer windows.
Reuse `AppNavSurface`'s inset bottom Drawer: fixed workspace header, one internal
vertical nav scroller, dynamic viewport bounds, and safe-area clearance. Keep
this temporary navigation surface rather than adding a route or another drawer.

```text
         +-----------------------------+
fixed    | Menu          Workspace [v] |
         | [Customize sidebar]         |
scroll   | [+ New Task]                 |
         | Home                        |
         | [Quick Chat] [Terminal]     |
         | Automations               > |
         | Canvases                  > |
         | Integrations              > |
         | Tasks                     > |
         | Stats / Settings / Theme    |
safe     +-----------------------------+
```

Phone navigation keeps its labelled flow in every desktop preference state.
Settings can edit the same preferences on phone without rewriting them on a
viewport change. Existing settings save and navigation guards apply.
The Customize action is introduced by Task 02; its drawer uses UI-07 below.

### UI-05: Desktop right-click customization menu

Entry: any optional top-level row or blank navigation space. Hidden entries
remain listed. Names follow current workspace eligibility and saved layout order.
The final action always opens Layout > Sidebar. Move up/down choices appear for
the selected row before that final action so keyboard users can reorder too.

```text
+--------------------------------------+
| Customize sidebar                    |
+--------------------------------------+
| [x] New Task                         |
| [x] Home                             |
| [x] Inbox                            |
| [x] Automations                      |
| [x] Canvases                         |
| [x] Integrations                     |
| [ ] Hidden custom group              |
+--------------------------------------+
| [ ] Show fast action icons           |
| New Task button style                |
|   (*) New design, smaller            |
|   ( ) Old compact design             |
+--------------------------------------+
| Move selected entry up               |
| Move selected entry down             |
+--------------------------------------+
| Open sidebar layout settings         |
+--------------------------------------+
```

Style choices are radio entries, shown together for illustration. In Office,
eligible Inbox and Needs you entries remain separately labelled. Unavailable
providers are not fabricated. Errors appear as localized feedback without
presenting an unacknowledged choice as saved.

### UI-06: Desktop dragging without handles

Entry: drag a row's label region past the activation threshold. No handles are
rendered at rest, on hover/focus, during drag, or in an edit mode. Other shortcut
controls keep their independent click behavior. Only the active drag changes
the cursor to `grabbing`; normal row cursors remain unchanged.

```text
  Home
  Inbox
  Canvases                         >
  -------- insertion marker --------
  Automations                      >
  Integrations                     >
```

Drop commits the new order once. Esc, same-position drops, invalid targets, and
workspace changes cancel without saving. Failed/conflicting saves restore
authoritative order with error/reconciliation feedback.

### UI-07: Phone customization drawer

Entry: visible Customize action in UI-04. An inset Drawer contains the same
visibility and preference controls, plus explicit reorder actions. Use existing
drawer boundaries and focus ownership. No long-press, touch-drag, or handle is
required, and edits retain the full desktop layout order.

```text
+--------------------------------------+
| Customize sidebar               Done | fixed
| [x] New Task                         |
| [x] Home                             | one scroller
| [x] Inbox                            |
| [x] Automations                      |
| [x] Canvases                         |
| [x] Integrations                     |
| Reorder entry       [Canvases v]      |
| [Move up] [Move down]                 |
| Show fast action icons         [off] |
| New Task style        [New design v] |
| Open sidebar layout settings         | last action
+--------------------------------------+
```

Controls measure at least 44px and clear safe areas. Keyboard users use the same
explicit reorder operations. Closing returns focus to Customize; navigating to
Settings closes the customization surface through existing guarded navigation.

## Tests

| Criteria | Planned evidence |
| --- | --- |
| 004.2/3 | Store migration test: legacy `{}`, mixed/explicit fields, unknown keys, revision preservation, replay, fresh reopen, new account after upgrade, rollback; SQLite and PostgreSQL |
| 004.1/6 | DTO/handler/service tests: strict inputs, unrelated PATCH preservation, round trip, event fields; client mapper and stale-event tests |
| 004.1/6 | Settings draft/helper/component tests: Save/Discard, failures, clean-event rebase, newer edits during save, no workspace |
| 004.4/5 | New Task component tests: four-state matrix, workspace-disabled creation, handlers, activity, stable dialog lifetime; section tests for gates and retained children |
| 004.7 | Existing footer and phone tests; saved-layout and creation-draft preservation |

All IDs above abbreviate `AC-UI-NAV-HIERARCHY-`. The work order owns exact files
and commands. Migration tests prove compatibility independently of screenshots.

Task 02 adds tests for all `AC-UI-SIDEBAR-CUSTOMIZATION-006.*` criteria: hidden-entry
restoration, direct-versus-Settings consistency, concurrent/failed writes,
workspace switch races, handle absence, cursor cleanup, drag click suppression,
Inbox normalization/eligibility, keyboard alternatives, and touch drawer behavior.

## E2E tests

- Chromium `tests/layout/navigation-hierarchy.spec.ts`: four preference states;
  actual 28px simple heights and 24px icons; existing 36px compact row; shortcut
  order; no duplicate utilities; launch handlers and Stats placement.
- Chromium `tests/settings/sidebar-customization.spec.ts`: shared save/discard,
  both preferences after reload, preservation of customized navigation order,
  canvas/settings access when shortcuts are off, and no-provider state.
- Mobile Chrome `tests/layout/mobile-navigation-hierarchy.spec.ts`: unchanged
  phone navigation in both preference states at 360/393/767px, settings controls,
  44px targets, containment, focus handoff, and draft retention across breakpoints.
- Mobile Chrome `tests/settings/mobile-sidebar-customization.spec.ts`: edit both
  preferences through the phone Sidebar tab, Save/Discard, and restore after reload.
- Keep the narrow fine-pointer and coarse-tablet scenarios in navigation hierarchy
  coverage. Test Portuguese labels and dark/light themes without reducing text to fit.
- Existing focus/lifecycle and plugin geometry specs must seed the preference
  state they assert; fresh fixtures must not accidentally masquerade as legacy users.
- Chromium `tests/settings/sidebar-direct-customization.spec.ts`: context menu
  contents and final settings link, restoring hidden entries, all-hidden recovery,
  immediate save/reload, Inbox visibility/order, pointer drag without handles,
  grabbing cursor, cancelled drops, click suppression, write failure/conflict,
  dirty settings draft preservation, and workspace scope.
- Mobile Chrome `tests/settings/mobile-sidebar-direct-customization.spec.ts`:
  visible entry, bounded customization drawer, 44px targets, immediate persistence,
  explicit reorder without handles, focus return, and layout-settings navigation.

## Work orders

- [x] [Task 01: Persist and apply sidebar presentation preferences](task-01-sidebar-preferences.md)
- [x] [Task 02: Customize directly from the sidebar](task-02-direct-sidebar-customization.md)
- [x] [Task 03: Persist the navigation and Tasks split](task-03-navigation-split.md)

## Verification results

Design-package validation passed on 2026-10-06:

- `python3 scripts/list-docs.py validate`: 351 decisions and 1366 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check`: passed; new plan/work-order files are visible in workspace status.
- PR-documentation coverage preflight: the actual docs-only diff is exempt.
  A local `validateCoverage` run with the new work order and one representative
  planned source path returned `covered` with zero errors, proving its references.

The menu/drag extension was validated on 2026-10-06: catalog and specification
lint passed; `git diff --check` passed; prospective implementation preflight
covered both work orders with zero reference errors. No product tests ran.
The user subsequently authorized implementation in this session. All three work
orders are implemented and verified. Final results are recorded below; the
implementation was recorded before delivery authorization. The user subsequently
authorized committing and pushing to existing PR #4239.

## Risks

- Seeding `{}` for new users would make migration replay wrongly assign legacy defaults.
- A whole-settings rewrite could lose custom layouts or unknown keys during backfill.
- Simplifying headers must not remove the only path to canvas management.
- Existing E2E assumptions use legacy icons; explicitly seed preferences where needed.
- Ordinary heights must not leak into phone or coarse-pointer controls.
- Inbox is currently fixed; every projection and validator must agree on its new optional identity.
- Row drag must preserve normal clicks and nested control behavior without a visible handle.
- Rapid direct writes or existing editor drafts must not overwrite newer database revisions.

## UI-08: Resized navigation and Tasks split

```text
Compressed                      Expanded
[       + New Task       ]      [       + New Task       ]
[ Quick Chat | Terminal  ]      [ Quick Chat | Terminal  ]
Home                            Home
~~~ bottom fading gradient ~~   Inbox
             v                  Automations             >
-----------------------------   Canvases                >
TASKS                 [view]     Integrations            >
Task A                                       ^
Task B                          ---------------------------
Task C                          TASKS                [view]
Task D                          Task A
```

The separator is the resize target, without a grip icon. The chevron uses only a
12px visible strip with an overlapping hit area. Navigation buttons keep their
normal sizes. Both chosen compressed height and expansion persist in the workspace
layout; expansion retains the compressed height for restoration. Viewport clamping
is effective only. Task 03 verifies the split, gradient, focus, reload, cancellation,
workspace isolation, and phone/Office exclusion through focused helpers and browser tests.


## Implementation results (2026-10-06)

All three work orders are complete in this workspace. New accounts receive
false/simple, existing accounts receive missing-key true/compact defaults, and
explicit choices survive restart. Both preferences travel through HTTP, WS, the
Go SPA boot payload, Settings discovery, and the generated contract snapshots.
The four presentation states use 28px simple controls, 36px compact creation,
24px inline icons, and at least 44px coarse-pointer targets.

Desktop customization includes hidden eligible entries, an all-hidden recovery
region, radio style choices, the final settings link, direct handle-free row
sorting, active grabbing cursor, and keyboard ordering. Phone customization uses
an inset nested drawer with visibility, preferences, and explicit reorder controls.
Direct writes render acknowledged settings, serialize against the newest layout,
refresh after save failures/conflicts, and preserve newer events when HTTP
responses arrive late. Settings retains Save/Discard and dirty-draft reconciliation.

The navigation/Tasks divider saves workspace height and expansion. A 12px desktop
strip and bottom fade expose clipped navigation. Expansion retains compressed
height, cancellation and unchanged drags do not write, and viewport clamps do
not replace stored geometry. Phone, Office, rail, and Settings takeover keep
their existing scrolling and focus ownership. Public tutorial and scoped guidance
are updated.

Validation passed:

- `go test ./internal/user/...`: all six user packages passed on SQLite and again
  with an isolated PostgreSQL 16 database. The explicit PostgreSQL upgrade/reopen
  case executed and passed; it was not skipped. The disposable database was removed.
- Backend boot mapping regression first failed for the missing preferences, then
  all `TestMapUserSettingsState*` checks passed after fixing hydration.
- Settings catalog tests and generation/check passed. Its three CI snapshot/schema
  tests passed. Backend lint over user, catalog, and backendapp reported zero issues.
- The broad frontend selection passed 775 tests across 69 files. Additional focused
  runs passed keyboard/rail, disabled section icons, WS/defaults/editor compatibility,
  phone canvas disclosure, lost-response recovery, no-op resize, and stale editor
  response checks. The final edge selection passed 12 tests across four files,
  and final editor/controller selection passed 14 tests across three files.
- Chromium ran the six planned legacy files (41 cases). Eight legacy presentation
  checks were corrected to select compact preferences and all passed on retry.
  The new direct-customization suite passed all five cases. The final existing
  Settings suite passed all three cases after adding the response guard.
- Mobile Chrome ran all five planned files (27 cases). The default canvas selector
  was restored on the shared projected component; the three affected canvas cases
  passed on retry. Direct drawer customization passed, including saved geometry,
  visibility, order, reload, and Settings navigation.
- Current managed browser builds passed. TypeScript, targeted ESLint with zero
  warnings, i18n completeness and the new-code ratchet passed. All seven supported
  language catalogs and generated pseudo/Traditional Chinese copy are complete.
- Public-doc validator tests: 62 passed; specification validator tests: 36 passed.
  Validation covered 47 public pages, 351 decisions, and 1366 specifications.
  Work-order documentation preflight returned covered with zero errors.
  Go formatting and `git diff --check` passed.

No unresolved blocker remains. The user subsequently authorized committing and
pushing these verified changes to existing PR #4239. Delivery evidence is tracked
in the Kandev task plan.


## Delivery checks (2026-10-06)

Normal commit hooks exposed a formatting conflict for the generated settings
contract: Prettier compacted its enum array, but the Go generator checks exact
bytes. The two generated settings snapshots now use their generator formatting
and are excluded from Prettier. The generator freshness tests and normal commit
hooks validate this correction before delivery to PR #4239.


## Approved follow-up: full collapse and menu heading

The user authorized implementing [Task 04](task-04-zero-height-and-menu-title.md).
Allow saved navigation height 0 through 1600, retaining the small chevron at zero.
Add a localized Sidebar settings heading above context-menu choices. Phone keeps
its existing inset customization drawer and must preserve a zero desktop height.

- [x] Complete Task 04 and its focused backend, unit, desktop/phone browser,
  localization, lint, type, and documentation checks.

Task 04 is complete. Backend service tests and 10 frontend regressions passed.
All six desktop browser cases and the phone case passed with retries disabled.
Fresh managed builds, typecheck, focused lint, localization, docs/spec validators,
and diff checks passed. Existing geometry assertions were corrected to use a
measured drag target and wait for viewport resize before capturing their baseline.
