---
created: 2026-11-25
status: complete
requirements:
  - REQ-UI-SETTINGS-COMPOSITION-001
system_design:
  - ../../specs/ui/system-design/settings-composition.md
legacy_specs: []
---

# Align the design guide with settings

## Overview

Resolve the documentation gap in [GitHub issue #5504](https://github.com/kdlbs/kandev/issues/5504). Describe settings groups in `DESIGN.md` and direct web contributors to that guidance. Settings use bordered groups. The workbench uses panels.

The UI system owns the reusable settings composition. Use the existing [settings requirements](../../specs/ui/requirements/settings-composition.md#req-ui-settings-composition-001-consistent-settings-groups) and [system design](../../specs/ui/system-design/settings-composition.md#shared-presentation-and-rollout).

## Evidence and rationale

The layout guidance from [PR #761](https://github.com/kdlbs/kandev/pull/761) discourages section cards without describing the settings pattern. The web contributor guide and mobile-parity skill give no reading trigger for `DESIGN.md`.

The September settings-composition requirement specifies card padding and one effective inset for bordered preference groups. [PR #4622](https://github.com/kdlbs/kandev/pull/4622) fixed excessive insets in the shared groups and bounded the Task behavior page width in October. The [component conventions](../../../apps/web/COMPONENT-CONVENTIONS.md#component-conventions) direct contributors to use bordered `SettingsGroup` components and simple `SettingsRow` components.

The following source paths reproduce the mismatch on `c671d8080883`:

1. Read [Layout in DESIGN.md](../../../DESIGN.md#layout) and the [shared settings design](../../specs/ui/system-design/settings-composition.md#shared-presentation-and-rollout).
2. Trace `/settings/preferences/task-behavior` through [TaskBehaviorSettings](../../../apps/web/components/settings/task-behavior-settings.tsx). The [SettingsGroup default](../../../apps/web/components/settings/settings-group.tsx) uses a heading and one `SettingsCard` with divided rows.
3. Trace the workspace automation create and edit routes through [AutomationEditor](../../../apps/web/components/automations/automation-editor.tsx). Its [section components](../../../apps/web/components/automations/automation-editor-sections.tsx) group the When and Then controls in bordered surfaces.
4. Search `apps/web/AGENTS.md` and `.agents/skills/mobile-parity/SKILL.md` for `DESIGN.md`. Both searches return no matches.

A settings-wide migration to bands would require a change to the active settings requirements and to the shipped layouts. The documentation option follows the current settings contract and needs only three Markdown edits.

## Scope

- Clarify the workbench and settings layout guidance in `DESIGN.md`.
- Add explicit reading triggers in `apps/web/AGENTS.md` and the mobile-parity skill.
- Limit edits to the three contributor Markdown files.

## Technical approach

Scope the panel-first and decorative-card guidance to its workbench context. Describe one heading, one bordered group, and one padding layer for related preferences or form fields. Use dividers for simple rows. Preserve resource lists, tables, and specialized editor layouts in the guidance. Link the shared settings design for details.

Add a short reading trigger to each contributor entry point. Both links lead to `DESIGN.md#layout`. The [settings composition rollout](../settings-composition/plan.md) and [spacing correction](../task-behavior-spacing/plan.md) are implementation evidence for the existing contract.

The phone reference is the shipped [settings shell](../../../apps/web/components/settings/settings-layout-client.tsx). Its breadcrumb leads to the Settings menu, and the page owns vertical scroll. The guidance follows the existing 44 px touch-target and viewport constraints in `AC-UI-SETTINGS-COMPOSITION-004.1`, `.2`, and `.4`.

## Verification

Run the exact link, heading, and lint checks in the [work order](task-01-align-guidance.md#verification). The link checks reproduce the documentation defect on the base revision. Compare the new wording with the linked design and the two source traces above.

- Desktop `settings/settings-composition.spec.ts` checks one frame and one padding layer. It also checks grouped preferences and workspace resource surfaces for `AC-UI-SETTINGS-COMPOSITION-001.1`, `.2`, `.3`, and `.6`.
- Phone `settings/mobile-settings-composition.spec.ts` checks containment, one scroll region, and 44 px touch targets for `AC-UI-SETTINGS-COMPOSITION-004.1`, `.2`, and `.4`.

## Work order

- [x] [Task 01. Align settings layout guidance](task-01-align-guidance.md)

## Verification results

Source inspection confirmed the shipped bordered groups. Each of the three link checks returned exit code 1 on the base revision, as expected.

- `make lint-harness lint-specs` passed.
- `python3 scripts/list-docs.py validate` passed.
- Local package links, heading anchors, and whitespace checks passed.
- `pnpm --filter @kandev/web build:vite` passed with existing chunk-size and dynamic-import warnings.
- The focused `SettingsGroup` test passed (7 tests).
- The full web unit run emitted an unhandled `ECONNREFUSED` for `localhost:3000` and was stopped after eight minutes without a test summary. The preview-feedback integration test passed when run by itself.

The work-order implementation checks passed. Browser E2E cases were not run because this change does not alter UI behavior.

## Risks

An unqualified rule about settings cards can encourage extra frames around rows or forced frames around existing lists and editors. Describe where the group frame belongs and link to the design for exceptions.
