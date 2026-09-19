---
id: task-05-picker-gone-support
title: Model pickers support disabled (gone) models
status: done
wave: 3
depends_on: []
plan: docs/plans/no-silent-model-fallback/plan.md
spec: docs/specs/agents/requirements/no-silent-model-fallback.md
---

# Task 5 — Model pickers: grey out gone models

## Change

`apps/web/components/model-config-selector.tsx`:

- `ModelSelectorOption` gains `disabled?: boolean` and
  `disabledReason?: string`.
- `ModelRow` renders disabled options greyed (`opacity-40`,
  `cursor-not-allowed`) with the reason in a tooltip; `onSelect` guarded by
  `!option.disabled`. Follow the existing disabled pattern in
  `apps/web/components/combobox.tsx` (visual treatment + tooltip).

`apps/web/lib/ws/handlers/session-models.ts`:

- `clearStaleActiveModel` no longer clears the active model when it
  disappears from the ACP list. Keep it; the picker marks it gone.

`apps/web/components/task/model-selector.tsx`:

- `buildModelOptions` / `resolveAvailableModels` mark the configured-but-
  absent models (active model, profile model) as `disabled` with reason
  "model no longer available — select a new model". The current model
  display still shows the (gone) model name so the user sees what was
  active.

i18n: new copy via `t()` (`settings:` namespace) — the ratchet judges added
lines.

## Acceptance

1. Unit test (`model-config-selector`): a disabled option is not selectable
   and renders the greyed class; tooltip carries the reason.
2. Unit test (`session-models` handler): when the ACP models list drops the
   active model, the active model is **kept** (not cleared).
3. Unit test (`model-selector`): a configured model absent from the list is
   included in options as `disabled` with a reason.
4. `pnpm run typecheck` + targeted vitest pass.

## Verification

```sh
cd apps/web && pnpm vitest run components/model-config-selector.test.tsx lib/ws/handlers/session-models.test.ts components/task/model-selector.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:ratchet
```

## Risks

- The session picker's option-building has several sources (ACP models,
  static model config, config options) — the disabled marking must apply to
  the merged option list without breaking the config-option path.
- Keep `clearStaleContextWindow` behavior unchanged (that one is about
  context-window reset on a legit model change, unrelated).
