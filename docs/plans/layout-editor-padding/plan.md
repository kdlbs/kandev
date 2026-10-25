---
created: 2026-10-22
status: implemented
requirements:
  - REQ-UI-SETTINGS-COMPOSITION-001
  - REQ-UI-SETTINGS-COMPOSITION-004
system_design:
  - ../../specs/ui/system-design/settings-composition.md
legacy_specs: []
---

# Restore layout editor side padding

## Overview

Restore the standard horizontal inset around the Layout profiles list and editor.
One work order corrects a local spacing override and checks desktop and phone rendering.

## Evidence and ownership

The supplied screenshot shows the profile list and editor touching the outer card edges.
`LayoutSettings` passes `contentClassName="p-0 divide-y-0"` to the profiles `SettingsGroup`.
`SettingsGroupContent` normally supplies `px-4 py-2`. The `p-0` override removes both axes.
`LayoutProfilesPanel` and `SettingsTarget` add no replacement padding. The outer `Card` supplies only vertical padding.

This violates existing `AC-UI-SETTINGS-COMPOSITION-001.1`.
UI owns the reusable settings spacing contract. The existing [requirements](../../specs/ui/requirements/settings-composition.md)
and [design](../../specs/ui/system-design/settings-composition.md#shared-presentation-and-rollout) already define the intended inset.
No requirement or design amendment is needed.

Reproduce by opening `/settings/preferences/layouts` with the built-in Default selected.
Compare the profile list's left edge and the editor's right edge with the card's inner edges.
Source and screenshot confirm the missing inset. Browser measurements confirmed zero padding before the correction on desktop and phone.

## Scope and technical approach

Change only the profiles group's override in `apps/web/components/settings/layouts/layout-settings.tsx`
from `p-0 divide-y-0` to `py-0 divide-y-0`.
This retains shared `px-4` padding, nominally 16px at a 16px root font, and preserves current vertical spacing.

Exclude shared Card changes, Sidebar-tab spacing, editor internals, profile persistence, and new copy.
The existing [settings composition package](../settings-composition/plan.md) remains historical evidence.
Its completed results are not results for this repair.

## UI preview

UI-01, desktop: Settings > General > Layouts > Layout profiles, Default selected.
The card contains a standard inset, the profile list, the existing column gap, the editor, and a matching inset.
The left list and right editor retain their current order and widths subject to the reduced content area.

UI-02, phone: the same route, reached through the Settings index.
The profile actions and list precede the selected editor in the existing single-column flow.
Both sections share the same left and right inset. The page remains the vertical scroll owner.
The current touch controls, menus, and safe-area-aware save surface remain in place.

The inset and viewport containment are required. Profile text and panel contents are illustrative.
Use the existing mobile layout-profile flow as the nearest shipped exemplar.
This spacing repair adds no navigation surface or breakpoint.

## Tests and rendered checks

This is a markup-class correction with no changed logic. No new permanent unit test is needed.
Before the correction, use a disposable browser check to measure zero horizontal content padding.
After the correction, check both insets against the shared spacing token, excluding card borders.
Check the profile list and editor bounds, since document overflow alone can miss ancestor clipping.
Inspect desktop and phone screenshots. Repeat containment checks at 1023px and 1024px around the existing grid breakpoint.

Existing desktop and mobile layout-profile E2E scenarios cover selection, editing, saving, and touch behavior.
Run the exact commands in [Task 01](task-01-restore-side-padding.md).
The rendered inset check covers `AC-UI-SETTINGS-COMPOSITION-001.1`.
Phone bounds and overflow checks cover `AC-UI-SETTINGS-COMPOSITION-004.2` and `.4`.

## Work orders

- [x] [Task 01: Restore side padding](task-01-restore-side-padding.md), done, sequential.

## Verification results

Implementation completed on 2026-10-22. One production class changed.
Dependency installation and targeted ESLint passed. Desktop E2E passed 8 scenarios and mobile E2E passed 6 scenarios.
Each count includes one disposable capture check, removed after verification.
Both insets measured 16px at 393, 1023, 1024, and 1440px viewport widths, with zero document horizontal overflow.
Desktop and phone screenshots were inspected. See the [work-order results](task-01-restore-side-padding.md#results) for exact commands and host setup.

Design validation on 2026-10-22:

- `python3 scripts/list-docs.py validate`: passed, 384 decisions and 1616 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Relative links, requirement IDs, design mappings, and untracked-file whitespace: passed.
- `git diff --check` and `git status --short`: passed; only this untracked package is present.
- PR documentation coverage has no local command entry point in this checkout. Cross-references were checked directly.

## Risks

Restoring both insets reduces editor width. Check phone controls for clipping and horizontal overflow.
Workspace dependencies were installed before package commands.
Public documentation has no changed terminology, workflow, or published screenshot in this scope.
