---
id: "01-picker-outcomes"
title: "Picker outcomes and selector feedback"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-CONFIG-FEEDBACK-001
acceptance_criteria:
  - AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.1
  - AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.2
  - AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.3
  - AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.5
  - AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.6
system_design:
  - ../../specs/agents/system-design/session-configuration-feedback.md
---

# Task 01 Picker outcomes and selector feedback

This work order records the original picker implementation. [Task 03](task-03-muted-transcript-feedback.md)
owns the revised presentation.

## Scope

Return the known outcome from the existing session setter. Keep the existing
picker apply sequence and use each step's result to describe what happened.
Replace the picker toast or console-only failure with feedback next to the
originating trigger. Keep the existing partial Retry action there.
[Picker response design](../../specs/agents/system-design/session-configuration-feedback.md#picker-responses)

Task 02 covers slash commands in the conversation. Settings editors keep
their own Save flow.

## Acceptance

1. A confirmed reply names the effective value. A saved preference, a
   clamped value, a rejection, and missing evidence each get accurate wording.
2. Partial application identifies the observed changes and the failed value.
   Retry stays scoped to the current action and execution, and checks its
   expected base model at the mutation boundary. Selector state uses the
   provider's effective value. An obsolete reply cannot confirm or roll back
   a newer action.
3. Desktop and phone announce the result beside the selector. A green
   check clears after one visible second. The trigger and nearby controls
   remain usable throughout. Failure is dismissible without losing the
   draft or changing the reading position.

## ASCII UI preview

### UI-01 Picker feedback

```text
                         +-----------+
                         | [OK] Plan |
                         +-----------+
[ Model: Sonnet v ]   [ Mode: Plan v ]   [Send]
```

The passive indicator appears above the originating trigger on both
surfaces. Dismiss and Retry use at least 44 px touch targets on phone.
This covers ACs .1 through .3, .5, and .6.
[Full UI-01 preview](plan.md#ui-01-picker-feedback)

## Verification

Start with `TestSessionConfigurationFeedbackPickerHTTP` in
`process_handlers_test.go`. The current fixture saves the override and
returns bare `ok`. Add setter cases for an effective value, explicit
rejection, and missing evidence. Use an ACP option or value ID supplied by
the fixture to exercise the shared path across agent types.
Extend the existing composition tests for the first failure and the
remaining retry. Use fake timers in `session-configuration-feedback.test.ts`
to test the one-second expiry and replacement by a newer selection.

Under `Picker feedback`, test a real mock picker success and a deterministic
rejection on desktop and phone. Use the Playwright clock to reopen the
picker with the indicator visible and choose again. Check dismissal,
accessible status, and the absence of a duplicate toast. On phone, keep a
draft and a reader position, then check the long rejection in a 420 px
reduced viewport. Measure the Dismiss target and check that the scrollable
feedback remains within the viewport. Update the existing desktop failure
test to expect an unconfirmed result and retain the last confirmed trigger
label.

If workspace dependencies are missing, run
`(cd apps && pnpm install --frozen-lockfile)`. Run the desktop build before
the no-build phone run.

```bash
(cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp \
  ./internal/agentctl/server/api ./internal/agent/runtime/agentctl \
  ./internal/agent/runtime/lifecycle ./internal/task/handlers \
  -run 'Test(SessionConfiguration.*|AgentSessionConfigOption.*|SetSessionRuntimeConfigOverridesPersistWithoutRunningAgent|SetModeUsesAdvertisedModeConfigOptionAndAuthoritativeClamp)$' -count=1)
(cd apps/web && pnpm run test lib/apply-model-configuration.test.ts \
  components/task/model-selector-composed-config.test.tsx \
  components/task/model-selector-consecutive-switch.test.tsx \
  components/task/session-configuration-feedback.test.tsx \
  lib/state/slices/session-runtime/session-configuration-feedback.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --project chromium \
  tests/chat/session-configuration-feedback.spec.ts tests/chat/model-selector-error.spec.ts \
  -- --grep 'Picker feedback|shows inline uncertainty')
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome \
  tests/chat/mobile-session-configuration-feedback.spec.ts -- --grep 'Picker feedback')
git diff --check
```

## Files likely touched

- Backend setter path through `adapter_session.go`, `server/api/agent.go`,
  `runtime/agentctl/agent.go`, `manager_interaction.go`, and
  `internal/task/handlers/process_handlers.go`, with adjacent tests.
- Web `lib/api/domains/session-api.ts`, `lib/apply-model-configuration.ts`,
  `components/task/model-selector-change-handlers.ts`, and
  `components/task/model-selector.tsx` and `components/task/mode-selector.tsx`.
- Selector feedback, runtime notice state, chat locale catalogs, and the
  Playwright specs in the commands above.

## Dependencies

None. Execute sequentially.

## Risk and results

Selector state can reflect a saved preference or an optimistic update. Only
the current session's execution-scoped setter response can confirm application.
The setter response and ACP adapter return structured evidence. Focused Go and
frontend tests passed, as did the desktop and phone picker acceptance checks.
The phone notice uses a viewport layer because the toolbar's horizontal
scroller clips positioned children. The long rejection remained reachable
with the phone keyboard viewport open, without changing the reader position.
