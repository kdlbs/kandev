---
created: 2026-08-24
status: done
requirements:
  - REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001
system_design:
  - ../../specs/ui/system-design/task-prompt-transcript-visibility.md
legacy_specs: []
---

# Implementation Plan: Hide the Redundant Older-Messages Control

## Overview

The transcript will stop visible pagination when it loads the first user prompt. The fix will use the existing prompt ordinal and shared pagination hook.

The backend can report older rows after prompt `#1`. These rows contain internal session activity and do not precede the visible transcript start.

## Scope

### In scope

- Derive visible transcript pagination from raw `has_more` and the loaded prompt ordinals.
- Apply the boundary to automatic loading, transcript navigation, and the explicit older-page control.
- Keep raw backend pagination metadata in the store.
- Preserve an explicit raw-pagination path for reverse-search backfill, while keeping transcript consumers on visible pagination.
- Keep task opening bounded to the newest message window.
- Load older transcript pages only after upward navigation or an explicit recovery action.
- Extend desktop and mobile pagination coverage with hidden pre-prompt rows.
- Retain desktop and mobile prepend-anchor coverage for older-page loading.

### Out of scope

- Backend queries, API fields, message persistence, or schema changes.
- Changes to transcript ordering or message visibility filters.
- Removal of the older-page recovery control before prompt `#1` is known.
- New copy, translation keys, layout, navigation, or touch behavior.

## Confirmed root cause

`useLazyLoadMessages` returns `messages.metaBySession[sessionId].hasMore` without a visible-start check. The value remains true when internal rows exist before prompt `#1`.

The Prompt History panel already stops at prompt `#1`. The native transcript does not use that boundary.

A throwaway hook test loaded prompt `#1` with raw `has_more: true`. The test expected false but received true.

The focused run reported one failed regression and seven passing tests. The throwaway test was removed after the run.

## Technical approach

### Shared visible pagination

- Add one helper that detects a loaded user message with `prompt_index === 1`.
- Make `useLazyLoadMessages` return raw `hasMore` only while prompt `#1` is absent.
- Recalculate this value after joined and completed requests.
- Use the recalculated value in the multi-page accumulation loop.
- Keep `requestOlderMessages` and stored pagination metadata unchanged.
- Expose raw pagination separately for direct recovery consumers.

### Consumer alignment

- Keep the native transcript, transcript navigation, and drain hook on `useLazyLoadMessages`.
- Keep transcript navigation on visible pagination, and let session search use the raw loader when a backend hit is before prompt `#1`.
- Remove duplicate prompt-`#1` pagination logic from the Prompt History panel if the shared hook owns the complete rule.
- Keep request coordination shared between visible and raw loaders.

### Bounded task opening

- Remove background pagination that locates the last prompt when a task opens.
- Remove initial backfill for a tool-only newest window.
- Keep the task-description fallback as the readable start until upward navigation loads a prompt.
- Preserve a stable message-row position across each older-page request.

### Browser regression

- Seed more than one page of hidden setup rows before the first user prompt.
- Keep the existing long collapsed activity history after the prompt.
- Scroll to prompt `#1` and stop before the hidden rows load.
- Assert that the older-page control is absent on desktop and mobile.
- Preserve the existing prepend-anchor assertion in a separate desktop and mobile flow.

## Tests

| Acceptance criterion | Test evidence |
| --- | --- |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.4` | Existing native sentinel tests remain green. |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.5` | Desktop and mobile tests load collapsed older history after upward navigation. |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.6` | Desktop and mobile tests reach the first prompt without another action. |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.7` | The hook stops a multi-page load at prompt `#1`. |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.9` | The hook and browser tests show no visible older history at prompt `#1`. |

## E2E tests

- Desktop: `apps/web/e2e/tests/chat/message-pagination.spec.ts`.
- Mobile: `apps/web/e2e/tests/chat/mobile-message-pagination.spec.ts` in the `mobile-chrome` project.
- Shared seed: `apps/web/e2e/tests/chat/message-pagination-helpers.ts`.

The shared seed will add hidden pre-prompt rows. Both tests already assert that prompt `#1` is visible and the control is absent.

## Mobile design contract

- Desktop outcome: the transcript removes the older-page control at prompt `#1`.
- Mobile entry point: the existing Chat tab in the full-height task layout.
- Nearest exemplar: `apps/web/components/task/task-layout.tsx` and the current mobile pagination test.
- Hierarchy and surface: no composition change. The transcript remains the only vertical scroll owner.
- Shared logic: desktop and mobile use the same store, pagination hook, native transcript, and cursor coordinator.
- Mobile proof: the existing mobile pagination test will include hidden pre-prompt rows and assert the same result.

## Work orders

- [x] [Task 01: Stop visible pagination at the first prompt](task-01-stop-visible-pagination-at-first-prompt.md)
- [x] [Task 02: Stop eager history loading on chat open](task-02-stop-eager-history-loading.md)

## Verification results

- `pnpm --filter @kandev/web test -- hooks/use-lazy-load-messages.test.ts components/task/prompt-history-panel-content.test.tsx components/task/chat/use-drain-older-messages.test.ts` — 3 files, 73 tests passed.
- `pnpm --filter @kandev/web run typecheck` — passed.
- `pnpm --filter @kandev/web lint` — passed.
- `pnpm e2e:run --project chromium tests/chat/message-pagination.spec.ts` — 2 tests passed, including the visible-boundary and prepend-anchor flows.
- `pnpm e2e:run --project mobile-chrome tests/chat/mobile-message-pagination.spec.ts` — 2 tests passed, including the visible-boundary and prepend-anchor flows.
- `pnpm exec vitest run components/task/chat/message-list-shared.test.tsx hooks/domains/session/use-session-message-fetch.test.ts hooks/domains/session/use-session-messages.test.ts` — 3 files, 66 tests passed.
- `pnpm --filter @kandev/web run typecheck` — passed after the bounded-opening change.
- `pnpm --filter @kandev/web lint` — passed after the bounded-opening change.
- `pnpm --filter @kandev/web run i18n:ratchet` — passed.
- `pnpm --filter @kandev/web run e2e:sleep-ratchet` — passed.
- `pnpm e2e:run --host --no-build --project chromium -- tests/chat/message-pagination.spec.ts` — 3 tests passed.
- `pnpm e2e:run --host --no-build --project mobile-chrome -- tests/chat/mobile-message-pagination.spec.ts` — 3 tests passed.

## Risks

- The raw store value must remain available to low-level recovery code.
- A joined request must update the local hook state with the visible boundary.
- The multi-page load loop must stop without one extra pre-prompt request.
- Payloads without prompt ordinals must retain raw `has_more` behavior.
- The browser seed must create a stable page boundary around prompt `#1`.
