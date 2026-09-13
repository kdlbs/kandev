---
id: "01-entry-recovery"
title: "Recover entry requests"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002
  - REQ-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.1
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.2
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.3
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.4
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.5
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.6
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.7
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.10
system_design:
  - ../../specs/platform/system-design/session-subscription-recovery.md
---

# Task 01: Recover entry requests

## Summary

Implement bounded entry recovery and truthful history state in the existing client and hooks.
Preserve shared registration ownership, cached messages, and mutation admission.

## In scope

- Typed timeout errors and entry-specific deadlines.
- Shared subscription retry and independently bounded status/history reads.
- Hook recovery outcome and manual retry APIs for Task 02.
- Session/connection guards, cleanup, cache preservation, and permanent-error handling.

## Out of scope

Rendered UI, localization, backend changes, and automatic mutation retries.

## Acceptance

- A seven-second response succeeds; dropped responses retry within the design's limits without a visibility event.
- Failed history preserves cached messages; only a successful snapshot marks history initialized.
- Concurrent consumers share recovery; obsolete requests cannot act, and a successful status runs resume processing at most once.

## Verification

From the repository root, install dependencies once if this worktree lacks them:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

```bash
(cd apps/web && pnpm exec vitest run lib/ws/client.test.ts hooks/domains/session/use-session-messages.test.ts hooks/domains/session/use-session-subscription-retry.test.ts hooks/domains/session/use-session-resumption.test.ts hooks/domains/session/use-session-resumption.archive.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 lib/ws/client.ts lib/ws/request-error.ts hooks/domains/session/use-session-messages.ts hooks/domains/session/use-session-message-fetch.ts hooks/domains/session/use-message-fetch-state.ts hooks/domains/session/use-session-subscription-retry.ts hooks/domains/session/use-session-resumption.ts)
git diff --check
```

Add a behavioral RED named `recovers timed out registration without a visibility change` before implementation.
Use existing FakeWebSocket and hook test mocks; add fake-timer assertions for exhaustion and late responses.
Run any extracted new helper tests explicitly and include their paths in Results.

## Files likely touched

- `apps/web/lib/ws/client.ts`, `client.test.ts`, and `request-error.ts`
- `apps/web/hooks/domains/session/use-session-messages.ts` and its tests
- `apps/web/hooks/domains/session/use-session-message-fetch.ts`
- `apps/web/hooks/domains/session/use-message-fetch-state.ts`
- `apps/web/hooks/domains/session/use-session-subscription-retry.ts` and its tests
- `apps/web/hooks/domains/session/use-session-resumption.ts` and its tests

Paths without full directories above refer to the preceding explicit directory.

## Dependencies

None.

## Risks

Ref-count churn, duplicate mutation admission, timer leaks, and stale response writes.

## Parallelism

`sequential`

## Inputs

- Requirement AC-002.1 through AC-002.7 and AC-002.10; preserve all AC-001 criteria.
- System design: Registration ordering, Entry recovery, and History state.
- Existing client fake socket and session hook tests.

## Results

Implemented typed WebSocket timeout classification, the 10-second session-entry deadline, and
bounded shared subscription recovery. Status and history reads now retry typed timeouts within
the design budget, while permanent errors remain distinct. History retains cached rows until a
successful snapshot and exposes explicit recovery state for the presentation task.

Verification:

- `cd apps/web && pnpm exec vitest run lib/ws/client.test.ts hooks/domains/session/use-session-messages.test.ts hooks/domains/session/use-session-subscription-retry.test.ts hooks/domains/session/use-session-resumption.test.ts hooks/domains/session/use-session-resumption.archive.test.ts hooks/domains/session/use-session-message-fetch.test.ts components/task/chat/session-entry-feedback.test.tsx components/task/ensure-session-error.test.tsx components/task/chat/message-list-shared.test.tsx` — 170 tests passed across 9 files.
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps/web && pnpm exec eslint --max-warnings 0 lib/ws/client.ts lib/ws/request-error.ts hooks/domains/session/use-session-messages.ts hooks/domains/session/use-session-message-fetch.ts hooks/domains/session/use-message-fetch-state.ts hooks/domains/session/use-session-subscription-retry.ts hooks/domains/session/use-session-resumption.ts` — passed.
- `git diff --check` — passed.

The implementation preserves acknowledgement-before-history ordering, cancels obsolete retry
timers, and does not retry session mutations.

Review remediation (complete):

- Status request failures are classified before resume processing. Workspace-restore and launch
  failures keep their launch recovery action, while status responses with `error` or an archive
  outcome remain visible and never start a session.
- History feedback now carries a session and connection generation through subscription,
  terminal-state, entry, visibility, settle, running-backfill, and manual-retry fetches. Stale
  completions release the original session's store bookkeeping without changing the active hook
  state.
- Manual history Retry clears the settled hydration marker before fetching, so it always requests a
  fresh snapshot while preserving cached rows until that snapshot succeeds.
- Added regressions for workspace-restore failure classification, timeout-then-error status
  responses, A-to-B and A-to-B-to-A deferred history races, terminal-state fetches, and fresh
  manual history retry.
- Focused review-remediation validation: 170 tests passed across 9 files. It covers
  environment-scoped workspace-restore errors and launch retry behavior, permanent status error
  payloads after a timeout, generation-fenced terminal and manual history fetches, settled
  request-cache refreshes, and local loading cleanup for stale fetches. Typecheck, zero-warning
  ESLint, and formatting checks passed.
- Follow-up CI remediation keeps cached-entry refresh finalization on the session/connection
  generation. A live message that rerenders the entry while its refresh is pending can no longer
  strand the loading flag or suppress initial divider placement. The deferred regression passed,
  as did the desktop unread-divider suite (5 tests) and mobile unread-divider suite (3 tests).
