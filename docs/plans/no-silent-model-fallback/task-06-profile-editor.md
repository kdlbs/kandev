---
id: task-06-profile-editor
title: Profile editor: gone start model red + fallback row + toggle
status: done
wave: 3
depends_on: [task-01-profile-fields, task-05-picker-gone-support]
plan: docs/plans/no-silent-model-fallback/plan.md
spec: docs/specs/agents/requirements/no-silent-model-fallback.md
---

# Task 6 — Profile editor rows

## Change

`apps/web/components/settings/profile-form-fields.tsx` (`CapabilitiesRow` /
`ModelPicker`):

- **Start model**: when `profile.model` is non-empty and absent from the
  current model list, keep it as the current value, render the trigger/row
  red (`text-destructive`) and the option disabled with reason ("no longer
  available — select a new model"). The user can still change it (the
  picker opens and valid models are selectable), but cannot re-select the
  gone one.
- **Agent fallback row** (new, directly under Start model): optional select
  over the same model list bound to `profile.fallback_model`, label
  "Agent fallback" + helper ("if the start model becomes unavailable,
  automatically switch to"). Same gone-red/disabled treatment when the
  fallback model itself is gone. Hidden when `auto_fallback` is ON.
- **"Fallback automatically to next model" toggle** (new row): switch bound
  to `profile.auto_fallback` with helper text. When ON, the fallback-model
  row is hidden (per spec precedence).
- Parity: `apps/web/components/agent/cli-profile-editor.tsx`
  (`ModelModeFields`) gets the same two rows.
- The profile form state type (`ProfileFormData`) gains `fallbackModel` /
  `autoFallback`; dirty-tracking (`profileModelIsDirty`-style helpers) for
  the new fields; save payload flows through `toAgentProfilePayload`.

i18n: ALL new copy via `t()` into `apps/web/src/locales/{en,pseudo}/settings.json`
(camelCase keys): `startModelUnavailable`, `agentFallback`, `agentFallbackHelper`,
`autoFallback`, `autoFallbackHelper`, `fallbackModelUnavailable`. The file
`profile-form-fields.tsx` is unmigrated — added lines are judged by the
ratchet, so no hardcoded literals on new/edited lines.

## Acceptance

1. Unit test: gone start model renders the red treatment + disabled option;
   current value preserved.
2. Unit test: fallback row appears when `auto_fallback` is off; hidden when
   on; toggle hides/shows it.
3. Unit test: gone fallback model renders red + disabled.
4. Unit test: save payload includes `fallback_model` / `auto_fallback`;
   dirty detection works for both.
5. i18n ratchet passes on the edited files.

## Verification

```sh
cd apps/web && pnpm vitest run components/settings/profile-form-fields.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:ratchet
```

## Risks

- The editor has two surfaces (`profile-form-fields.tsx` and
  `cli-profile-editor.tsx`) — keep both consistent.
- The empty-model case ("profile model means use the agent's default")
  must stay untouched: gone-treatment applies only to non-empty models.
- Do not disturb the `data-settings-dirty` container semantics used by the
  settings save indicator.
