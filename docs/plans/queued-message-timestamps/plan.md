---
created: 2026-10-28
status: implemented
requirements:
  - REQ-UI-QUEUED-MESSAGE-TIMESTAMPS-001
system_design:
  - ../../specs/ui/system-design/queued-message-timestamps.md
legacy_specs: []
---

# Implementation Plan: Queued Message Timestamps

## Overview

One sequential work order exposes existing server timestamps in queue rows.
Task 01 is complete. The requirement is active and the system design is current.

## Scope

Include queue timestamps in task chat and Quick Chat, shared formatting,
mobile details, merge regression coverage, and a short public-doc update.
Exclude new backend fields, migrations, timers, and merge-policy changes.

## Technical approach

Extract the existing `MessageTimestamp` from
`apps/web/components/task/chat/messages/message-actions.tsx` into
`messages/message-timestamp.tsx`. Render it from `DisplayView` in
`queued-ghost-message.tsx` with `entry.queued_at`.
Use `formatMessageTime`, the existing preference, and existing localized labels.
The backend already stores the newest contributing time for both merge paths.
`useQueuePanelEscape` must defer to the open timestamp drawer so Escape
dismisses only that drawer. The mobile browser test exposed this integration
case, and a focused hook test covers it.

## UI-01: Expanded queue row

Entry point: queue chip in task chat or Quick Chat. State: pending message.

The row retains its position and sender cue on the left, content in the center,
and existing actions on the right. The timestamp occupies a metadata line
below the preview and attachments. An attachment-only row uses the same order.
The required structure is identical on desktop and phone. Phone labels wrap
inside the content column, and tapping a timestamp opens the existing details
drawer. Invalid timestamps omit the metadata line.

Illustrative timestamp: messages queued at 14:32 and 14:36 merge into a row
showing 14:36 in an absolute display mode. Actual formatting includes the date
and follows the user's saved preference.

No ASCII drawing is included because the current task instructions require
Kandev rich output for previews. The conversation preview uses that renderer.
This structural description remains the durable implementation reference.

## Tests

| Acceptance | Evidence |
| --- | --- |
| .1, .3, .4 | New `queued-ghost-message-timestamp.test.tsx`: all sender kinds, attachment-only content, three display modes, locale, invalid and Date-normalized malformed input |
| .2 | Component rerender with a new server timestamp, existing repository merge tests, browser merge and reload flows |
| .5 | Touch drawer component tests, mobile browser target and containment measurements |

Run existing message-actions and queue-row tests to protect extraction,
preview expansion, and queue controls.

## E2E tests

Add `e2e/tests/chat/queued-message-timestamps.spec.ts` for `chromium`.
Cover a single pending row, manual merge, repeated merge after reordering,
automatic merge, reload, and a Quick Chat smoke case (.1-.3).
Ensure an older source merges into a newer target at least once.
Use existing queue helpers and isolated fixture settings for merge policies.

Add `e2e/tests/chat/mobile-queued-message-timestamps.spec.ts` for
`mobile-chrome`. Cover visible time, manual merge, timestamp tap, drawer
content and dismissal, and long absolute labels (.1-.3, .5).
Measure trigger size, containment, and document overflow at the configured
phone viewport and at 390 pixels. Capture the rendered queue and drawer.

## Work orders

- [x] [Task 01: Render queue timestamps](task-01-render-queue-timestamps.md)

## Verification results

Implementation checks passed: 181 focused tests, four desktop browser cases,
three mobile browser cases, typecheck, zero-warning lint, localization,
public-doc validation, and specification gates. See the
[work-order results](task-01-render-queue-timestamps.md#results) for commands,
logs, environment adjustments, and visual evidence.

Design checks on 2026-10-28:

- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: passed for the proposed
  queue component change and this package. This was a local preflight with
  an explicit planned runtime path, not a claim that implementation exists.

## Risks

Long localized dates can crowd queue controls. Place them inside the content
column and test actual phone geometry. Older persisted merges may lack the
newest source time. Do not invent missing history.

## Documentation impact

Updated the queue reference in `docs/public/operations.md`. It explains that
queue timestamps follow the message-time setting and show the latest
contributing time after a merge.
