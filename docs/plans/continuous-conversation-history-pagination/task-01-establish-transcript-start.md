---
id: "01-establish-transcript-start"
title: "Establish the true transcript start"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001
acceptance_criteria:
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.1
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.2
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.3
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.4
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.10
system_design:
  - ../../specs/ui/system-design/task-prompt-transcript-visibility.md
---

# Task 01: Establish the true transcript start

## Summary

Record when session message history has authoritative pagination metadata and
use that state to prevent a bounded tool-only window from synthesizing the task
description. Preserve bounded opening and prompt-`#1` behavior.

## In scope

- Add and maintain per-session message-history initialization state.
- Expose initialized and raw-history state through the session hook.
- Gate task-description fallback on initialized, exhausted history.
- Preserve stored user prompts as authoritative.
- Add the failing tool-only-window regression before implementation.

## Out of scope

- Sentinel continuation and retry-control behavior.
- Backend message APIs or persistence.
- Eager history loading.

## Acceptance

- An initialized window with older history and no user row renders no synthetic
  task description.
- Exhausted legacy history with no user row renders one non-empty fallback.
- Uninitialized, refetching, prompt-`#1`, and empty-description cases do not
  produce a misleading or duplicate fallback.

## Verification

```bash
cd apps/web
pnpm exec vitest run \
  hooks/use-processed-messages-fallback.test.ts \
  hooks/domains/session/use-session-messages.test.ts \
  hooks/domains/session/use-session-message-fetch.test.ts \
  lib/state/slices/session/session-slice.merge-messages.test.ts \
  components/task/chat/message-list-shared.test.tsx
pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/state/slices/session/types.ts`
- `apps/web/lib/state/slices/session/session-slice.ts`
- `apps/web/lib/state/slices/session/session-slice.merge-messages.test.ts`
- `apps/web/lib/state/hydration/merge-strategies.ts`
- `apps/web/hooks/domains/session/use-session-messages.ts`
- `apps/web/hooks/domains/session/use-session-message-fetch.test.ts`
- `apps/web/components/task/chat/use-chat-panel-state.ts`
- `apps/web/hooks/use-processed-messages.ts`
- `apps/web/hooks/use-processed-messages-fallback.test.ts`
- `apps/web/components/task/chat/message-list-shared.test.tsx`

## Dependencies

None.

## Risks

- Boot-hydrated and client-fetched sessions must set the same initialization
  state.
- Live rows arriving before the newest snapshot must not claim that history is
  initialized or exhausted.
- Purged sessions must not retain metadata that affects a later session.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001` acceptance criteria `.1` to
  `.4` and `.10`.
- History state and opening boundary sections of the system design.
- Existing message-window reconciliation and processed-message fallback tests.

## Results

- Added per-session `historyInitialized` metadata with safe defaults, live-row
  isolation, newest-window fetch/boot hydration, and purge-compatible state.
- Gated the task-description fallback on initialized, exhausted history while
  preserving stored user prompts and the prompt-`#1` visible boundary.
- Added regressions for older history, uninitialized history, live inserts, and
  authoritative fetch metadata.
- The focused Task 01 Vitest suite passed (5 files, 87 tests).
- `pnpm run typecheck` passed.
