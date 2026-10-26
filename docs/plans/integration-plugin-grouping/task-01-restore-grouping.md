---
id: "01-restore-grouping"
title: "Restore grouping and scoped customization"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-CUSTOMIZATION-001
  - REQ-UI-SIDEBAR-CUSTOMIZATION-002
  - REQ-UI-SIDEBAR-CUSTOMIZATION-004
  - REQ-UI-SIDEBAR-CUSTOMIZATION-005
  - REQ-UI-SIDEBAR-CUSTOMIZATION-006
system_design:
  - ../../specs/ui/system-design/sidebar-customization.md
acceptance_criteria:
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.10
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.11
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.12
  - AC-UI-SIDEBAR-CUSTOMIZATION-002.1
  - AC-UI-SIDEBAR-CUSTOMIZATION-004.2
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.2
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.3
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.4
  - AC-UI-SIDEBAR-CUSTOMIZATION-006.4
  - AC-UI-SIDEBAR-CUSTOMIZATION-006.5
  - AC-UI-SIDEBAR-CUSTOMIZATION-006.7
---

# Task 01: Restore grouping and scoped customization

## Summary

Restore the integration hierarchy across desktop, phone, and layout editors.
Keep saved flat nodes while deriving one effective grouping and legal move scope.

## In scope

Shared grouping and movement, integration section inputs, duplicate suppression,
hidden/unavailable children, parent visibility, mode-specific fallback, Settings
preview, pointer/keyboard/phone customization, and relevant unit/component tests.
Update the scoped frontend guidance that currently instructs plugin exclusion.

## Out of scope

Storage migration, provider changes, plugin packages, new flags, and footer layout.
Browser end-to-end proof and public docs belong to Task 02.

## Acceptance

1. Regular desktop and phone navigation render each eligible original integration
   link inside Integrations, once, for default/saved/reset layouts. Main/footer
   behavior, Office access, and custom-group references retain their behavior.
2. Existing visibility, sibling order, unrelated ordering, revisions, and geometry
   survive. Parent hiding/folding suppresses children without changing their choices.
   No rendering or invalid move saves a layout.
3. Settings preview and all edit controls match the runtime hierarchy. Child moves
   stay among integration siblings; parent moves stay among top-level entries.
   Nested click, drag, and keyboard events cannot execute both parent and child actions.

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

Full context: [plan previews](plan.md#ui-preview).

## TDD sequence

First add the default-layout component regression named in the plan. Observe
failure because the integration link is outside the group. Add the mixed-state
pure-helper cases and implement only the behavior they require. Do not accept a
missing import or selector as the behavioral RED. Finish with the focused suite.

## Verification

From repository root. Install once if this worktree lacks dependencies.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/sidebar/integration-plugin-layout.test.ts lib/sidebar/layout-projection.test.ts lib/sidebar/layout-operations.test.ts lib/sidebar/immediate-writer.test.ts components/app-sidebar/sidebar-layout-navigation.test.tsx components/app-sidebar/sidebar-customize-menu.test.tsx components/navigation/mobile-sidebar-customization.test.tsx components/app-sidebar/sidebar-draggable-navigation.test.tsx components/app-sidebar/sections/integrations-section.test.tsx components/navigation/app-nav-sheet.test.tsx components/integrations/mobile-integrations-section.test.tsx components/settings/sidebar-layout-editor.test.tsx components/settings/sidebar-layout-editor-dnd.test.ts components/settings/sidebar-layout-editor-save.test.ts components/app-sidebar/sidebar-navigation-split.test.tsx components/settings/sidebar-layout-editor-state.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

Lint changed web files from repository root, including untracked tests:

```bash
python3 - <<'CHECK'
import subprocess
tracked = subprocess.check_output(['git', 'diff', '--name-only', '-z'], text=True).split('\0')
untracked = subprocess.check_output(['git', 'ls-files', '--others', '--exclude-standard', '-z'], text=True).split('\0')
files = sorted({p.removeprefix('apps/web/') for p in tracked + untracked
                if p.startswith('apps/web/') and p.endswith(('.ts', '.tsx'))})
if files:
    subprocess.run(['pnpm', 'exec', 'eslint', *files], cwd='apps/web', check=True)
CHECK
```

The new layout-navigation, direct-menu, phone-customization, and helper suites
are owned by this work order. Include any additional changed suites in the final
recorded Vitest command.

## Files likely touched

All paths below are under `apps/web/`, except the final guidance file.

- `lib/sidebar/integration-plugin-layout.ts` and `.test.ts` (new),
  `layout-projection.ts`, `layout-projection.test.ts`, `layout-operations.ts`
  and `.test.ts` as needed for shared scoped moves.
- `components/app-sidebar/sidebar-layout-navigation.tsx` and new `.test.tsx`,
  `sidebar-customize-menu.tsx` and its `.test.tsx`, `sidebar-draggable-navigation.tsx` and `.test.tsx`,
  `sections/integrations-section.tsx` and `.test.tsx`, `sidebar-navigation-split.tsx` and `.test.tsx`.
- `components/navigation/mobile-sidebar-layout-navigation.tsx`,
  `mobile-sidebar-customization.tsx` and new `.test.tsx`, `app-nav-sheet.test.tsx`.
- `components/integrations/integrations-menu.tsx`,
  `mobile-integrations-section.test.tsx`.
- `components/settings/sidebar-layout-editor.tsx`,
  `sidebar-layout-editor-node-list.tsx`, `sidebar-layout-editor-dnd.ts`,
  `sidebar-layout-editor-preview.tsx`, `sidebar-layout-editor-state.test.tsx`, related node editor components and their tests.
- Existing locale files only if unavoidable new copy is introduced.
- `apps/web/AGENTS.md`: remove the outdated standalone integration-plugin rule.

## Dependencies

None.

## Risks

See plan risks. In particular, preserve unsupported-version and unavailable-node
handling, stale-write protection, and plugin ownership through existing helpers.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/sidebar-customization.md), criteria above.
- [Design](../../specs/ui/system-design/sidebar-customization.md#integration-plugin-grouping).
- Existing sections, projection, direct movement, and editor tests named above.
- [Plugin contract](../plugins/PLUGIN-API.md#registry-pluginregistry).

## Results

Completed on 2026-10-26 against base `6677802efa4fac6c2ecc909cb433477c90f7fb2d`.

- Behavioral RED: the desktop layout test could not find YouTrack inside
  Integrations. Scoped-move and no-op-write tests also failed before correction.
- The final Vitest command above passed: 16 suites, 112 tests. It includes direct menu, phone controls, and portaled-focus isolation.
- Typecheck passed with an 8 GB Node heap. The default heap exhausted memory.
- ESLint passed on all changed/new TS and TSX files with zero warnings.
- i18n check and new-code ratchet passed. No new UI copy or locale keys.
- Catalog and specification lint passed. Harness tests passed (19 tests), and
  full harness lint plus the targeted harness hook passed.
- `git diff --check` passed.
- Actual YouTrack v0.2.0 passed desktop and touch-phone placement and activation
  checks. See [reproduction](reproduction.md) for the observed result and limits.

The grouping and movement helpers serve both navigation and customization.
Flat saved nodes and existing wire/CAS contracts are preserved. The source-level
regression uses the actual registry and projection with minimal boundary mocks.


## Review remediation

Review found two layout cases: an inferred Integrations parent was
absent from explicit edit state, and wider listing menus received the phone-only
collapsed disclosure. The parent now materializes for both rendering and editing
without writing on read. The shared resource component passes the phone
presentation choice through instead of applying it to every consumer.
Regression tests reproduce both defects before correction. The desktop browser
flow additionally edits a layout that originally lacks the parent, and a new
coarse-pointer tablet flow opens the listing menu with expanded plugin links.
Fresh published-plugin screenshots passed on desktop, phone, and tablet. The touch/pen pointer handler now preserves propagation to the context-menu trigger. The Office singleton check passes without a code change because builtin eligibility removes its Integrations parent. The shared draft materialization also supplies the legacy parent to Settings, so its controls remain editable. Focused validation passed: 111 tests in 16 suites, typecheck, changed-file ESLint, i18n, docs, and specification checks. Final browser regression passed: 12 desktop tests, including legacy-parent editing and tablet navigation, plus 11 phone tests owned by Task 02.

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
