---
id: "01-normalize-move-fields"
title: "Normalize shared move fields"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CONTROL-SIZING-001
acceptance_criteria:
  - AC-UI-CONTROL-SIZING-001.1
  - AC-UI-CONTROL-SIZING-001.4
  - AC-UI-CONTROL-SIZING-001.8
  - AC-UI-CONTROL-SIZING-001.9
system_design:
  - ../../specs/ui/system-design/control-sizing.md
---

# Task 01: Normalize shared move fields

## Scope and owned files

- `apps/web/components/task/workflow-move-options.tsx`: explicit field typography,
  consistent gaps, label emphasis, and an accurate ownership comment.
- `apps/web/components/task/task-move-context-menu.tsx`: inner form padding.

No changes to payloads, state, translations, or desktop/mobile surface selection.

## Acceptance

1. Shared labels render at 12px on desktop and 14px on touch surfaces, independent
   of their host. Touch editing retains the 16px anti-zoom minimum (001.9).
2. The existing Move action remains 28px desktop and at least 44px touch
   (001.1, 001.4), with existing submission and dismissal behavior (001.8).
3. Checkbox spacing is consistent, instructions have a medium-weight label, and
   menu padding contains the form without horizontal overflow.

## ASCII UI preview

UI-01 from the [plan](plan.md#ascii-ui-preview):

```text
[ ] Reset context
[ ] Skip step prompt                     (i)
One-time instructions
[ multiline input                         ]
                                  [ Move ]
```

Labels above abbreviate existing localized copy. Desktop remains anchored;
phone keeps its bottom surface and existing scroll/safe-area ownership.

## Verification commands

From `apps/web`:

```bash
pnpm exec vitest run components/task/workflow-move-options-form.test.tsx components/task/task-switcher-context-menu.test.tsx components/task/workflow-move-proceed-button.test.tsx
pnpm exec eslint components/task/workflow-move-options.tsx components/task/task-move-context-menu.tsx
pnpm exec prettier --check components/task/workflow-move-options.tsx components/task/task-move-context-menu.tsx
```

From repository root: `make build-web` and `make -C apps/backend build-dev`.

## Results

- 50 focused tests passed; targeted lint, formatting, and normal commit hooks passed.
- Both builds passed. The seeded task's move-options surface loaded through the
  isolated backend. This was UI verification, not a successful agent-turn test.
- Chromium component previews verified desktop labels/input at 12px, Move at
  28px; Pixel 5 labels at 14px, input at 16px, Move at 44px; no horizontal overflow.
- Touch checks at 767, 768, 900, and 1024px all measured 14px labels and 16px input.
  `app/globals.css` overrides the textarea's breakpoint font with the anti-zoom rule.
- The same 50 tests passed on a conflict-free synthetic merge with the newer base.
- Existing desktop/mobile `workflow-step-move-overrides` E2E specs cover the move
  workflow, but full workflow E2E was not run for this presentation-only patch.
  Temporary rendered checks were removed after verification.

## Risks and documentation

Preserve the touch input anti-zoom minimum even when label typography differs.
No public docs change is needed because behavior and terminology are unchanged.
