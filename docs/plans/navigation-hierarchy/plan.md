---
created: 2026-09-28
status: done
requirements:
  - REQ-UI-NAV-HIERARCHY-001
  - REQ-UI-NAV-HIERARCHY-002
  - REQ-UI-NAV-HIERARCHY-003
system_design:
  - ../../specs/ui/system-design/navigation-hierarchy.md
legacy_specs: []
---

# Navigation hierarchy and seeded comparison

## Overview

Apply the supplied navigation reference and subsequent visual feedback using
neutral action surfaces, existing selection accents, and compact density. Deliver desktop action/group semantics first, the
shared contextual task panel second, phone navigation third, then leave an
isolated populated instance running for comparison. Execute sequentially in the
primary conversation after the design-package handoff.

Sources: [requirements](../../specs/ui/requirements/navigation-hierarchy.md),
[design](../../specs/ui/system-design/navigation-hierarchy.md), and baseline HEAD
`75a37f34eb6bd0e0a46e9a35a46491bb9f52f7ef`. The screenshots are layout references,
not authoritative routes, task states, account identity, or exact pixel sizes.

## Scope

In scope: primary New Task, explicit tool disclosures with indented children,
contextual Tasks and filter cues, semantic state-group headings, subtle compact
task surfaces, footer labels, native phone access to issues, saved-layout
compatibility, localized copy, focused tests, and a disposable demo.

Out of scope: new workflow/provider/account behavior, forced grouping changes,
assignee filtering, wider navigation, new preferences or feature flags, public
deployment, or marketing capture. Keep Office mode gates and custom
shortcut icon groups intact.

## Technical approach

- Desktop: `DefaultSidebarLayout`, `AppSidebarPrimaryNav`,
  `AppSidebarNewTaskItem`, `AppSidebarSection`, resource sections,
  `SidebarLayoutNavigation`, and `AppSidebarFooter`. New default New Task/Home
  order is canonical in Go; existing saved desktop order wins.
- Shared tasks: `TasksSection`, `TasksViewPicker`, `SidebarFilterBar`,
  `GroupHeader`, `GroupSection`, and `TaskItem`. Derive applied filter state from
  the effective view, and state-group indicators from resolved state keys.
  Keep bounded task reads, descendant aggregation, row configuration, and actions.
- Phone: `AppNavSheet`, `AppNavSections`, `MobileSidebarLayoutNavigation`,
  `MobileTaskNavigationProvider`, and `InlineTaskHeader`. Tools move before Tasks
  in default and saved layouts. Dialog ownership remains above the menu outlet.
- Documentation: revise the current phone-order clauses only when their successor
  is implemented. The old completed plans remain historical evidence. Update
  `apps/web/AGENTS.md` and `docs/public/use-kandev.md` with the actual new order.
- Preview: use this branch's implementation with isolated data and mock providers.
  The user's explicit comparison request supersedes the demo skill's marketing
  current-main source rule. Record branch/diff provenance instead of claiming
  this preview is a clean current-main release.

## ASCII UI preview

UI-01: desktop default expanded sidebar, Home current, Integrations expanded.

```text
BEFORE (source)                 PROPOSED
Kandev / Workspace v [collapse] Kandev / Workspace v [collapse]
Home                           [ + New Task       shortcut ]
New Task  [terminal][chat][...] Quick Chat   Terminal
AUTOMATIONS             >      Home                         *
INTEGRATIONS [provider]  >      Automations                  >
GitHub                         Canvases                     >
TASKS   All tasks v [gear]      Integrations                 v
flat task rows                   GitHub
                               --------------------------------
                               TASKS  [All tasks v] [filter *]
                               (o) To do                 [1] v
                               +------------------------------+
                               | o Fix issue search           |
                               |   compass/app #125   32s     |
                               +------------------------------+
                               (o) In progress           [1] v
                               | o Improve empty states       |
                               |   compass/app   +733 -9      |
                               (o) Review                [2] v
                               (v) Completed             [1] v
                               --------------------------------
                               [avatar] Settings   [theme] [...]
```

Header and footer stay outside task scrolling. Tasks is a section, not a second
route. Status rows are illustrative: preserve all real states and chosen grouping.
Task boxes are subtle inset surfaces with current padding, not larger cards.
The primary button uses theme tokens, 44px height, and the effective
shortcut. Existing saved layouts can place it elsewhere.

UI-02: phone app drawer from Home or a task, default/saved layout.

```text
BEFORE (source)            PROPOSED
Menu                 x    Menu                       x
Workspace v               Workspace v                 [fixed]
Home                      -------------------------------
Chat | Terminal           [ + New Task               ]
Tasks v              +    Home
  [potentially long list] Chat | Terminal
Automations >             Automations                 >
Integrations >            Canvases                    >
Utilities                 Integrations                v
                            GitHub
                            Integration settings
                          [eligible plugins/custom tools]
                          -------------------------------
                          TASKS v [All tasks v] [filter]
                            [compact touch task rows]
                          Utilities / Settings
                          [bottom safe area]
```

One scrolling body below the fixed workspace/header. Tools precede task rows.
Standalone changed controls measure at least 44px. A hidden built-in New Task
restores the Tasks plus; explicit custom shortcuts remain. The separate task-title
picker remains. At 768px, preserve the wider layout and its pointer-aware controls.

UI-03: disclosure, filter, and empty/error states on the same surfaces.

```text
Integrations >             children hidden; no navigation on toggle
Integrations v             no configured providers
  Integration settings     actual setup destination

TASKS [Review v] [filter *] saved or draft filter is active
TASKS [All tasks v] [filter] no explicit filter clauses
  No matching tasks        selector and filters stay usable
  Could not load tasks [Retry]  tool navigation still works
```

Control order, hierarchy, single scroll ownership, and state distinctions are
requirements. Spacing, sample labels/numbers, and ASCII glyphs are illustrative.
All product copy comes from localization catalogs. UI-01 covers 001.1-001.6 and
002.1-002.6; UI-02 covers 003.1-003.6; UI-03 covers 001.3, 002.2, 002.5, and 003.6.

## Tests

Planned new/extended behavioral cases:

| Evidence | Acceptance criteria |
| --- | --- |
| `app-sidebar-new-task-item.test.tsx`: primary launch retains regular/Office/Improve routing and shortcut behavior | 001.1, 001.4 |
| `app-sidebar-primary-nav.test.tsx`, `app-sidebar-section.test.tsx`, `sections/integrations-section.test.tsx`: link versus disclosure, setup, current route | 001.2, 001.3 |
| `app-sidebar-footer.test.tsx`, `app-sidebar.test.tsx`: footer/account gates, saved layout, collapse | 001.5, 001.6 |
| Go `internal/user/models/sidebar_layouts_test.go`: `TestDefaultSidebarLayoutPrimaryActionOrder`; service sidebar layout regressions | 001.1, 001.6 |
| New `task/sidebar-filter/sidebar-filter-indicators.test.tsx`: saved-filter cue, draft-only edit, clear/reload behavior | 002.2 |
| New `task-switcher-group.test.tsx`: state versus repository grouping, counts, semantic expanded state | 002.3 |
| Existing `task-item*.test.tsx` compact/trailing/metadata tests plus `task-switcher-render-stability.test.tsx` | 002.4-002.6 |
| `app-nav-sheet.test.tsx`, `mobile-integrations-section.test.tsx`, `session-task-switcher-sheet.test.tsx`: default/saved ordering, launch once, plus fallback, controller survival | 003.1, 003.2, 003.4, 003.6 |

Numbers abbreviate `AC-UI-NAV-HIERARCHY-`. Each work order lists the full IDs.
Add meaningful assertions before implementation; do not add snapshots that merely
mirror classes. Run targeted tests, not a full local verification suite.

## E2E tests

| Scenario and planned file | Project | Criteria |
| --- | --- | --- |
| New `layout/navigation-hierarchy.spec.ts`: primary action, disclosure independence, indentation, default/saved layout, footer, workspace/rail | chromium | 001.1-001.6 |
| New `task/sidebar-task-panel-hierarchy.spec.ts`: filter save/reload, group collapse/count/status, compact geometry, missing data, long titles | chromium | 002.1-002.6 |
| New `task/mobile-sidebar-task-panel-hierarchy.spec.ts`: task tap/menu containment, filters and state groups | mobile-chrome | 002.1-002.6, 003.5 |
| Update `layout/mobile-menu-hierarchy.spec.ts` and `settings/mobile-sidebar-customization.spec.ts`: new order, plus fallback, focus, rotation, 360/393/767px | mobile-chrome | 003.1, 003.2, 003.4-003.6 |
| New `layout/mobile-navigation-hierarchy.spec.ts`: Home/workbench to GitHub Issues with long Tasks, empty/failed reads, workspace isolation, dark/light and Portuguese | mobile-chrome | 001.3, 003.1-003.6 |
| Existing `github/mobile-github-sidebar.spec.ts`, `layout/mobile-unified-navigation.spec.ts`, `layout/mobile-navigation-tasks.spec.ts`, `settings/sidebar-customization.spec.ts` | owning project | provider path and retained behavior |

Run managed production-build E2E commands as written in each work order. New UI
tests prove behavioral RED before production changes; distinguish selector
scaffolding from a real failing assertion. Do not overlap full suites or override
the runner's resource/worker budget.

## Work orders

- [x] [Task 01: Desktop actions and navigation groups](task-01-desktop-navigation.md)
- [x] [Task 02: Contextual task panel and compact rows](task-02-task-panel.md)
- [x] [Task 03: Phone navigation and issue access](task-03-phone-navigation.md)
- [x] [Task 04: Launch the seeded comparison](task-04-seeded-comparison.md)

Dependency order: 01 -> 02 -> 03 -> 04. No subagents are authorized.
Bootstrap once with `(cd apps && pnpm install --frozen-lockfile)` before the first
pnpm command in a fresh worktree. Implementation activated the pinned Node 24, pnpm 9.15.9, and Go toolchain using
`scripts/bootstrap-dev-env --without-os-packages --without-hooks --with-e2e`.
Commands in this shell use `/home/jcfs/.local/bin/mise exec --`; workspace
dependencies and browser runtimes are installed.

## Verification results

Design package validated on 2026-09-28:

- `python3 scripts/list-docs.py validate`: passed, 321 decisions and 1224 specs.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check -- docs/specs docs/plans`: passed. Status confirms all seven
  new package files are present and unstaged/uncommitted.
- `.github/scripts/pr-docs.cjs::validateCoverage`: dry-run preflight against the
  planned sidebar entry point reports `covered`, four work orders, zero errors.
  Run with the installed Playwright driver's Node because `node` is absent from
  PATH; no application runtime was installed or launched for this check.
- Local Markdown link audit passed. All 41 existing paths in work-order test/lint
  commands exist; the six proposed test paths are explicitly new.

The preceding results describe the design-only checkpoint. The user subsequently
authorized full implementation; current application results are recorded in each
work order.

## Implementation delivery (2026-09-28)

All four work orders are complete. The seeded comparison remains running at
`http://127.0.0.1:37497/tasks?workspaceId=6f4eb8c7-75f5-4ece-9187-2f8065a86836`.
See [Task 04 results](task-04-seeded-comparison.md#results) for screenshots,
provenance, isolation paths, verification, and the exact stop command.

Targeted Go and frontend unit checks passed. Browser coverage passed for default
and saved desktop navigation, sidebar filtering/layout/row metadata, phone menu
and task actions, provider issue access, hidden-action fallback, localization,
both themes, and 360/393/767/768px boundaries. Type checking, changed-file lint,
formatting, localization completeness/ratchet, documentation validation, spec
lint, and `git diff --check` passed. The scope did not require a full-repository
verification audit. Changes remain uncommitted for review.

## Risks

- Moving phone tools above Tasks deliberately replaces the September 19/23
  ordering. Update old order assertions, not the retained capability coverage.
- New Task already has a persistent hidden host in saved layouts; a second
  subscription/dialog host could launch twice or lose drafts during rotation.
- State-group identity comes from effective task trees and paginated responses;
  the first row or translated label is not an authoritative group state.
- Global `TaskItem` styles affect nested tasks and multiple sidebar consumers;
  keep changes compact and test the trailing/action hitboxes.
- Preview URLs must refer to this implementation and be reachable through the
  available development port exposure. Report a reachability blocker rather
  than presenting an unreachable URL as a working comparison.

## PR review and CI remediation (2026-09-30)

This entry supersedes the implementation checkpoint's uncommitted status.
PR #4063 incorporates the current main branch without conflicts. Remediation
preserves the accepted plain rows, neutral primary action, quiet utilities,
compact grouping, and saved-layout behavior.

All three inline review findings are addressed: exceptional task states retain
their canonical group icons, and desktop section/phone Canvases disclosures
identify their controlled content. The aggregate review's filter cues now expose
accessible image names, phone-order documentation and coverage annotations match
the implementation, and What's New remains available after reading or disabling
notifications. Reopening it displays the latest notes.

The command-selected row now stays visible when hydration grows the list content
during its selection cue. A short-lived resize observer covers both content and
viewport changes and disconnects on cancellation, replacement, or expiry. Browser
measurements proved content growth, rather than a changing viewport height, caused
the clipping. Regression coverage also verifies cancellation cannot recenter an
obsolete selection.

CI test remediation updates assumptions changed by this UI: plugin actions follow
both built-in utilities; plain-row padding is eight pixels; localized phone/touch
time and action slots remain separate; and phone task creation uses the primary
action. Session recovery measures controls while the injected failure remains
active. The agentctl launch-deadline test uses Go's virtual clock and an injected
transport to prove timeout, transfer cancellation, and fresh retry without a real
40ms filesystem scheduling race. Backend runtime behavior is unchanged.

Local validation passed on the reconciled tree:

- 506 focused frontend unit tests across 44 files, including sidebar disclosure,
  state, filter, footer, release-notes, navigation cancellation, and phone cases.
- 20 desktop browser tests across navigation, task hierarchy, scroll preservation,
  workspace plugins, PR trailing actions, and localized time slots.
- 11 phone browser tests across navigation, hierarchy/subtasks, disclosure,
  hidden-action fallback, GitHub/empty task views, session recovery, and Office
  manager reassignment.
- `KANDEV_E2E_CONTAINERS=1 pnpm --dir apps/web e2e:run --host --no-build --project
  containers tests/docker/repository-secrets.spec.ts`: one passed after building
  the required Linux mock helper. The previous blank-terminal CI failure did not
  reproduce on the reconciled tree. The pause/resume cleanup case also passed.
- `go test -race ./internal/agent/runtime/lifecycle -run 'TestAgentctlResolver'
  -count=1`: passed. The deadline regression also passed 20 repeated race runs.
- `GOMAXPROCS=2 GOGC=50 golangci-lint run ./... --new-from-rev=origin/main
  --timeout=5m`: zero issues against the verified base.
- `pnpm --dir apps/web run typecheck`, `pnpm --dir apps/web run i18n:check`,
  changed-file ESLint, production E2E build, harness validation, documentation
  catalog/spec lint, and both staged/unstaged diff checks passed.

Commands use the pinned toolchain through `mise exec --`; Go commands run from
`apps/backend`. Focused browser runs use the managed host runner with one worker
and retries disabled. CI/review results for the pushed remediation commit and
fresh screenshots remain externally pending until verified on GitHub. No merge
is part of this remediation request.

### Second CI pass and advancing base

The next CI pass exposed three stale browser assumptions: Office creation matched
the hidden desktop primary action, promoted phone canvases required expanding
their tool group, and a checkout without tags generated empty development release
notes. Office coverage now scopes creation to the page's main region; canvas
coverage expands and verifies the disclosure before using its child destination.
The E2E-only build selects a fixed non-development version and the latest
changelog entry, making release-note availability independent of runner tags.
Production version resolution is unchanged.

The advancing main branch's fixture and task-interaction fixes were integrated.
Its stable task-row geometry wait and this PR's bounded content/viewport observer
are both retained; phone recovery keeps the upstream session-scoped failure and
teardown handling. The two overlaps were resolved with those invariants preserved.
The earlier module-proxy download failure cleared after one targeted backend
rerun, including the backend aggregate.

On the reconciled tree, 81 focused units across 10 files and type checking passed.
Four phone browser cases passed: promoted canvas navigation, Office creation,
session recovery, and stream-overload isolation. Release-note data generation
reports the latest changelog fallback for the fixed E2E version. The desktop
release-note, delayed-reveal, and ended-session terminal cases are validated
before the second remediation push. The prior 506-test audit and screenshots
remain historical evidence until the new commit's checks and captures complete.

The combined reveal wait now invalidates an unfinished scroll when its content
or viewport dimensions change before the row first enters view. A regression
fails without that reset; the fixed navigation suite passes 20 cases. Desktop
browser checks pass for readable release notes, the ended-session terminal, and
the delayed settings-blocked reveal after a fresh production E2E build. The
earlier clipping failure is preserved as regression evidence. Changed-file
ESLint and staged/unstaged diff checks pass, with no unresolved merge entries.

## Requested refinement (2026-09-28)

After reviewing the seeded preview, the user explicitly requested a larger New
Task action, discoverable Quick Chat and Terminal, and one compact footer row.
The revised design uses a 44px primary action, equal-width labelled 28px secondary
actions, and Settings/theme/More Actions in one row. The authenticated account is
an avatar with identity in its menu. All secondary utilities and eligible footer
plugin destinations are labelled entries in the shared menu, preserving handlers
and the unseen-release indicator. Phone keeps its existing labelled utilities and
44px controls. The same isolated instance will be refreshed without reseeding.

Refinement completed. Three focused component suites passed (76 tests), covering
creation routes/shortcuts, utility navigation, release notes, account/logout,
settings guards, plugin registration order, and phone navigation. Fifteen distinct
desktop browser scenarios and twelve phone scenarios passed: 44px primary action,
equal labelled quick actions, one-row footer, plugin utilities, support dialogs,
terminal lifecycle, keyboard focus restoration, touch menus/theme control,
localized phone quick actions, issue access, and draft continuity. Type checking,
changed-file lint/formatting, six-language localization checks, documentation
validation, spec lint, and whitespace checks passed.

The same seeded instance was refreshed without resetting tasks, sessions, saved
views, or the grouping changed during user review. API read-back still confirms
five tasks, four sessions, two issues, two PRs, and one paused automation. The
browser checks passed in dark/light at 1280px and on 393/767px phones, plus
Portuguese, without page errors. `screenshots-iteration-1` preserves the previous
captures; `screenshots` contains the revision and its open utilities menu.
The original preview URL and stop command remain valid.


## Quiet action hierarchy (2026-09-28)

The user requested autonomous refinement of the saturated primary button and
oversized secondary actions. Visual comparison of neutral filled and outlined
variants selected a faint neutral surface and quiet border for New Task, retaining
its 44px target. Quick Chat and Terminal now use content-width muted ghost actions
at 28px on desktop. Phone retains native 44px targets and labelled ghost actions.
The existing one-row footer is unchanged. Comparison included neutral fill and
plain outline in both themes; the selected intermediate fill provides enough
creation affordance without a saturated block. Labels keep at least 4.74:1
contrast in light mode and 5.00:1 in dark mode, measured from rendered colors.
New Task retains 44px height; desktop utilities are 28px and phone utilities 44px.

Validation passed: 55 component tests, four desktop hierarchy/touch scenarios,
twelve phone navigation/translation/draft scenarios, and three Quick Chat focus
and terminal lifecycle scenarios. Type checking, changed-file lint/formatting,
localization completeness and ratchet, and documentation validation passed.
Actual final captures include dark/light desktop and phone, Portuguese desktop,
and keyboard focus. Live tasks and preferences were preserved. Evidence lives in
`/tmp/kandev-navigation-compare-z54_iuek/quiet-final` and `quiet-check.json`.
The seeded preview remains running at its existing URL.

## Automations destination placement (2026-09-28)

The user requested moving Open automations into its group, matching Integration
settings. The desktop disclosure now ends with a labelled, indented destination
and has no separate header shortcut. The phone disclosure owns the same final
link for both default and saved-layout children, with a 44px target and existing
menu dismissal. Fetch gating, routes, counts, and setup links are preserved.

Completed with four meaningful failing component assertions before the change,
then 23 passing component tests, four desktop browser scenarios, and one phone
browser scenario. Type checking, changed-file lint, documentation validation,
specification lint, and whitespace checks passed. The existing seeded instance
was refreshed without reseeding; API read-back confirmed 11 tasks, seven sessions,
two issues, two PRs, and one paused automation. Direct preview checks passed for
desktop placement/current route and phone routing/dismissal at 360/393/767px.
Screenshots and browser evidence are in
`/tmp/kandev-navigation-compare-z54_iuek/automations-link/`.

## Workflow group hierarchy (2026-09-28)

The user found workflow-step headings visually flat. Named task groups now use a
leading disclosure chevron, semibold heading, count, and spacing between groups.
A subtle guide connects the indented task body to its header; existing subtask
indentation remains a deeper level with a clearer decorative connector. Header
and body use unique per-instance accessibility IDs. Ungrouped rows keep their
existing horizontal allocation. Grouping, counts, continuation, selection,
pagination, and drag ownership remain unchanged.

The phone composition retains the AppNavSheet/task-picker inline list and its
single scroll owner. Group disclosures and row actions retain 44px touch targets;
the same task model drives all layouts. The existing saved workflow-step view in
the preview was preserved.

Validation: the workflow browser regression first failed on the flat 8px inset,
then passed with the intended hierarchy. All 36 focused component tests passed,
including render stability, nested tasks, and drag behavior. Six browser
scenarios passed: desktop state/workflow groups, phone state/workflow groups,
drag reparenting, and sibling reorder. Type checking, changed-file lint,
formatting, documentation/specification checks, and whitespace checks passed.
Rendered checks covered dark/light desktop, phone, a narrow fine-pointer window,
and a coarse-pointer tablet; phone E2E covered 360/393/767px. The seeded instance
was refreshed without reseeding, and saved view read-back remained identical.
Screenshots and live verification are in
`/tmp/kandev-navigation-compare-z54_iuek/workflow-groups/`.

## Quieter grouping without vertical rules (2026-09-28)

The user preferred the reference's lighter hierarchy. Removed the decorative
left rules from named task groups and expanded built-in navigation. Indentation,
stronger headings, spacing, counts, disclosure accessibility, and deeper subtask
offsets remain. Existing behavioral coverage retains those guarantees without
requiring a decorative border. Six desktop and two phone E2E scenarios passed,
along with changed-file lint/formatting, docs/spec validation, and whitespace
checks. Rendered desktop and phone checks confirmed the rules are absent.

The original disposable `/tmp` instance and its data no longer existed. A fresh
isolated Compass Studio comparison now runs at
`http://127.0.0.1:37497/tasks?workspaceId=31bd354e-3f7a-4b9f-816f-a5360eede356`.
It has five parent tasks, six subtasks, two prepared sessions with four real
changed files, two mock issues, two linked mock PRs, and a paused automation.
This is new demo data, not a restoration of the expired instance's edits.
Both prepared tasks open their populated Changes panels.

Artifacts, source/build provenance, ownership, API counts, and screenshots are
under `/home/jcfs/.local/share/kandev-demos/navigation-hierarchy-5jnqf8pa`.
Stop only this preview with
`python3 /home/jcfs/.local/share/kandev-demos/navigation-hierarchy-5jnqf8pa/stop.py`.

## Plain task rows (2026-09-29)

The user preferred plain rows over task cards. Removed persistent borders and
backgrounds from parent tasks and subtasks. Quiet hover and active-selection
fills, keyboard-focus rings, and multiselection feedback remain. Workflow
headings, indentation, spacing, metadata, and actions retain their behavior.
The shared AppNavSheet/task-picker rows keep the existing phone composition,
scroll owner, and 44px action targets. Navigation and task-focus specifications
now describe the same borderless treatment.

Validation: 66 focused component tests and eight desktop/phone browser scenarios
passed, covering active task selection, Home deselection, status/workflow group
collapse, saved filters, nested navigation, touch targets, and containment.
Changed-file lint/formatting, docs validation, specification lint, and whitespace
checks passed. Live rendered checks covered dark/light desktop and phone,
including transparent idle rows, active fills, hover, keyboard focus, and
unchanged saved views/layouts/task preferences.

Only frontend assets were refreshed in the owned comparison at port 37497.
Existing seed data and user edits were preserved, including the collapsed group
and draft view configuration. Screenshots and live-check evidence are under
`/home/jcfs/.local/share/kandev-demos/navigation-hierarchy-5jnqf8pa/plain-rows/`.

## Tighter task-group spacing (2026-09-29)

Reduced separation between named groups from 12px to 4px and desktop disclosure
headers from 32px to 28px. Phone and coarse-pointer headers retain 44px targets.
Plain rows, indentation, group collapse, and the shared scroll owner are unchanged.
The user's separate navigation-order question was resolved as their own workflow
configuration; no ordering or live-instance settings were changed.

Two desktop and two phone browser scenarios passed, covering both themes,
360/393/767px phone widths, hierarchy, collapse, navigation, and touch targets.
Live dark/light desktop and phone checks measured the new group gap and header
heights and confirmed saved preferences remained identical. Changed-file lint
and formatting, docs/spec validation, and whitespace checks passed. The owned
comparison at port 37497 has refreshed frontend assets and retains its data.
Evidence: `/home/jcfs/.local/share/kandev-demos/navigation-hierarchy-5jnqf8pa/group-spacing/`.

## Anchored quick-action utilities (2026-09-29)

The user found the loose Quick Chat and Terminal labels visually lost. They now
form a compact utility bar using the existing ButtonGroup and inset separator.
The shared quiet boundary aligns to New Task on desktop, with equal-width
targets, clearer secondary text, transparent button fills, and existing activity
cues. Desktop actions remain 28px tall; phone/coarse-pointer targets remain 44px.
Phone menu composition, launcher handoff, focus return, collapsed-rail access,
and plugin workspace-action placement are preserved.

All 55 focused component tests and eight browser scenarios passed. Browser
coverage includes desktop navigation, 768px coarse-pointer and 767px fine-pointer
controls, terminal creation/reuse/switching/closure, phone quick actions from Home
and a task, and phone terminal containment. Type checking, changed-file lint,
formatting, documentation/specification validation, and whitespace checks passed.
Live desktop/phone captures in both themes confirmed equal target widths, shared
grouping, hover/keyboard feedback, containment, and preserved saved preferences.

The owned comparison at port 37497 has updated frontend assets and retains its
seed data. Evidence and screenshots:
`/home/jcfs/.local/share/kandev-demos/navigation-hierarchy-5jnqf8pa/utility-bar/`.

## Remove duplicate quick-action tooltips (2026-09-29)

Removed the hover/focus tooltip wrappers and their local pointer state from the
labelled Quick Chat and Terminal buttons. Visible labels, accessible activity
names, activity indicators, launchers, hover fill, and keyboard focus remain.
Collapsed icon-only controls retain their identifying tooltips. Phone controls
keep the existing utility bar, drawer composition, and touch targets.

Both replacement hover assertions failed before the change; all 28 component
tests passed afterward. The frontend build, lint, formatting, docs/spec validation,
and whitespace checks passed. Live desktop/phone checks in both themes confirmed
no duplicate tooltip, retained feedback, geometry, and saved preferences.

The owned preview had stopped. Its existing database was backed up, and the same
isolated instance was restarted at port 37497 with updated frontend assets and no
reseeding. Evidence: `/home/jcfs/.local/share/kandev-demos/navigation-hierarchy-5jnqf8pa/tooltips/`.

## Sidebar header typography (2026-09-29)

Increased the expanded Kandev brand to 18px bold and reduced its workspace
combobox text to 12px. The header retains its 40px height, centered alignment,
truncation, and collapse control. The phone workspace picker retains 14px text
and a 44px target; dropdown options retain their existing typography.

All 33 header and workspace-picker component tests passed. The frontend build,
changed-file lint and formatting, docs/spec validation, and whitespace checks
passed. Live desktop/phone checks in both themes confirmed type sizes, geometry,
containment, menu opening, Escape dismissal, focus return, and unchanged saved
preferences, with no page errors.

The owned comparison at port 37497 has refreshed frontend assets and retains its
existing data. Evidence and screenshots:
`/home/jcfs/.local/share/kandev-demos/navigation-hierarchy-5jnqf8pa/header-type/`.

## Centered New Task without a shortcut hint (2026-09-29)

Centered the plus icon and New Task label together and removed the displayed
keyboard hint, following the user's correction. The shared button uses the
existing flex alignment and no longer subscribes to shortcut settings. Keyboard
bindings and task-creation handlers are unchanged. Phone navigation shares the
centered action and retains its 44px target.

All 55 sidebar-action and phone-navigation component tests passed. The frontend
build, changed-file lint and formatting, docs/spec validation, and whitespace
checks passed. Seven live browser scenarios covered both themes, desktop and
phone, and fine/coarse pointers around the 768px boundary. They confirmed centered
icon/label geometry, no hint, 44px targets, containment, keyboard/button launch,
preserved saved preferences, and no page errors.

The owned comparison at port 37497 has refreshed frontend assets and unchanged
data. Evidence and screenshots:
`/home/jcfs/.local/share/kandev-demos/navigation-hierarchy-5jnqf8pa/new-task-center/`.

## PR publication verification (2026-09-29)

The user requested committing, pushing, and opening a PR for the completed
implementation. Final verification passed 481 focused frontend tests across 41
files, six desktop browser scenarios, five phone browser scenarios, and the Go
sidebar-layout model/service tests. Type checking, the production frontend build,
localization checks, harness validation, specification validation, and whitespace
checks passed. Browser coverage includes saved navigation behavior, disclosure
destinations, footer actions, status/workflow grouping, nested task collapse,
filters, phone issue access, draft continuity, and the 767/768px boundary.

The latest New Task correction centers the icon/label with no visible keyboard
hint; it supersedes the initial preview above. Tasks use plain rows, subtle active
states, indentation without vertical guides, and compact group spacing. Footer
utilities and plugins share the labelled menu, while settings/theme/account remain
in one row. Existing saved layouts and the owned comparison data remain intact.

Integrated main at `ccaa7c0a8d80ef4f2f089b1f416bd1230417da0a` before publication.
The seven locale conflicts were independent additions: retained the upstream
query/filter validation copy and this change's active-filter label in every
locale, with duplicate-key and localization validation.


## Third CI remediation (2026-09-30)

The second published remediation completed CI with 58 passed checks and three
failed checks: browser shard 6 and its two dependent E2E gates. Its PostgreSQL 16
timeout passed on the single failed-job retry. All other backend, frontend,
browser, and container jobs passed; no unresolved review threads remained.

The nesting scenario reproduced a hover against the rounded row corner, where
the context-menu wrapper receives the pointer. It now hovers the task title
before opening actions, preserving the real nest and un-nest assertions. The
external PR scenario reproduced a leaked-history failure after adding twelve
commits to the shared worker repository. It now owns an isolated repository and
offline origin using existing fixture helpers. Both scenarios and the polluted
repository reproduction passed without Playwright retries (three checks). The
temporary reproduction file was removed. No product behavior or timeout changed.

Main advanced without file conflicts; validation of the final combined result
and exact-head external CI will follow publication of this test-only fixup.


## Fourth CI remediation (2026-09-30)

The third fixup cleared browser shard 6, including the nesting and isolated PR
scenarios. Shard 3 exposed a matching rounded-corner hover in the diff-summary
scenario and a shared seeded profile referenced by soft-deleted dynamic profiles.
Both failures reproduced locally. The diff-summary and remaining PR-summary
corner hovers now target the actual title. The settings scenario creates its own
Mock profile, exercises disabling and re-enabling, and removes that owned profile.

Four targeted browser checks passed without retries, including a reproduction
with a deleted dynamic profile referencing the shared fixture. The temporary
reproduction was removed. No product behavior, protection policy, timeout, or
assertion was weakened. External checks will rerun after publication.

## Latest-main conflict resolution (2026-09-30)

Merged authoritative main at `61d791ebd7425dd93f99e84e56a0a8803b886ae7`
after the user reported new conflicts. The profile-navigation test conflicted
because main selected the shared seeded Mock profile. Retained explicit Mock
agent selection and this branch's test-owned profile with deterministic deletion,
which avoids disabling a seed referenced by other scenarios. The automatically
merged PR-detection test retains its isolated repository and offline origin;
resetting the unrelated shared repository is unnecessary for that fixture.

Both fixture corrections preserve main's cleanup intent and the existing
assertions. The incoming changelist measurement, stall-notice recovery,
dispatch cancellation, and walkthrough fixes remain intact.

Verification on the combined tree passed 253 frontend tests across 21 files,
web typecheck and lint, affected lifecycle/task-service/SQLite Go race tests,
and documentation, specification, architecture, and harness validation. A fresh
managed production build passed nine desktop browser scenarios without retries.
Its unchanged artifacts passed four phone scenarios without retries, including
changelist spacing, tree touch controls, status groups in both themes, and
workflow hierarchy. Fresh exact-head CI will run after the merge is published.


## Conflict reconciliation (2026-10-05)

- Integrated main `def028f60c73bb8dfd43bed84c5aef066dc24382` into the reviewed
  navigation branch. The relocated lifecycle deadline regression keeps one
  deterministic-clock test, main's 10-second transfer and 1-second launch
  deadlines, and the cancellation/progress/fresh-retry assertions.
- Preserved incoming pending-removal guards, the Quick Chat opening composer,
  mobile terminal controls, PR approval badges, and exported plugin SDK types
  alongside the approved navigation hierarchy, plain task rows, utility order,
  and footer placement. No production interaction contract changed in resolution.
- Added `sidebar:filtersActive` to the newly landed Korean catalog. Full i18n
  validation passes with all real locales and pseudo synchronized.
- Focused frontend validation: 227 tests in 19 files, typecheck, and full lint
  passed. Targeted lifecycle race tests, documentation/spec lint, harness
  validation and its tests, and JSON syntax/duplicate-key validation passed.
- Fresh managed desktop browser validation: 13 tests passed with retries disabled,
  covering launch/focus, navigation, task groups, PR summaries/approval badges,
  tablet sizing, and terminal reuse. The unchanged fresh build also passed 11
  phone tests with retries disabled, including prompt delivery, draft continuity,
  nested navigation, PR drawers, touch targets, and terminal keyboard behavior.
  Final delivery evidence is recorded in the current Kandev task plan.
- Resolution changes test composition and the Korean filter label only. The
  existing English navigation screenshots remain representative; no affected
  screenshot viewport requires replacement.
