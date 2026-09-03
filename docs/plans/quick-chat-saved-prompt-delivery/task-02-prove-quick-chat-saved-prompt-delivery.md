---
id: "02-prove-quick-chat-saved-prompt-delivery"
title: "Prove Quick Chat saved-prompt delivery"
status: done
wave: 2
depends_on:
  - "01-canonicalize-direct-saved-prompt-messages"
plan: "plan.md"
requirements:
  - REQ-TASKS-SAVED-PROMPT-DELIVERY-001
acceptance_criteria:
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.1
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.2
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.3
  - AC-TASKS-SAVED-PROMPT-DELIVERY-001.7
system_design:
  - ../../specs/tasks/system-design/saved-prompt-delivery.md
---

# Task 02: Prove Quick Chat Saved-Prompt Delivery

## Summary

Add desktop and phone Playwright regressions for the first eager Quick Chat
request. Each test selects a saved prompt and observes behavior from its stored
definition.

## In scope

- Seed a disposable saved prompt through the existing API helper.
- Add a bounded mock-agent directive for trusted saved-prompt test context.
- Select the saved prompt through the shared Quick Chat composer.
- Assert a deterministic mock-agent result on desktop and phone.
- Clean up the saved prompt after each scenario.

## Out of scope

- New UI controls, copy, selectors, or responsive layout.
- Queue behavior and passthrough sessions.
- Visual snapshots that do not prove agent behavior.

## Acceptance

- The desktop test proves that the first eager Quick Chat request applies the
  selected saved definition.
- The phone test proves the same outcome through the existing full-screen Quick
  Chat surface and touch selection.
- A mock-agent unit test proves that visible or untrusted copies of the test
  directive do not activate it.
- Both focused projects discover and pass the new scenarios.

## Verification

```bash
cd apps
rtk pnpm install --frozen-lockfile
cd web
rtk pnpm e2e:run --project chromium tests/chat/quick-chat-saved-prompt-delivery.spec.ts
rtk pnpm e2e:run --project mobile-chrome tests/chat/mobile-quick-chat-saved-prompt-delivery.spec.ts
cd ../backend
rtk go test ./cmd/mock-agent -count=1
```

## Files likely touched

- `apps/web/e2e/tests/chat/quick-chat-saved-prompt-delivery.spec.ts`
- `apps/web/e2e/tests/chat/mobile-quick-chat-saved-prompt-delivery.spec.ts`
- `apps/web/e2e/tests/chat/quick-chat-saved-prompt-delivery-helpers.ts`
- `apps/backend/cmd/mock-agent/handler.go`
- `apps/backend/cmd/mock-agent/mock_agent_test.go`

## Dependencies

- Task 01: Canonicalize direct saved-prompt messages.

## Risks

- The mock directive must be bounded to the canonical expansion block. Visible
  text and untrusted prompt blocks must not activate it.
- The test must wait for eager session readiness before it selects the prompt.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-SAVED-PROMPT-DELIVERY-001`
- `docs/specs/tasks/system-design/saved-prompt-delivery.md`
- Existing Quick Chat helpers and mobile saved-prompt mention tests.

## Results

Implemented the bounded mock-agent directive and shared desktop/mobile Quick
Chat scenarios. Both scenarios select the saved prompt through the composer,
assert the current expansion in the stored message, and verify the
deterministic agent response. Cleanup removes the disposable prompt.

Verification:

- `cd apps/backend && rtk go test ./cmd/mock-agent -count=1` - passed as part of the focused five-package run.
- `cd apps/web && rtk pnpm exec tsc --noEmit --pretty false` - passed.
- `cd apps/web && rtk pnpm e2e:run --project chromium tests/chat/quick-chat-saved-prompt-delivery.spec.ts` - 1 passed.
- `cd apps/web && rtk pnpm e2e:run --no-build --project mobile-chrome tests/chat/mobile-quick-chat-saved-prompt-delivery.spec.ts` - 1 passed.
