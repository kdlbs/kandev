---
id: "03-expose-workflow-policy"
title: "Build the combined step agent selector"
status: done
wave: 3
depends_on:
  - "02-implement-safe-session-parking"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.6
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.7
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.8
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.10
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.11
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 03: Build the Combined Step Agent Selector

## Summary

Replace the simple step profile select and workflow-level policy control with
one robust step selector. It searches profiles, exposes session behavior as a
nested setting, and shares draft/save logic across a desktop popover and mobile
inset drawer.

## In scope

- Add the step policy to frontend HTTP, boot, WebSocket, and domain types.
- Add it to step draft hydration, saved baseline, dirty tracking, updates,
  coordinated save, duplication, and read-only rendering.
- Build a dedicated `WorkflowStepAgentProfileSelector`.
- Show selected profile and session behavior in the closed trigger.
- Add searchable profile choice and a nested three-option session behavior view.
- Use a field-style popover on desktop and an inset bottom drawer on phones.
- Preserve profile health, workflow-default fallback, focus restoration, and the
  existing conditional `configure_session` incompatibility.
- Remove the workflow-level policy component, draft state, and translations.
- Add five-locale copy and focused utility/component tests.

## Out of scope

- Runtime session routing.
- Browser E2E and public documentation.
- Folding conditional model configuration into the new selector.

## Acceptance

- Profile and policy changes participate in one step draft and the shared
  workflow save action.
- The trigger summarizes both selected values and marks dirty when either
  differs from the saved step.
- The profile list is searchable and the policy view has Back navigation and
  descriptions for all three choices.
- Synced workflows display both values but do not mutate them.
- The mobile drawer has at least 44 px active dimensions, one scroll owner,
  focus return, safe-area spacing, and no horizontal overflow.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run hooks/domains/settings/use-workflow-settings.test.ts components/settings/workflow-dirty-state.test.ts components/settings/workflow-step-agent-profile-selector.test.tsx
cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/lib/types/http.ts`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/state/slices/kanban/types.ts`
- `apps/web/hooks/domains/settings/use-workflow-settings.ts`
- `apps/web/hooks/domains/settings/use-workflow-settings.test.ts`
- `apps/web/components/settings/workflow-dirty-state.ts`
- `apps/web/components/settings/workflow-dirty-state.test.ts`
- `apps/web/components/settings/workflow-pipeline-editor-panels.tsx`
- `apps/web/components/settings/workflow-step-agent-profile-selector.tsx`
- `apps/web/components/settings/workflow-step-agent-profile-selector.test.tsx`
- `apps/web/components/model-config-selector-content.tsx` only if a small
  reusable hierarchical-selector primitive is extracted
- `apps/web/src/locales/*/workflows.json`

## Dependencies

Task 02.

## Risks

- The step draft has separate displayed and saved baselines. Omitting either
  creates lost updates or phantom dirty state.
- Reusing model-specific types would couple workflow profiles to model
  configuration and make future changes brittle.
- A nested mobile picker can create two scroll owners or lose focus unless the
  responsive shell owns navigation explicitly.

## Parallelism

`sequential`

## Inputs

- System-design Combined step agent selector and Mobile design contract
  sections.
- Existing `StepAgentProfileSelect`, `ModelConfigSelector`, responsive drawer
  primitives, and mobile workflow settings tests.

## Results

Replaced the workflow-level control and standalone step profile selector with a
combined step agent-profile and session-behavior selector. Desktop uses a
searchable popover; phones use an inset drawer with nested policy navigation,
focus return, safe-area spacing, and a single scroll owner. Step draft, dirty,
save, sync read-only, duplication, and five-locale contracts are covered. The
focused frontend suite passed 52 tests, plus lint, typecheck, i18n checks, and
the new-code ratchet.
