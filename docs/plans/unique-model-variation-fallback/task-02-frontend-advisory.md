---
id: "02-frontend-advisory"
title: "Present variation decisions"
status: done
wave: 2
depends_on: ["01-runtime-resolution"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-002
acceptance_criteria:
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.3
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.4
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.5
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.6
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.7
system_design:
  - ../../specs/agents/system-design/no-silent-model-fallback-01.md
---

# Task 02: Present Variation Decisions

## Summary

Present one host variation as an advisory and render the runtime decision in
task chat. Keep the host probe non-authoritative on every viewport.

## In scope

- Add the mirrored frontend matcher and table tests.
- Update task-create and profile-editor missing-model states.
- Render the new warning reason with requested and effective models.
- Add all required localized copy.

## Out of scope

- Runtime model selection.
- New dialog, drawer, or navigation patterns.
- Changes to the saved profile model.

## Acceptance

- One host variation names a possible target and keeps the profile selectable.
- Zero or multiple host variations keep the existing missing-model state.
- Desktop, keyboard, and touch users can reach the advisory and durable warning.

## Verification

```bash
(
  cd apps
  pnpm --filter @kandev/web test -- --run components/task-create-dialog-options components/settings/profile-form-fields components/task/chat/messages/status-message lib/model-variation
  pnpm --filter @kandev/web lint
)
(
  cd apps/web
  pnpm run typecheck
  pnpm run i18n:check
  pnpm run i18n:ratchet
)
git diff --check
```

## Files likely touched

- `apps/web/lib/model-variation.ts`
- `apps/web/lib/model-variation.test.ts`
- `apps/web/components/task-create-dialog-options.tsx`
- `apps/web/components/task-create-dialog-options.test.tsx`
- `apps/web/components/settings/profile-model-fields.tsx`
- `apps/web/components/settings/profile-form-fields.test.tsx`
- `apps/web/components/task/chat/messages/status-message.tsx`
- `apps/web/components/task/chat/messages/status-message.test.tsx`
- `apps/web/src/locales/*/settings.json`
- `apps/web/src/locales/*/task.json`

## Dependencies

Task 01.

## Risks

- Host preview text can sound authoritative when another executor can advertise
  a different catalog.
- Missing and resolvable states must be mutually exclusive in the profile row.

## Parallelism

`sequential`

## Inputs

- Task 01 warning reason and payload fields.
- Existing host warning tooltip, touch drawer, profile selector, and status
  message patterns.

## Results

Added the mirrored structural matcher, host advisory states, and the task-chat
warning reason while preserving the saved profile model. Added localized copy
for all required catalogs and retained the existing desktop tooltip and mobile
touch-drawer patterns. Focused frontend tests passed (51), typecheck and lint
passed, and both i18n checks passed.
