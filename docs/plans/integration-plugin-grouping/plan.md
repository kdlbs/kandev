---
created: 2026-10-25
status: implemented
requirements:
  - REQ-UI-SIDEBAR-CUSTOMIZATION-001
  - REQ-UI-SIDEBAR-CUSTOMIZATION-002
  - REQ-UI-SIDEBAR-CUSTOMIZATION-004
  - REQ-UI-SIDEBAR-CUSTOMIZATION-005
  - REQ-UI-SIDEBAR-CUSTOMIZATION-006
system_design:
  - ../../specs/ui/system-design/sidebar-customization.md
legacy_specs: []
---

# Restore integration plugin grouping

## Overview

Restore plugin integration links inside Integrations while retaining personal
visibility and sibling order. First implement the shared projection, navigation,
and editing behavior with focused tests. Then prove the user flows with desktop
and phone E2E and update user documentation.

UI owns the reusable layout composition contract. The existing plugin API owns
registration section values and destination identities. This repair adds explicit
customization acceptance criteria where the prior requirements were incomplete.
It reconciles the previous phone design's standalone-plugin instruction with the
plugin API's documented grouping. It does not change the plugin API.

## Root cause and reproduction

`useHasSavedSidebarLayout` returns true for any active workspace.
`materializeSidebarPluginNodes` inserts integration plugin nodes after the
Integrations node. `SidebarLayoutNavigation` excludes plugins from that section
and renders the flat nodes independently. Phone saved navigation does likewise.
`layout-projection.test.ts` currently expects a separate Slack node after
Integrations, so the existing test does not catch the rendered grouping defect.

Install the existing E2E fixture, whose `e2e-integration` registration targets
`integrations`. Open a regular workspace with default or saved layout. Its link
appears outside the group. No user customization or YouTrack-specific state is
needed. Source tracing established this path during planning. A subsequent isolated
browser run installed the actual YouTrack v0.2.0 release and reproduced the defect.
See [reproduction evidence](reproduction.md).

## Scope

In scope: original integration-plugin grouping, old/default/reset layouts,
individual visibility, sibling order, Settings preview, direct/keyboard/phone
editing, collapsed desktop launcher, mobile disclosure, and targeted regression
coverage. Preserve custom-group references and current Office plugin access.

Out of scope: provider authentication, plugin releases, SDK changes, backend
schema changes, new feature flags, footer regrouping, unrelated sidebar redesign.

The later user request also authorizes commit, push, PR creation, CI/review remediation, and merge after all gates pass.

## Technical approach

Keep flat version-1 saved nodes and existing CAS writes. Add a small pure helper,
proposed as `lib/sidebar/integration-plugin-layout.ts`, to derive hierarchy and
legal sibling moves from projected nodes and current catalog metadata. Retain the
full flat nodes for editing and serialization. Never infer ownership from a label
or raw plugin item ID. Use the owner's destination ID.

Remove grouped nodes only from the top-level render list. Supply their ordered,
visible destinations to the existing desktop/mobile integration sections.
Preserve manifest defaults for callers without layout input. Apply availability,
parent visibility, and phone omission filters before rendering. A custom shortcut
is an independent reference and must not be removed by this operation.

All editing entry points use the same sibling/top-level scope. Preserve unrelated
node order and unavailable nodes. Scope nested pointer/keyboard handlers so a child
move cannot also move the parent. Keep explicit no-op/invalid moves write-free.
Restore defaults continues through the existing reset flow and creates no second
settings source. Existing global positions of integration links become relative
order inside Integrations. No read or viewport change writes that interpretation.

Preserve Office's standalone plugin access when that mode omits the group.
In regular mode, a hidden group hides its children. For a missing legacy group,
materialize the canonical anchor for display and explicit edits without saving on read. Settings preview
and runtime navigation must agree for both conditions.

The current navigation ADR and ADR 0041 remain authoritative. A new ADR is not
needed because this repair restores grouping within their existing boundaries.
The linked completed sidebar-customization, mobile-saved-navigation,
sidebar-presentation-preferences, and flagged-sidebar-entries packages remain
historical evidence. This package owns the correction and its new results.

## UI preview

These labelled tables replace ASCII sketches to respect the session's explicit
preview-format instruction. Hierarchy and control order are requirements;
provider names illustrate available entries, not a hardcoded provider list.

### UI-01: Desktop sidebar, expanded Integrations

| State | Top-level entries | Integrations children, in order |
| --- | --- | --- |
| Before (source-confirmed) | Integrations, YouTrack | GitHub, GitLab, Integration settings |
| After | Integrations | GitHub, GitLab, YouTrack, Integration settings |
| Folded | Integrations | Hidden until expanded |
| YouTrack hidden | Integrations | GitHub, GitLab, Integration settings |

The existing rail launcher exposes the same child list when the sidebar is
collapsed. The existing navigation region owns scrolling. Header fast actions
keep their current first-party-only policy. No new icon, motion, or sizing rule.

### UI-02: Phone app menu, Integrations disclosure

| State | Menu row | Disclosure contents |
| --- | --- | --- |
| Initial | Integrations, collapsed | None visible |
| Expanded | Integrations, expanded | Provider rows, YouTrack, Integration settings |
| Activation | Tap YouTrack | Open its registered route and close navigation |

Reuse AppNavSheet and MobileIntegrationsSection. Rows retain 44px touch targets,
one drawer scroller, safe-area clearance, and existing focus-return behavior.
No top-level duplicate appears. Settings remains reachable without any providers.

### UI-03: Customization and Settings preview

| Level | Entry | Allowed actions |
| --- | --- | --- |
| Top level | Integrations | Visibility, move among top-level entries |
| Child | YouTrack | Visibility, move among integration-plugin siblings |
| Child | Another integration plugin | Visibility, move among integration-plugin siblings |
| Separate custom group | Explicit YouTrack shortcut | Existing shortcut actions |

Settings preview uses UI-01 grouping. Phone customization keeps its existing
inset drawer and explicit move controls. A hidden parent does not erase the
child's checked state. Controls do not offer cross-group detachment.

UI-01 through UI-03 cover AC-UI-SIDEBAR-CUSTOMIZATION-001.10 through 001.12,
004.2, 005.2 through 005.4, and 006.7.

## Tests

Task 01 uses these regression cases:

- `integration-plugin-layout.test.ts`: groups visible integrations under their
  parent without mutating flat settings (001.10, 001.12). Include two integration
  plugins, a hidden plugin, unavailable registration, main/footer plugins, and
  an explicit custom shortcut in one mixed-state case.
- `integration-plugin-layout.test.ts`: sibling moves preserve unrelated node
  order, hidden choices, revisions, and geometry; invalid moves are no-ops
  (001.11, 001.12, 006.5).
- `sidebar-layout-navigation.test.tsx` (new): renders the registered integration
  once inside Integrations for default and saved layouts (001.10). This is the
  initial RED gate using the real projection and registry, not a mocked final list.
- `app-nav-sheet.test.tsx` and `mobile-integrations-section.test.tsx`: matching
  grouping, omission, folding, route activation, and setup behavior (001.10,
  001.11, 005.2 through 005.4).
- Settings/editor and direct-navigation tests: preview hierarchy, sibling moves,
  parent moves, child context targets, and no accidental parent drag (004.2,
  006.4, 006.5, 006.7). Existing save/reset tests retain their CAS coverage.

## E2E tests

Task 02 adds `plugins/integration-plugin-grouping.spec.ts` (chromium) and
`plugins/mobile-integration-plugin-grouping.spec.ts` (mobile-chrome). Reuse the
packaged fixture's `e2e-integration` registration and real settings persistence.
Verify ancestry and uniqueness, collapse, navigation, visibility and reload,
parent hide/show, explicit custom-group shortcut independence, and reset.
Use unit/component coverage for two-plugin ordering and unavailable entries.
The desktop flow includes the collapsed rail launcher. The phone flow measures
44px targets, containment, no horizontal overflow, and menu closure on activation.
Capture both rendered states for comparison with UI-01 and UI-02.

## Work orders

- [x] [Task 01: Restore grouping and scoped customization](task-01-restore-grouping.md)
- [x] [Task 02: Prove desktop and phone behavior](task-02-prove-navigation.md)

Execution is sequential. Task 02 depends on Task 01. No delegation is authorized.

## Verification results

Design validation on 2026-10-25:

- `python3 scripts/list-docs.py validate`: passed (384 decisions, 1616 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `git diff --check`: passed.
- Local `.github/scripts/pr-docs.cjs` `validateCoverage`: the actual docs-only
  change is exempt. A preflight including the planned sidebar runtime edit is
  covered by both work orders with no reference errors.

Task 01 is complete: 112 focused tests in 16 suites, typecheck, ESLint, and i18n passed.
Actual YouTrack desktop and touch-phone placement and activation passed.
Task 02 is complete: 12 desktop (including tablet) and 11 phone E2E tests passed against the
same fresh production build. Public documentation and checks passed. All work
orders are done. PR CI/review and merge remain external delivery gates.

Final local E2E logs: `/tmp/kandev-run.e2e.MSPoa0sK.log` (phone) and
`/tmp/kandev-run.e2e.lXHOuXfD.log` (desktop). Updated base catalog validation:
403 decisions and 1738 specifications. Public docs: 63 validator tests passed; 54 pages validated.
Actual plugin captures were inspected after finite UI animations settled.
Pre-fix browser reproduction with actual YouTrack v0.2.0: confirmed on 2026-10-25,
including reload. See [evidence](reproduction.md).
Each work order contains exact implementation commands and owns its results.

## Risks

- Turning plugin inclusion back on without removing standalone rendering creates
  duplicates and ignores saved visibility. Test ancestry and count together.
- Flat global indexes can cause child moves to affect unrelated tools. Route
  all move entry points through the shared scope helper.
- A hidden parent differs from a mode-suppressed parent. Preserve Office access
  explicitly without allowing hidden regular-workspace children to escape.
- New labels require all shipped locales. Reuse current strings where possible.
- Existing integration-plugin global placement intentionally becomes grouped
  placement. Stored identities and individual choices remain recoverable.

## Review remediation

Review found two layout cases: an inferred Integrations parent was
absent from explicit edit state, and wider listing menus received the phone-only
collapsed disclosure. The parent now materializes for both rendering and editing
without writing on read. The shared resource component passes the phone
presentation choice through instead of applying it to every consumer.
Regression tests reproduce both defects before correction. The desktop browser
flow additionally edits a layout that originally lacks the parent, and a new
coarse-pointer tablet flow opens the listing menu with expanded plugin links.
Fresh published-plugin screenshots passed on desktop, phone, and tablet. The touch/pen pointer handler now preserves propagation to the context-menu trigger. Portaled-menu focus no longer invokes sidebar expansion or starts competing layout writes. The Office singleton check passes without a code change because builtin eligibility removes its Integrations parent. The shared draft materialization also supplies the legacy parent to Settings, so its controls remain editable. Focused validation passed: 111 tests in 16 suites, typecheck, changed-file ESLint, i18n, docs, and specification checks. Final browser regression passed: 12 desktop tests (including tablet navigation) and 11 phone tests against the same fresh production build.

### UI-04: Tablet listing menu, expanded Integrations

| Level | Content | Behavior |
| --- | --- | --- |
| Heading | Integrations | Expanded, without a phone disclosure toggle |
| Link | YouTrack | Visible on opening the menu, opens its registered route |
| Link | Integration settings | Retains the existing setup destination |

Reuse the wider menu surface and its scroller. Touch controls keep their existing
44px sizing. This view preserves the wider-consumer contract in the frontend
guide. See [combined plan](plan.md#ui-04-tablet-listing-menu-expanded-integrations).

Final review-validation logs, 2026-10-26:

- Desktop/tablet: `/tmp/youtrack-e2e-logs/kandev-run.e2e.r9Mj2pnK.log`, 12 passed, retries disabled.
- Phone: `/tmp/youtrack-e2e-logs/kandev-run.e2e.xj4A8ZlJ.log`, 11 passed, retries disabled.
- Focused reproductions of four initial CI failures: `/tmp/youtrack-e2e-logs/kandev-run.e2e.a5BE0TWh.log`, 4 passed, retries disabled. These unchanged tests cover Jira auth health, cancellation across task switches, jump-to-latest, and the Quick Chat goal disclosure. Fresh PR CI remains required.

The final source also excludes portaled menu focus from sidebar expansion, with a behavioral regression and a positive control for clipped-row focus. The final browser runs above include the legacy-parent hide, save, reload, and restore flow. Fresh published YouTrack captures cover desktop, phone, and tablet.

## Final Claude observations

The review at head `8c21847710c` confirmed the earlier fixes. The possible Settings dirty-draft issue was not reproduced: the actual draft and drag/action hooks keep both directions of a rejected cross-group move clean. A valid integration sibling move marks the draft dirty. Three new hook regressions cover those outcomes. The full focused command now passes 111 tests in 16 suites.

A missing legacy parent intentionally appends to the flat list, preserving the prior fallback position and existing node order. The design and projection assertion now state that position. Settings move controls disable boundary actions and clamp their positions; sibling lists always include the rendered node, so those controls cannot request an out-of-range position.

Removed the redundant inner editor key and used `cn()` for the existing phone-row classes. The rendered class values and keyed row identity are unchanged. Typecheck and focused tests passed. Fresh desktop/tablet and phone browser checks passed before this follow-up is pushed.

The final follow-up incorporates base `e5c38184cf9c191075dc88c1ffad28c636858bf5` by rebasing the existing branch. Base and PR paths do not overlap, and the update applied without conflicts. The combined source passed 111 focused tests and typecheck. The fresh combined-source browser build passed 12 desktop/tablet cases and 11 phone cases with retries disabled. Logs: `/tmp/youtrack-e2e-logs/kandev-run.e2e.2rP49zaU.log` and `/tmp/youtrack-e2e-logs/kandev-run.e2e.zsiyyYKR.log`.

The previous-head CI locale setup failure (`pseudo-coverage.spec.ts`, Office workspace org) passed three consecutive focused repetitions on the combined source, with retries disabled. Log: `/tmp/youtrack-e2e-logs/kandev-run.e2e.gLW8excu.log`. The failed CI artifact's retry trace also showed a successful pseudo-cookie request; the first attempt had no retained trace. No locale source change was made. Fresh exact-head CI is still required; this focused pass does not by itself classify the original failure as resolved.

## Collision-safe legacy parent

Claude's review at `a07ef4e6edb` raised a possible ID collision. Inspection confirmed that valid custom sections may use `integrations`. The materializer now follows the existing Coordinator suffix convention to choose the first unused node ID, preserving custom IDs and destination ownership. A behavioral regression failed with `duplicate_id` before the fix and passed afterward. It covers multiple occupied suffixes, stable repeated materialization, editable parent visibility, and unchanged custom sections.

The desktop legacy-layout flow now seeds a custom section with that colliding ID and an explicit integration shortcut. It hides and restores the distinct Integrations parent through real settings writes and reload, checks unique persisted IDs, and keeps the custom shortcut visible. The fixture explicitly expands navigation so this case tests identity and persistence. The normal desktop, phone, and geometry cases remain covered separately. Final collision validation passed: 112 focused tests in 16 suites, typecheck, zero-warning changed-file ESLint, 12 desktop/tablet browser cases, and 11 phone cases. The browser cases used retries disabled and the same fresh frontend build. Logs: `/tmp/youtrack-e2e-logs/kandev-run.e2e.0VfWh9CF.log` and `/tmp/youtrack-e2e-logs/kandev-run.e2e.LLasneTS.log`.


## CI fixture remediation

The user authorized full PR cleanup and merge. Four browser-shard failures on the
reviewed source exposed independent test setup and readiness races. The E2E-only
follow-up uses a valid idle session and gated recovery for the mobile Retry tap,
the saved-query button after drawer animation, an image-response wait after
reload, and the existing return-to-Files flow after upload activates Changes.
All behavioral assertions remain, with native input, no added retries, and no
longer timeouts. The affected full files passed nine mobile and nine desktop
tests with retries disabled; the mobile failure cases also passed six stress
repetitions. Details and exact commands are in Task 02. The product grouping
proof remains valid because these changes do not alter production source. Fresh
CI and review of the remediation commit remain external delivery gates.


The isolated failed-jobs rerun cleared the runner-settings and cancellation
shards. The repeated MCP explorer failure was reproduced locally: its legacy
profile write no longer configures delivery after startup migration completes.
The E2E fixture now exercises that migrated state and uses the current MCP
catalog/selection API, retaining its delivery assertion and cleanup. The corrected
case passed three repetitions without retries, and the full MCP file passed all
three tests without retries. See Task 02 for RED/GREEN evidence.
No production source changed. Fresh CI and review remain required.
