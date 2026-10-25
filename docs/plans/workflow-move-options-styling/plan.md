---
created: 2026-09-13
status: done
requirements:
  - REQ-UI-CONTROL-SIZING-001
system_design:
  - ../../specs/ui/system-design/control-sizing.md
legacy_specs: []
---

# Implementation Plan: Workflow move options styling

## Overview

Normalize the shared one-time move form's typography and spacing across existing
menus, popovers, dialogs, and touch surfaces. This delivery record documents the
completed presentation correction and its validation; it introduces no new
workflow behavior.

## Scope and technical approach

`WorkflowMoveOptionsFields` owns explicit 12px desktop and 14px touch label
baselines instead of inheriting different host fonts. Use 12px field gaps,
consistent checkbox spacing, and medium-weight instructions labeling.
`StepMenuSubItem` increases its inner form padding from 4px to 8px.
Preserve the shared textarea's 16px touch anti-zoom rule, Move action dimensions,
existing state, translations, and surface selection.

Backend behavior, workflow prompts, new copy, and overlay restructuring are out
of scope. The existing mobile task move picker remains the touch exemplar.

## ASCII UI preview

UI-01: Expanded move fields, reached through destination options.

```text
[ ] Clear the agent context before entering
[ ] Skip the step prompt                 (i)
One-time instructions
[ Add instructions for the receiving agent ]
                                  [ Move ]
```

The field order is shared. Desktop keeps the existing anchored surface and 28px
Move action. Phone retains its existing bottom surface, scroll owner, and safe
area, with 44px actions and 16px editable text. Spacing above is illustrative.
These checks preserve AC-UI-CONTROL-SIZING-001.1, .4, .8, and .9.

## Work orders

- [x] [Task 01: Normalize shared move fields](task-01-normalize-move-fields.md)

## Verification results

The work order records 50 passing focused tests, production builds, rendered
font/geometry checks, and a compatible synthetic merge with the newer base.
No public docs update is needed: control behavior and user-facing wording are
unchanged. Remote CI and automated review status remain separate delivery gates.

## Risks

An inherited font can differ across portaled hosts. Touch inputs intentionally
remain larger than labels because the global anti-zoom minimum is authoritative.
