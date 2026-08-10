---
id: dynamic-provider-options-04
title: Integrate settings selectors
status: done
wave: 4
depends_on:
  - dynamic-provider-options-03
plan: docs/plans/dynamic-provider-options/plan.md
spec: docs/specs/agents/dynamic-provider-options.md
---

# Integrate settings selectors

## Scope

Wire the shared model-resolution behavior into workflow session rules and ACP
agent profile settings while preserving existing persistence, read-only, and
generic selector behavior.

## Acceptance conditions

1. Workflow rules display and save arbitrary provider-returned options after a
   model change, including replacement/removal of options that the new model
   does not advertise.
2. ACP agent profile settings use the same resolver and save/reload the same
   dynamic options without provider-specific branches.
3. Loading, retry, and error states are localized; mobile layouts retain
   touch-sized controls and no horizontal overflow.

## Files

- `apps/web/components/settings/workflow-session-config-rule-card.tsx`
- `apps/web/components/settings/workflow-session-config-shared.ts`
- `apps/web/components/settings/workflow-session-config-editor.test.tsx`
- `apps/web/components/settings/profile-form-fields.tsx`
- `apps/web/components/settings/profile-form-fields.test.tsx`
- `apps/web/components/model-config-selector.tsx` (only if an optional
  resolver status affordance is required)
- `apps/web/public/locales/` affected locale catalogs

## Dependencies and inputs

- `dynamic-provider-options-03` hook and reconciliation contract.
- Existing workflow rule serializer, profile save coordinator, and
  `ModelConfigSelector` select-option behavior.
- Mobile-parity requirements for the existing settings layouts.

## Output contract

Both settings surfaces consume the same provider-neutral resolved snapshot.
Persisted config maps are not mutated by a failed discovery request; successful
model changes save only values valid for the returned snapshot.

## Checks

```bash
cd apps && pnpm --filter @kandev/web exec vitest run components/settings/profile-form-fields.test.tsx components/settings/workflow-session-config-editor.test.tsx components/model-config-selector.test.tsx --reporter=dot
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
```

Results: profile/workflow component tests, frontend typecheck, ESLint, i18n
check, and i18n ratchet — passed.
