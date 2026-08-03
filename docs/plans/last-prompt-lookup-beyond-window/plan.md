---
spec: docs/specs/last-prompt-pinning-regressions/spec.md
created: 2026-07-31
status: completed
---

# Implementation Plan: Last-prompt affordances when the prompt is beyond the loaded window

## Overview

Sessions whose last user prompt is older than the initially loaded transcript window never show the scroll-to-last-prompt control or the anchored last-prompt bar, even though the prompt is in the transcript above the loaded window. Replace the bounded background lookup with a single targeted query for the session's last user message, resolve the affordances from it, and drain older pages on demand when the user clicks.

## Root cause

`apps/web/components/task/task-chat-panel.tsx` derives `lastPromptMessageId` only from the loaded message window (`getLastUserMessageId(allMessages)`). When the newest ~100 messages contain no user message, a background lookup paginates older pages but caps at `MAX_LAST_PROMPT_LOOKUP_PAGES = 3` (3 × 20 = 60 messages past the initial fetch). A session whose last user prompt is older than ~160 messages — e.g. an autonomous session whose only user prompt is its task description — never resolves it, so `showScrollButton` is `false` and the anchored bar never renders. `useTranscriptEdgeTracking` reports `"visible"` when the target row is not mounted, keeping the controls suppressed.

Smallest reliable reproduction: seed an autonomous session with 200 agent messages after its task-description user message, reload the task page, and assert the scroll-to-last-prompt button / anchored bar appear. Fails today (`apps/web/e2e/tests/chat/last-prompt-multi-tab.spec.ts`, "autonomous session …" test).

## Approach

Add an `author_type` filter to the existing paginated messages endpoint, fetch the last user message directly (one request) instead of draining pages, and treat a resolved-but-not-rendered last prompt as edge `"above"` so the affordances render immediately. Clicking jumps by draining older pages until the prompt is mounted, then scrolling — the same pattern scroll-to-start already uses.

- Backend: `author_type` query filter on `GET /api/v1/task-sessions/:id/messages` (also the WS list path via the shared `service.ListMessagesPaginated`), threaded through `models.ListMessagesOptions`, `service.ListMessagesRequest`, and `buildListMessagesQuery`.
- Frontend: `useLastUserMessage` hook that prefers the window-derived last user message (always freshest) and otherwise issues one filtered fetch `{ limit: 1, author_type: "user", sort: "desc" }`.
- Edge resolution: while the last prompt is resolved but not mounted in the loaded window and `hasMore` is true, its position is deterministically above the viewport → effective edge `"above"`. Once mounted, the renderer's tracked edge takes over.
- Click behavior: if the prompt is not mounted, set a pending state that drains older pages (`useDrainOlderMessages`) and scrolls once loaded; if mounted, scroll immediately.

## Files

### Backend (`apps/backend`)

- `internal/task/models/models.go` — add `AuthorType string` to `ListMessagesOptions`.
- `internal/task/repository/sqlite/message.go` — add `AND author_type = ?` to `buildListMessagesQuery` when set.
- `internal/task/repository/interface.go` — unchanged (method signature stable).
- `internal/task/service/service_requests.go` — add `AuthorType string` to `ListMessagesRequest`.
- `internal/task/service/service_messages.go` — pass `AuthorType` through to `ListMessagesOptions`.
- `internal/task/handlers/message_handlers.go` — parse and validate `author_type` (`user`/`agent`) in `listMessagesParams`, pass through.

### Frontend (`apps/web`)

- `lib/api/domains/session-api.ts` — add `author_type?: string` to `listTaskSessionMessages` params.
- `hooks/domains/session/use-last-user-message.ts` (new) — window-derived first, else one filtered fetch; refetch per session; prefer window-derived once present. Unit test alongside.
- `components/task/task-chat-panel.tsx` — replace `lastPromptMessageId`/`lastPromptMessage` memos and the bounded lookup effect with the hook; compute effective edge (`"above"` when resolved-but-not-mounted with `hasMore`); add pending-scroll drain for scroll-to-last-prompt.
- `components/task/chat/message-list-shared.tsx` — add `resolveEffectiveLastPromptEdge` helper (pure); remove the now-dead `shouldLoadMoreForTranscriptTarget`.
- `components/task/chat/message-list-shared.test.tsx` — cover `resolveEffectiveLastPromptEdge`; drop `shouldLoadMoreForTranscriptTarget` cases.
- `components/task/chat/use-drain-older-messages.ts` — update the stale comment reference to the removed last-prompt preload effect.

## Tests

- **Backend filter:** repository test asserting `author_type=user` returns only user messages (newest-first), `author_type=agent` the reverse, empty = unfiltered; handler test asserting invalid `author_type` → 400 and valid pass-through. Files: `internal/task/repository/message_repository_test.go`, `internal/task/handlers/message_handlers_test.go` (or existing test files for these packages).
- **Hook:** `apps/web/hooks/domains/session/use-last-user-message.test.ts` — window-derived wins when a user message is loaded; filtered fetch fires when none is; fetch result used as fallback; switching sessions refetches.
- **Edge helper:** `apps/web/components/task/chat/message-list-shared.test.tsx` — `resolveEffectiveLastPromptEdge` truth table (mounted / not-mounted × hasMore).
- **E2E (red before, green after):** `apps/web/e2e/tests/chat/last-prompt-multi-tab.spec.ts` "autonomous session …" test asserts the button + open anchored bar after reload of a session with 200 seeded agent messages; extend it to click the scroll button and assert the transcript lands at the task-description prompt.

## Verification commands

- `cd apps/backend && PATH=/tmp/go/bin:$PATH make test`
- `cd apps/backend && PATH=/tmp/go/bin:$PATH make lint`
- `cd apps/web && pnpm exec vitest run components/task/chat/message-list-shared.test.tsx hooks/domains/session/use-last-user-message.test.ts`
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm run lint`
- `cd apps/web && KANDEV_SERVER_HOST= pnpm e2e --project=chromium tests/chat/last-prompt-multi-tab.spec.ts`

## Implementation waves

1. [task-01-backend-author-type-filter](task-01-backend-author-type-filter.md) — sequential. Backend query filter + tests.
2. [task-02-frontend-targeted-lookup](task-02-frontend-targeted-lookup.md) — sequential; depends on task 01 (needs the filter). Hook + edge helper + panel wiring + unit tests.
3. [task-03-e2e-verification](task-03-e2e-verification.md) — sequential; depends on tasks 01–02. E2E red→green + full verification.
