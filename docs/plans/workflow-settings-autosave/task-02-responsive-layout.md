---
id: "02-responsive-layout"
title: "Responsive workflow editor layout"
status: done
wave: 2
depends_on: ["01-autosave-state"]
plan: "plan.md"
spec: "../../specs/workflow-settings-autosave/spec.md"
---

# Task 02: Responsive Workflow Editor Layout

## Acceptance

- Page actions, workflow details, card actions, and step header controls fit narrow viewports without document-level horizontal overflow.
- The workflow pipeline retains its own horizontal scrolling and desktop layout remains dense and usable.
- Required mobile controls have touch-reachable dimensions and do not rely on hover.

## Verification

```bash
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/components/settings/settings-section.tsx`
- `apps/web/components/settings/workflow-card.tsx`
- `apps/web/components/settings/workflow-pipeline-editor.tsx`
- `apps/web/components/settings/workflow-pipeline-editor-panels.tsx`

## Inputs

- Spec: responsive requirements and 390px Scenario.
- Mobile parity guidance and current workflow settings screenshots.

## Output Contract

Report responsive rules changed, rendered viewport checks, files touched, blockers, and update this task plus `plan.md` to done.
