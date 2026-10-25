---
created: 2026-08-31
status: completed
requirements:
  - REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001
system_design:
  - ../../specs/ui/system-design/task-prompt-transcript-visibility.md
legacy_specs: []
---

# Implementation plan: Text-aware transcript pagination

## Overview

Make one upward transcript load target 20 readable text parts instead of 20
arbitrary persisted rows. Extend the existing per-consumer accumulation path,
opt only the native transcript into it, then prove the behavior on desktop and
mobile with standalone tool activity that currently consumes a page and stops
the preload cycle.

## Scope

### In scope

- Count `message`, `content`, and legacy untyped rows toward the transcript's
  20-part lazy-load target.
- Keep tool calls and all other activity rows in the loaded transcript without
  letting them satisfy the text target.
- Preserve prompt `#1`, raw exhaustion, no-progress, and bounded-loop stops.
- Preserve Prompt History's user-prompt target and raw search/drain semantics.
- Add focused hook coverage and desktop/mobile browser regressions.

### Out of scope

- Backend pagination queries, cursors, page limits, persistence, or API fields.
- Initial newest-window size or eager history loading on task open.
- Message filtering, grouping, collapsing, or transcript virtualization.
- New controls, copy, layout, or mobile composition.
- Changing the prompt `#1` visible-start boundary or recovery behavior.

## Confirmed root cause

`useLazyLoadMessages` requests `OLDER_PAGE_LIMIT` (20) raw rows. The native
transcript calls the hook without an accumulation option, so `loadMore` returns
after the first page and `OlderPageResult.count` reports every prepended row,
including tool activity.

The committed-geometry sentinel can request another page only after that load
returns and React commits it. Standalone tool rows can move the sentinel out of
the preload region, so a page containing mostly or entirely tool calls consumes
one upward reach without delivering a useful text batch.

The smallest regression is two raw pages: the first contains 19 tool rows and
one text row; the second supplies the remaining text rows. The current
transcript load settles after page one. The corrected load continues until 20
new `message`/`content` rows arrive or a defined stop condition applies.

## Technical approach

### Text-aware hook accumulation

- Extend `LazyLoadMessagesOptions` in
  `apps/web/hooks/use-lazy-load-messages.ts` with a text-part target.
- Count only message types `message`, `content`, and absent legacy types.
- Reuse the current multi-page accumulation loop, joined-flight handling,
  prompt-`#1` boundary, zero-progress stop, and pages-per-load safety bound.
- Keep `loadMore`'s numeric result as total raw rows prepended so sentinel
  progress and recovery semantics do not change.
- Leave `loadMoreRaw` and callers without a target on their current single-page
  path. Leave Prompt History's `minUserPromptsPerLoad` behavior unchanged.

### Native transcript wiring

- Pass a 20-text-part target from
  `apps/web/components/task/chat/message-list-native.tsx` when constructing the
  native transcript loader.
- Keep the loading state active across the accumulated batch and restore the
  prepend anchor after each intermediate React commit.
- Keep sentinel continuation and recovery contracts unchanged.

### Browser regression

- Add a shared fixture in
  `apps/web/e2e/tests/chat/message-pagination-helpers.ts` with an older text
  batch behind one raw page of tool calls placed in separate turns. Separate
  turns keep the tool rows standalone so the current geometry rule stops after
  the first page.
- In desktop Chromium and mobile Chrome, perform one upward reach, assert more
  than one older-page request, assert the older text marker becomes visible,
  and retain the existing prepend-anchor tolerance.

## Tests

| Acceptance criterion | Test evidence |
| --- | --- |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.13` | `apps/web/hooks/use-lazy-load-messages.test.ts` proves mixed tool/text pages accumulate 20 text parts and that `message`, `content`, and legacy rows count while activity rows do not. |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.3`, `.7` | Hook cases preserve prompt `#1`, exhaustion, zero-progress, and safety-bound stops. |
| `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.9`, `.11`, `.13` | Existing desktop and mobile message-pagination specs gain the standalone-tool-page regression and anchor assertion. |

## E2E tests

- Desktop: `apps/web/e2e/tests/chat/message-pagination.spec.ts`, `chromium`
  project.
- Mobile: `apps/web/e2e/tests/chat/mobile-message-pagination.spec.ts`,
  `mobile-chrome` project.
- Shared cues: older-request count, older text marker, stable anchor-row top,
  and absence of the recovery control during successful loading.

## Mobile design contract

- Desktop outcome: one upward reach loads a useful text batch even when tool
  activity occupies the first raw page.
- Mobile entry point: existing Chat tab in the full-height task layout.
- Nearest exemplar: `apps/web/components/task/task-layout.tsx` and the current
  `mobile-message-pagination.spec.ts` flow.
- Hierarchy and surface: no composition change; transcript remains the only
  vertical scroll owner.
- Shared logic: desktop and mobile use the same store, text predicate, cursor
  coordinator, sentinel, and anchor restoration.
- Mobile proof: Pixel 5 `mobile-chrome` performs the same one-reach tool-heavy
  pagination scenario and observes the older text marker.

## Work orders

- [completed] [Task 01: Count transcript text during lazy loads](task-01-count-transcript-text.md)

## Verification results

- Focused Vitest: 50 tests passed across the lazy-load hook and native scroll
  management suites.
- TypeScript typecheck, changed-file ESLint, production E2E build, i18n
  new-code ratchet, and E2E sleep ratchet passed.
- Desktop Chromium message pagination: 7 tests passed.
- Mobile Chrome message pagination: 7 tests passed.
- Specification linter tests: 20 passed; all specification files passed.
- `git diff --check` passed.

## Risks

- Sequential pages increase request volume for tool-heavy histories; the
  existing per-load safety bound must remain effective.
- Joined requests can use another consumer's page size, so progress must come
  from the merged store delta rather than assumptions about response size.
- `content` and legacy untyped rows must count consistently without counting
  structured activity whose payload also contains text.
- Intermediate store commits must not break prepend anchoring or flash the
  recovery control between successful pages.
