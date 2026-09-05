---
id: "01-suppress-invalid-message-timestamps"
title: "Suppress invalid message timestamps"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001
acceptance_criteria:
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.4
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.15
system_design:
  - ../../specs/ui/system-design/task-prompt-transcript-visibility.md
---

# Task 01: Suppress invalid message timestamps

## Summary

Make empty and malformed message timestamps render no timestamp affordance and
never expose the browser's `Invalid Date` string. Preserve the existing message
action row and valid timestamp behavior.

## In scope

- Add a failing formatter regression for empty and invalid values.
- Add a failing message-action regression for the synthetic task-description
  message shape (`created_at: ""`).
- Add the minimum guards in the shared formatter and `MessageTimestamp`.

## Out of scope

- Task/session persistence or backend message contracts.
- Changes to transcript pagination, fallback eligibility, responsive layout,
  touch targets, or action semantics for valid timestamps.

## Acceptance

- `formatRelativeTime` returns an empty string for empty, malformed, and
  normalized-malformed values such as `"0"` and February 30th.
- `MessageActions` renders no timestamp element or invalid-date disclosure for
  an empty or malformed `created_at` while copy/raw/favorite actions remain
  available.
- Valid timestamp tests continue to pass unchanged.

## Verification

```bash
(cd apps && pnpm --filter @kandev/web test -- --run lib/utils.test.ts components/task/chat/messages/message-actions.test.tsx)
(cd apps/web && pnpm exec eslint lib/utils.ts lib/utils.test.ts lib/utils/strict-timestamp.ts lib/state/slices/session/turn-actions.ts components/task/chat/messages/message-actions.tsx components/task/chat/messages/message-actions.test.tsx)
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/lib/utils.ts`
- `apps/web/lib/utils/strict-timestamp.ts`
- `apps/web/lib/utils.test.ts`
- `apps/web/lib/state/slices/session/turn-actions.ts`
- `apps/web/components/task/chat/messages/message-actions.tsx`
- `apps/web/components/task/chat/messages/message-actions.test.tsx`
- `docs/specs/ui/requirements/task-prompt-transcript-visibility.md`
- `docs/specs/ui/system-design/task-prompt-transcript-visibility.md`

## Dependencies

None.

## Risks

- Invalid persisted timestamps will be intentionally hidden rather than
  displayed as raw values, matching the fallback's lack of a trustworthy date.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.4` and `.15`.
- The fallback timestamp boundary in the transcript system design.
- Existing `MessageActions` and `formatRelativeTime` tests.

## Results

Implemented and verified:

- `parseStrictRfc3339Timestamp` is shared by session reconciliation and UI date formatting.
- `formatRelativeTime` returns an empty label for empty, malformed, and normalized-malformed dates.
- `MessageTimestamp` omits the timestamp element and coarse-pointer disclosure for invalid dates while preserving other message actions.
- Focused frontend tests pass with 51 tests; turn timestamp tests pass with 54 tests.
- ESLint, TypeScript typecheck, specification lint, and `git diff --check` pass.
- Managed Chromium and Pixel 5 capture specs pass against the production build; the captured fallback shows no `Invalid Date` text. Temporary capture specs were removed.
