---
id: "01-align-guidance"
title: "Align settings layout guidance"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SETTINGS-COMPOSITION-001
acceptance_criteria:
  - AC-UI-SETTINGS-COMPOSITION-001.1
  - AC-UI-SETTINGS-COMPOSITION-001.2
  - AC-UI-SETTINGS-COMPOSITION-001.3
  - AC-UI-SETTINGS-COMPOSITION-001.6
system_design:
  - ../../specs/ui/system-design/settings-composition.md
---

# Task 01. Align settings layout guidance

## Summary

Document the shipped settings layout in the root design guide. Make that section required reading for web UI work and mobile-parity work.

## In scope

- In `DESIGN.md`, group the panel-first recommendations under the workbench and add a short paragraph on settings layout. Describe section headings, bordered groups with one inset, divided rows, and form fields grouped by purpose.
- Explain that resource lists, tables, and specialized editors use their existing layouts. Link to the shared presentation section of the settings system design.
- In `apps/web/AGENTS.md` and the mobile-parity skill, add one reading trigger that links to `DESIGN.md#layout`. Keep the detailed pattern in the design guide and its linked system design.

## Out of scope

- Component, test, style, and route changes.
- General workbench redesign, public docs, and screenshots.

## Acceptance

1. The Layout section describes the shared settings groups and the settings forms shown in the automation editor. Its description agrees with the linked [settings system design](../../specs/ui/system-design/settings-composition.md#shared-presentation-and-rollout).
2. The web guide and mobile-parity skill each direct contributors to the Layout section before UI work.
3. The diff contains the three contributor Markdown edits and the work-order status and result updates.

## Verification

Run from the repository root. The first three link checks return exit code 1 on the base revision. Each must return a matching Markdown link after the change.

```bash
rg -n -F '](../../DESIGN.md#layout)' apps/web/AGENTS.md
rg -n -F '](../../../DESIGN.md#layout)' .agents/skills/mobile-parity/SKILL.md
rg -n -F '](docs/specs/ui/system-design/settings-composition.md#shared-presentation-and-rollout)' DESIGN.md
rg -n '^## Layout$' DESIGN.md
rg -n '^## Shared presentation and rollout$' docs/specs/ui/system-design/settings-composition.md
make lint-harness lint-specs
python3 scripts/list-docs.py validate
git diff --check
```

Read the resulting Layout section beside `SettingsGroupContent` and the automation editor sections linked in the [source trace](plan.md#evidence-and-rationale). Check that the section describes the existing card boundary and row hierarchy. Existing desktop and phone browser cases in the plan are references for the documented behavior.

## Files likely touched

- [`DESIGN.md`](../../../DESIGN.md#layout)
- [`apps/web/AGENTS.md`](../../../apps/web/AGENTS.md#ui-components)
- [`.agents/skills/mobile-parity/SKILL.md`](../../../.agents/skills/mobile-parity/SKILL.md)

## Dependencies

None.

## Risks

The settings section can lose its link to the current technical design if that file moves. Check both the relative path and the target heading before finishing.

## Parallelism

`sequential`

## Inputs

- [Issue #5504](https://github.com/kdlbs/kandev/issues/5504)
- [Settings composition requirements](../../specs/ui/requirements/settings-composition.md#req-ui-settings-composition-001-consistent-settings-groups)
- [Shared presentation design](../../specs/ui/system-design/settings-composition.md#shared-presentation-and-rollout)

## Results

Completed the three contributor Markdown edits and recorded the plan status.

Verification passed:

- Exact link and heading checks, relative link resolution, and `git diff --check`.
- `make lint-harness lint-specs` and `python3 scripts/list-docs.py validate`.
- `pnpm --filter @kandev/web build:vite`.
- `pnpm exec vitest run components/settings/settings-group.test.tsx` (7 tests).
- `pnpm exec vitest run components/task/inspector/preview-feedback-controls.integration.test.tsx` (2 tests).

The full `pnpm --filter @kandev/web test` run emitted an unhandled `ECONNREFUSED` for `localhost:3000` and was stopped after eight minutes without a test summary. The browser E2E cases were not run because this change does not alter UI behavior.
