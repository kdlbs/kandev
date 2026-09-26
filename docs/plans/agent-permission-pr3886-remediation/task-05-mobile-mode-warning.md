---
id: "05-mobile-mode-warning"
title: "Show mode mismatch on phones"
status: done
wave: 2
depends_on:
  - "03-acp-mode-confirmation"
plan: "plan.md"
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-002
acceptance_criteria:
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.2
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.3
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.18
system_design:
  - ../../specs/agents/system-design/agent-permission-control-integrity.md
---

# Task 05: Show mode mismatch on phones

## Summary

Make the requested/effective mode mismatch readable by touch while keeping
the same mode choices and selection handler on desktop and phone.

## In scope

- Use the shipped task `MobilePickerSheet` pattern for the phone selector.
- Show both mode names ahead of the choices when they differ.
- Add localized copy, accessible names, focus return, and mobile E2E coverage.

## Out of scope

- Changing mode precedence or provider mode names.
- A general redesign of task composer controls.

## Acceptance

1. Phone users open one sheet and can read the mismatch and choose any available mode.
2. The sheet owns scrolling, clears the safe area, has 44px touch rows, and returns focus on dismissal.
3. Desktop keeps its compact selector, matching mode state, and keyboard path.

## ASCII UI preview

`UI-01`, phone mismatch state. See the [full preview](plan.md#ascii-ui-preview).
The trigger, both mode names, choice list, and close action are required.

```text
[! Default  v]
       bottom picker
  Permission mode                    [Close]
  Requested: Bypass
  Effective: Default
  -----------------------------
  [x] Default
      Bypass
  (safe-area padding)
```

## Verification

```bash
cd apps/web
pnpm test -- components/task/selector-trigger-class.test.tsx
pnpm run typecheck
pnpm run i18n:check
pnpm e2e:run --project=mobile-chrome e2e/tests/task/mobile-session-mode-mismatch.spec.ts
```

## Files likely touched

- `apps/web/components/task/mode-selector.tsx`
- `apps/web/components/task/selector-trigger-class.test.tsx`
- `apps/web/e2e/tests/task/mobile-session-mode-mismatch.spec.ts`
- `apps/web/src/locales/*/task.json`

## Dependencies

Task 03 defines the mode result that the UI displays.

## Risks

The selector is shared by desktop and mobile chat toolbars. Keep one state
source and avoid a second mode mutation path.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/agents/requirements/agent-permission-control-integrity.md), `002.2`, `002.3`, `002.18`.
- [System design](../../specs/agents/system-design/agent-permission-control-integrity.md), mode mismatch presentation.
- [Mobile UI language](../../../.agents/skills/mobile-parity/references/kandev-mobile-ui-language.md).

## Results

The mode selector now branches on phone or coarse-pointer input. Phone users
open the shipped `MobilePickerSheet`, which shows the requested/effective mode
warning before the available choices and uses the existing `setSessionMode`
action. The fine-pointer dropdown and its keyboard behavior remain unchanged.

The picker has one internal scroll area with safe-area padding, 44px trigger,
close, and option targets, and returns focus to its trigger after dismissal.
Existing localized permission-mode and close strings are reused; no locale keys
were added.

Verification passed:

- `pnpm exec vitest run components/task/selector-trigger-class.test.tsx` (9 tests)
- `pnpm exec eslint components/task/mode-selector.tsx components/task/mobile/mobile-pill-button.tsx components/task/selector-trigger-class.test.tsx e2e/tests/chat/agent-permission-unattended.spec.ts e2e/tests/task/mobile-session-mode-mismatch.spec.ts` (no warnings)
- `pnpm run typecheck`
- `pnpm run i18n:check`
- `pnpm e2e:run --project=mobile-chrome e2e/tests/task/mobile-session-mode-mismatch.spec.ts`

The mobile browser scenario confirms the warning and both choices, 44px action
targets, internal scrolling and safe-area padding, API selection, focus return,
and no document-level horizontal overflow. The rendered phone sheet was also
captured and visually checked against the plan preview.
