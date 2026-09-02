---
id: "01-separate-composer-readiness-gates"
title: "Separate composer readiness gates"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SESSION-START-COMPOSER-READINESS-001
acceptance_criteria:
  - AC-UI-SESSION-START-COMPOSER-READINESS-001.1
  - AC-UI-SESSION-START-COMPOSER-READINESS-001.2
  - AC-UI-SESSION-START-COMPOSER-READINESS-001.3
  - AC-UI-SESSION-START-COMPOSER-READINESS-001.4
  - AC-UI-SESSION-START-COMPOSER-READINESS-001.5
  - AC-UI-SESSION-START-COMPOSER-READINESS-001.6
system_design:
  - ../../specs/ui/system-design/session-start-composer-readiness.md
---

# Task 01: Separate Composer Readiness Gates

## Summary

Keep the chat editor editable while a session starts or resumes. Keep every
regular submission path blocked until the session becomes ready.

## In scope

- Separate editor and submission gates in `useChatInputContainer`.
- Preserve the clarification exception and environment-prepare reason.
- Add a hook regression test for the state split.
- Add session-recovery E2E evidence for draft entry and preservation.

## Out of scope

- Change backend message admission or queue behavior.
- Change non-startup editor gates.
- Change composer layout, copy, or mobile composition.

## Acceptance

- During startup, the editor accepts text and all regular submit paths remain
  blocked.
- When the session becomes ready, the draft remains and submission becomes
  available.
- When startup fails, the draft remains and the existing recovery gate applies.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- --run components/task/chat/use-chat-input-container.test.ts components/task/chat/chat-input-body.test.tsx
cd apps/web && pnpm e2e:run tests/session/session-recovery.spec.ts -- --grep "resume session recovers"
```

## Files likely touched

- `apps/web/components/task/chat/use-chat-input-container.ts`
- `apps/web/components/task/chat/use-chat-input-container.test.ts`
- `apps/web/components/task/chat/chat-input-body.test.tsx`
- `apps/web/e2e/tests/session/session-recovery.spec.ts`

## Dependencies

None.

## Risks

- The keyboard or plugin submit path can diverge from the toolbar gate.
- A session identity change can replace the stored draft during startup.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-SESSION-START-COMPOSER-READINESS-001`.
- The session-start composer readiness system design.
- Existing composer state and session-recovery test patterns.

## Results

- Split startup submission readiness from editor readiness in
  `useChatInputContainer` while preserving the clarification exception and
  environment-prepare disabled reason.
- Added a hook regression test that proves startup keeps the editor enabled and
  regular submission disabled.
- Extended the resume recovery E2E flow to type during startup, retain the
  draft, enable submission at readiness, and submit the retained draft.
- Verification passed: locked dependency install, 19 focused Vitest tests, one
  production-build Chromium E2E test, targeted ESLint, specification lint, and
  `git diff --check`.
