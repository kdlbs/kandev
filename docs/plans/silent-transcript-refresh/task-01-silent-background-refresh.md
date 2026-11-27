---
id: "01-silent-background-refresh"
title: "Refresh a visible transcript in the background without loading feedback"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002
acceptance_criteria:
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.5
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.11
system_design:
  - ../../specs/platform/system-design/session-subscription-recovery.md
---

# Task 01: Refresh a Visible Transcript in the Background Without Loading Feedback

## Summary

`doFetchMessages` accepts `background`. With messages already on screen, a background fetch
shows no loading row, no retrying notice, and does not raise the shared store loading flag.
The turn-end refresh and core conversation gap recovery pass `background: true`.

## Scope

- `apps/web/hooks/domains/session/use-session-message-fetch.ts`: silent start, separate total
  and visible in-flight counts, silent retry notice, visible failure.
- `apps/web/hooks/domains/session/use-session-messages.ts`: background call sites.
- `apps/backend/cmd/mock-agent`: `/with-usage` reports token usage in the prompt result, so
  E2E reproduces the post-turn metadata write that real agents trigger.

Out of scope: the backend revision bump without a published change after turn metadata is
patched. That is a separate backend fix.

## Acceptance conditions

1. A background refresh with messages on screen never inserts `session-history-loading` or
   `conversation-loading-state`.
2. A failed background refresh still reports `unavailable` with Retry.
3. Overlapping visible and background fetches clear visible loading when the last visible one
   settles.

## Verification

- `cd apps/web && pnpm exec vitest run hooks/domains/session`
- `cd apps/web && pnpm e2e:run tests/chat/turn-end-history-refresh.spec.ts`
- `cd apps/backend && go test ./cmd/mock-agent/`

## Results

Done. The E2E drops the first conversation change after the turn completes, so only the
periodic revision check finds the gap while the transcript is idle; it failed before the
store-flag change and passes after it.
