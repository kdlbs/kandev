---
id: "02-stop-eager-history-loading"
title: "Stop eager history loading on chat open"
status: done
wave: 2
depends_on:
  - "01-stop-visible-pagination-at-first-prompt"
plan: "plan.md"
requirements:
  - REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001
acceptance_criteria:
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.4
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.5
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.6
system_design:
  - ../../specs/ui/system-design/task-prompt-transcript-visibility.md
---

# Task 02: Stop Eager History Loading on Chat Open

## Summary

Keep chat opening bounded to the newest message window. Older pages load only
after the user navigates upward or starts an explicit recovery action.

## Confirmed root cause

`TaskChatPanel` starts as many as three 20-row pagination requests merely to
locate the last prompt. The uncached initial-fetch path can separately prepend
as many as ten 100-row pages when the newest window contains only tool activity.
Every page immediately changes the rendered transcript and scroll geometry.

An isolated 451-row browser reproduction issued three unsolicited older-page
requests and grew the transcript from 1,807px to 3,407px while opening.

## In scope

- Remove background last-prompt pagination from task opening.
- Remove initial tool-only history backfill.
- Preserve scroll-triggered pagination and explicit search recovery.
- Add desktop and mobile regressions proving that task opening requests no
  older page.

## Out of scope

- Backend pagination, persistence, schema, or API changes.
- Transcript virtualization.
- Changes to explicit upward pagination, search recovery, or scroll-to-start.
- New copy, layout, navigation, or touch behavior.

## Acceptance

- Opening a task renders only the newest message window.
- Opening a task does not request a page with a `before` cursor.
- Older messages remain reachable by scrolling upward on desktop and mobile.
- A tool-only initial window does not keep initial loading active while older
  pages are fetched.

## Verification

```bash
cd apps
pnpm --filter @kandev/web test -- \
  hooks/domains/session/use-session-message-fetch.test.ts \
  hooks/domains/session/use-session-messages.test.ts \
  components/task/chat/message-list-shared.test.tsx
pnpm --filter @kandev/web run typecheck
pnpm --filter @kandev/web lint
```

```bash
cd apps/web
pnpm e2e:run --project chromium tests/chat/message-pagination.spec.ts
pnpm e2e:run --project mobile-chrome tests/chat/mobile-message-pagination.spec.ts
```

## Files likely touched

- `apps/web/components/task/task-chat-panel.tsx`
- `apps/web/components/task/chat/message-list-shared.tsx`
- `apps/web/hooks/domains/session/use-session-message-fetch.ts`
- `apps/web/hooks/domains/session/use-session-messages.ts`
- `apps/web/hooks/domains/session/message-backfill.ts`
- `apps/web/hooks/domains/session/use-session-message-fetch.test.ts`
- `apps/web/hooks/domains/session/use-session-messages.test.ts`
- `apps/web/e2e/tests/chat/message-pagination-helpers.ts`
- `apps/web/e2e/tests/chat/message-pagination.spec.ts`
- `apps/web/e2e/tests/chat/mobile-message-pagination.spec.ts`

## Mobile design contract

- Desktop outcome: chat opens on the newest bounded window without background
  older-page requests.
- Mobile entry point: the existing Chat tab in the full-height task layout.
- Nearest exemplar: `apps/web/components/task/task-layout.tsx` and the existing
  mobile pagination test.
- Hierarchy and surface: unchanged. The native transcript remains the single
  vertical scroll owner.
- Shared logic: desktop and mobile use the same task panel, message hook, and
  pagination coordinator.
- Mobile proof: the mobile pagination spec opens tool-heavy history, proves no
  eager request, then confirms upward navigation still loads older history.

## Risks

- Last-prompt controls remain unavailable until a user prompt enters the loaded
  window.
- The task-description fallback must remain the readable anchor for a tool-only
  newest window.
- Removing initial backfill must not block explicit pagination after loading
  settles.

## Parallelism

`sequential`

## Results

- Removed task-open pagination for last-prompt discovery.
- Removed the tool-only initial backfill path and its unused helper.
- Added a stable row anchor for complete older-page request cycles.
- Added desktop and mobile coverage that observes zero older-page requests during task opening.
- `pnpm exec vitest run components/task/chat/message-list-shared.test.tsx hooks/domains/session/use-session-message-fetch.test.ts hooks/domains/session/use-session-messages.test.ts` passed 66 tests.
- `pnpm run typecheck` passed.
- `pnpm e2e:run --host --no-build --project chromium -- tests/chat/message-pagination.spec.ts` passed 3 tests.
- `pnpm e2e:run --host --no-build --project mobile-chrome -- tests/chat/mobile-message-pagination.spec.ts` passed 3 tests.
