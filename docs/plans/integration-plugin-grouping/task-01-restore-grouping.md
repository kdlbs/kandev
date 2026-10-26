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
(cd apps/web && pnpm exec vitest run lib/sidebar/integration-plugin-layout.test.ts lib/sidebar/layout-projection.test.ts lib/sidebar/layout-operations.test.ts lib/sidebar/immediate-writer.test.ts components/app-sidebar/sidebar-layout-navigation.test.tsx components/app-sidebar/sidebar-customize-menu.test.tsx components/navigation/mobile-sidebar-customization.test.tsx components/app-sidebar/sidebar-draggable-navigation.test.tsx components/app-sidebar/sections/integrations-section.test.tsx components/navigation/app-nav-sheet.test.tsx components/integrations/mobile-integrations-section.test.tsx components/settings/sidebar-layout-editor.test.tsx components/settings/sidebar-layout-editor-dnd.test.ts components/settings/sidebar-layout-editor-save.test.ts)
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
  `sections/integrations-section.tsx` and `.test.tsx`.
- `components/navigation/mobile-sidebar-layout-navigation.tsx`,
  `mobile-sidebar-customization.tsx` and new `.test.tsx`, `app-nav-sheet.test.tsx`.
- `components/integrations/integrations-menu.tsx`,
  `mobile-integrations-section.test.tsx`.
- `components/settings/sidebar-layout-editor.tsx`,
  `sidebar-layout-editor-node-list.tsx`, `sidebar-layout-editor-dnd.ts`,
  `sidebar-layout-editor-preview.tsx`, related node editor components and their tests.
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
- The exact Vitest command above passed: 14 suites, 95 tests. It includes direct menu and phone controls.
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

