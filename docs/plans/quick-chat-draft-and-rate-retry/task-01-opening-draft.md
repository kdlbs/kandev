---
id: "01-opening-draft"
title: "Preserve opening prompt text"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-QUICK-CHAT-COMPOSER-001
acceptance_criteria:
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.9
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.10
system_design:
  - ../../specs/tasks/system-design/quick-chat-opening-composer.md
---

# Task 01: Preserve opening prompt text

## Summary

Restore multiline text without adding hard breaks. Clear only the accepted
opening snapshot, including rich content and attachments.

## In scope

Use structured plain-text conversion in editor synchronization. Add
`preserves blank paragraphs during draft restoration` to `tiptap-helpers.test.ts`.
Add mounted-editor restoration and accepted-payload regressions to
`chat-input-draft-multiline-restoration.test.tsx`.
Require red evidence before changing production code.

Cover multiple blank lines, literal HTML, code, saved-prompt tokens, and existing
rich-document restoration. Preserve a newer text/attachment draft and a switched
session. After acceptance, a provider failure must not restore the old input.

## Out of scope

New layouts, unconditional clearing, whitespace normalization, storage migrations.

## Acceptance

- Plain text survives restoration and serialization exactly.
- Accepted payload clears; newer or foreign drafts and attachments survive.
- Desktop and phone send one message and show an empty composer after reload.

## ASCII UI preview

UI-01: Accepted Quick Chat opening message. Shared content order on desktop
and phone; desktop uses the existing dialog, phone the existing full-height chat.

```text
Before: transcript [opening message]    composer [opening message]
After:  transcript [opening message]    composer [empty, or newer draft]
Retry:  transcript [opening message]
        [Rate limit | attempt 1/5 | countdown | Cancel]
        composer [empty, or newer draft]
```

The transcript scrolls; the existing composer remains at the bottom. Keep phone
safe-area handling and touch targets. Reuse existing localized recovery copy.
The empty accepted draft and single retry notice are required; wording above is
illustrative. No new layout, navigation, picker, or control is proposed.

Full view: [UI-01](plan.md#ascii-ui-preview). Covers composer .9 and .10.

## Verification

Run from the repository root. The managed E2E runner rebuilds the application.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/chat/tiptap-helpers.test.ts components/task/chat/chat-input-draft-ownership.test.tsx components/task/chat/chat-input-draft-multiline-restoration.test.tsx components/task/chat/use-chat-input-state-accepted-payload.test.ts components/quick-chat/use-quick-chat-initial-prompt.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/quick-chat-opening-composer.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-quick-chat-opening-composer.spec.ts)
```

## Files likely touched

- `apps/web/components/task/chat/use-tiptap-editor.ts`
- `apps/web/components/task/chat/tiptap-helpers.test.ts`
- `apps/web/components/task/chat/chat-input-draft-multiline-restoration.test.tsx`
- Both opening-composer E2E files and their shared helper

## Dependencies

None.

## Risks

Shared editor conversion must preserve rich content and token semantics.

## Parallelism

`sequential`

## Inputs

Read the requirement and design documents named in frontmatter, the matching
sections of plan.md, existing source, and adjacent test patterns.

## Results

- Red: the mounted editor test serialized the restored blank paragraph as three
  newlines, and desktop/mobile E2E both found the accepted prompt still in the
  composer.
- Green: the original focused Vitest command passed (4 files, 75 tests). After
  moving the mounted-editor cases to a dedicated multiline-restoration file,
  the draft-ownership and restoration files passed 29 tests.
- Green: `node --max-old-space-size=8192 ./node_modules/typescript/bin/tsc --noEmit`
  passed. The default `pnpm run typecheck` exceeded Node's 4 GB heap.
- Green: desktop Quick Chat opening E2E passed (2 tests); phone E2E passed (1
  test). Both verified exact multiline delivery, empty composer, and one user
  message after reload. The managed runner rebuilt the production app.
- Review follow-up: after reload, the shared helper reopens the accepted Quick
  Chat tab and verifies the exact saved user message and empty composer. From
  `apps/web`, `pnpm e2e:run --project chromium
  tests/chat/quick-chat-opening-composer.spec.ts` passed (2 tests), and
  `pnpm e2e:run --project mobile-chrome
  tests/chat/mobile-quick-chat-opening-composer.spec.ts` passed (1 test). The
  updated focused Vitest command in Verification passed 5 files and 75 tests;
  `node --max-old-space-size=8192 ./node_modules/typescript/bin/tsc --noEmit`
  and `pnpm exec eslint
  e2e/tests/chat/quick-chat-opening-composer-helpers.ts` also passed.
- PR CI follow-up: CI replay exposed two file-tree tests that expected Changes
  to auto-activate in a Files/Changes/Todos layout, where the layout guard
  intentionally blocks automatic activation. The tests now select Changes
  explicitly before checking its panel. The file-tree spec passed 3 tests with
  retries disabled. The duration-aware CI shards passed under a 4-process,
  4-CPU, 12-GB cap with retries disabled: shard 27 passed 139 tests and shard
  23 passed 133 tests. The original mobile prompt toggle failure did not recur
  in either exact shard replay.
- Follow-up PR CI run flagged a flaky file-tree tab switch in shard 13 and a
  mobile Stats retry test in shard 21. The file-tree test now waits for both
  rename entries to reach Changes before returning to Files. The Stats test
  now uses a 300-second Retry-After so its automatic recovery cannot remove
  the manual retry control during the touch-target assertion. Archived shard
  replays passed with retries disabled in the CI runtime image, four processes,
  four CPUs, and 12 GB: shard 13 passed 155 tests; shard 21 passed 138 tests
  with 8 skipped. After these test adjustments, the file-tree spec passed all
  3 tests and the mobile Stats test passed, both with retries disabled.
- The rendered structure matches UI-01; no layout changes were needed.
