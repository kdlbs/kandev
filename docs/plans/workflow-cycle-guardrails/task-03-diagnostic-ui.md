---
id: "03-diagnostic-ui"
title: "Workflow cycle diagnostic UI"
status: done
wave: 3
depends_on: ["02-mutation-guard"]
plan: "plan.md"
spec: "../../specs/workflow-cycle-guardrails/spec.md"
---

# Task 03: Workflow cycle diagnostic UI

## Acceptance

- Existing and proposed diagnostics render severity, exact ordered trigger
  trace, user-required hops, and prompt source through one reusable component.
- Blocking dialogs expose no override; warning dialogs use **Apply anyway** or
  **Create anyway**, and affected pipeline steps are identified without color
  alone.
- Desktop and narrow layouts keep text/actions usable, keyboard/touch
  accessible, and page overflow contained.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/settings/workflow-cycle-diagnostic.test.tsx components/settings/workflow-card-actions.test.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
```

## Files Likely Touched

- `apps/web/components/settings/workflow-cycle-diagnostic.tsx`
- `apps/web/components/settings/workflow-cycle-diagnostic.test.tsx`
- `apps/web/components/settings/workflow-pipeline-editor.tsx`
- `apps/web/components/settings/workflow-card.tsx`
- `apps/web/components/settings/workflow-card-dialogs.tsx`

## Dependencies

Task 02.

## Inputs

- Spec prompt-source language, trace requirements, and mobile behavior.
- Pending-operation state from Task 02.
- Existing `AlertDialog`, workflow pipeline scroll area, and workflow-card
  dialog patterns.

## Output Contract

When finished, change this task's `status` to `done`, check it in `plan.md`, and
report responsive behavior, accessibility treatment, files changed, tests run,
blockers, and residual risks.
