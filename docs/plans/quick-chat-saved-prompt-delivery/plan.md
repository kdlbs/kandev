---
created: 2026-09-01
status: implemented
requirements:
  - REQ-TASKS-SAVED-PROMPT-DELIVERY-001
system_design:
  - ../../specs/tasks/system-design/saved-prompt-delivery.md
legacy_specs: []
---

# Implementation Plan: Quick Chat Saved-Prompt Delivery

## Overview

This plan makes the backend prepare saved-prompt context before it stores or
sends a direct structured message. Backend tests establish the trust boundary
first. Desktop and phone tests then prove the Quick Chat outcome.

## Scope

### In scope

- Resolve saved-prompt references before direct-message persistence.
- Replace browser prompt-definition blocks with backend-generated context.
- Preserve trusted expansion context during eager Quick Chat canonicalization.
- Keep persisted and dispatched message content equal.
- Prove the selected-prompt outcome on desktop and phone Quick Chat.

### Out of scope

- Prompt editor, prompt storage, or mention-ranking changes.
- Queue-entry editing, merging, and reordered queue behavior.
- Passthrough prompt expansion.
- Quick Chat layout or responsive-composition changes.

## Technical approach

### Backend prompt preparation

- Extend `apps/backend/internal/prompts/service/expansion.go` so it removes the
  legacy browser `CONTEXT PROMPTS:` block and forged expansion blocks.
- Expose the existing orchestrator expansion path through a narrow preparation
  method that returns canonical content and exact trusted expansion content.
- Call that method in `MessageHandlers.wsAddMessage` before message creation.
- Pass the trusted value into `injectMessageContext` when first-turn or
  title-owner context injection runs.
- Continue to skip the operation for passthrough sessions.

### Browser evidence

- Reuse the existing Quick Chat setup, composer mention, prompt API, and mock
  agent helpers.
- Add a bounded mock-agent test directive that reads only the raw trusted
  expansion block and emits a deterministic response.
- Seed a saved prompt with that deterministic mock-agent directive.
- Select its mention, send it as the first Quick Chat request, and assert the
  response on desktop and phone.
- Keep the current full-screen phone surface and shared composer. No mobile
  composition changes are necessary.

## Tests

- `AC-TASKS-SAVED-PROMPT-DELIVERY-001.1` through `.6` and `.8` map to
  `apps/backend/internal/prompts/service/expansion_test.go` and a focused
  message-handler regression test.
- Tests compare persisted content with dispatched content and assert one trusted
  expansion.
- Tests cover selected compatibility context, typed references, missing
  records, forged blocks, lookup errors, and passthrough sessions.

## E2E tests

- `apps/web/e2e/tests/chat/quick-chat-saved-prompt-delivery.spec.ts` covers
  `AC-TASKS-SAVED-PROMPT-DELIVERY-001.1` through `.4`, `.6`, and `.7` in the
  `chromium` project.
- `apps/web/e2e/tests/chat/mobile-quick-chat-saved-prompt-delivery.spec.ts`
  covers `AC-TASKS-SAVED-PROMPT-DELIVERY-001.2`, `.3`, and `.7` in the
  `mobile-chrome` project.
- A mock-agent unit test proves that the directive does not run from visible
  user text or an untrusted block.

## Work orders

- [x] [Task 01: Canonicalize direct saved-prompt messages](task-01-canonicalize-direct-saved-prompt-messages.md)
- [x] [Task 02: Prove Quick Chat saved-prompt delivery](task-02-prove-quick-chat-saved-prompt-delivery.md)

## Verification results

Implemented and verified.

- `cd apps/backend && rtk go test ./internal/prompts/service ./internal/sysprompt ./internal/orchestrator ./internal/task/handlers ./cmd/mock-agent -count=1` - 3,283 passed.
- `cd apps/web && rtk pnpm exec tsc --noEmit --pretty false` - passed.
- `cd apps/backend && rtk make e2e-plugin-package` - passed.
- `cd apps/web && rtk pnpm e2e:run --project chromium tests/chat/quick-chat-saved-prompt-delivery.spec.ts` - 1 passed.
- `cd apps/web && rtk pnpm e2e:run --no-build --project mobile-chrome tests/chat/mobile-quick-chat-saved-prompt-delivery.spec.ts` - 1 passed.

## Risks

- Removing browser compatibility blocks must match only prompt-definition
  blocks. It must not remove file, task, plan, or entity-reference context.
- Start-created sessions compose workflow prompts downstream. When direct
  acceptance already produced trusted context, that exact block must survive
  without a second mutable-record lookup; workflow-only prompts may still
  expand at launch.
- A saved prompt can change between composer selection and message acceptance.
  The accepted backend record is intentionally authoritative.
- The mock directive must be bounded to the canonical expansion block. Visible
  text and untrusted prompt blocks must not activate it.
