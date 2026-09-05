---
spec: docs/specs/tasks/requirements/clarification-response-reliability.md
related_specs:
  - docs/specs/tasks/system-design/clarification-response-reliability.md
  - docs/specs/tasks/requirements/clarification-active-lifecycle.md
created: 2026-09-05
status: implemented
---

# Implementation Plan: Clarification response reliability

## Overview

Make answer and Skip requests independent of unrelated transcript size. Bound
the complete browser wait and preserve a safe retry path. The change adds one
pending-ID-leading expression index for SQLite and PostgreSQL. It extends the
five-second claim budget to all pre-claim work. It maps that timeout to a
retryable 503. The shared web hook gets a 40-second deadline.

The plan preserves current-turn authority, detached-resume behavior, durable
delivery intents, and the existing idempotent loser reconstruction. It does not
add a clarification table, rewrite message rows, or expand PostgreSQL support
to multiple backend replicas.

## Confirmed cause

- The affected SQLite database held about 904,000 message rows and was 4.44 GB.
- `FindMessagesByPendingID` and the bundle claim filter by pending ID without a
  session ID.
- `idx_messages_metadata_pending_id` is ordered by session ID before the JSON
  pending-ID expression, so SQLite cannot use it for that query shape.
- The live query plan was a full `task_session_messages` scan. Submit and Skip
  requests took 37 to 55 seconds and several ended with
  `claim active clarification bundle: context deadline exceeded`.
- Only the atomic claim currently receives the five-second claim context. The
  preceding identity lookup uses the unbounded request context.
- The web hook awaits `fetch` without an abort deadline, so a slow backend or
  proxy leaves the controls in `submitting` indefinitely.

## Design decisions

### Persistence access path

- Add `idx_messages_metadata_pending_id_lookup` with extracted pending ID
  first, followed by `created_at` and `id`, and restrict it to non-null pending
  IDs.
- Generate the expression and DDL through `internal/db/dialect`. Query and
  index expressions must remain textually aligned for both database drivers.
- Retain the existing session-first index for session-scoped consumers.
- Use the current startup-critical `CREATE INDEX IF NOT EXISTS` path. A new
  index name upgrades existing installations and is replay-safe.
- Use real SQLite and environment-gated PostgreSQL query-plan tests. PostgreSQL
  remains under the documented one-replica, stop-the-writer upgrade contract.
  this package does not introduce concurrent index management.

### Bounded server result

- Apply one fresh five-second context to identity resolution, authorization
  inputs, validation reads, and the atomic claim.
- Convert exhaustion of that internal pre-claim budget into a typed resolver
  error and HTTP 503 with code `temporarily_unavailable`.
- Preserve the existing post-claim 30-second delivery/recovery budget and all
  durable compensation rules.
- Add low-cardinality timeout counters by phase and structured phase-duration
  logs without placing pending IDs in metrics.

### Desktop and phone recovery

- Give `postClarification` its own 40-second `AbortController` deadline and
  clear the timer on every path.
- Reuse the existing error banner and idempotent Retry action. Preserve answers
  and release the in-flight guard after timeout, network failure, 503, or 5xx.
- Keep collapse/dismiss local and Skip as the explicit rejection. Both become
  available again after a retryable failure.
- Share the same hook, state, and banner across desktop and phone. Preserve the
  existing 44-pixel phone Retry target and add phone outcome coverage rather
  than a separate mobile component.

## Work order

### Wave 1: Indexed lookup foundation

- [x] [Task 01: Index pending-ID bundle access](task-01-index-pending-id-bundle-access.md)

### Wave 2: Bounded backend contract

- [x] [Task 02: Bound clarification response resolution](task-02-bound-clarification-response-resolution.md) - depends on Task 01.

### Wave 3: Client recovery and viewport coverage

- [x] [Task 03: Bound and recover clarification submission](task-03-bound-and-recover-clarification-submission.md) - depends on Task 02.

Implementation remains sequential in the primary conversation unless the user
explicitly authorizes implementation sessions.

## Verification strategy

- SQLite schema and query-plan tests prove fresh creation, replay, upgrade, and
  index use with substantial unrelated message history.
- Environment-gated real PostgreSQL tests prove the native expression index,
  replay, pending-ID query eligibility, and response semantics.
- Resolver and handler tests use controlled blocking repositories and fake
  clocks/channels rather than wall-clock sleeps. They prove the internal
  timeout classification, no-delivery boundary, 503 envelope, and unchanged
  post-claim outcomes.
- Hook tests use fake timers and a non-resolving fetch to prove the 40-second
  deadline, timer cleanup, answer preservation, mutex release, and safe retry.
- Desktop and Pixel 5 Playwright scenarios inject a retryable backend result,
  prove that controls recover and complete the same saved answer after Retry.

## Risks and mitigations

- **Index build time and disk headroom:** A large existing message table may
  lengthen the first startup. Keep the migration startup-critical, retain the
  current operator backup/maintenance requirements, and prove the new partial
  index against a representative large SQLite database before release.
- **PostgreSQL planner variation:** Do not assert a universal plan for tiny
  tables. Seed enough unrelated rows, run `ANALYZE`, and prove the expression
  index is eligible in the real PostgreSQL suite.
- **Client deadline drift:** Keep the browser deadline at 40 seconds. Pin the
  browser and server constants in focused tests.
- **Duplicate delivery after an ambiguous response:** Reuse the existing durable
  claim, delivery intent, and winner reconstruction. Do not add client-side
  optimistic success for timeouts or 503 responses.

## Package validation commands

- `cd apps/backend && go test ./internal/db/dialect ./internal/task/repository/sqlite ./internal/clarification -count=1`
- `cd apps/backend && go test -race ./internal/clarification -count=1`
- `cd apps/backend && KANDEV_TEST_POSTGRES_DSN="$KANDEV_TEST_POSTGRES_DSN" go test ./internal/task/repository/sqlite ./internal/clarification -run 'Postgres.*Clarification|Clarification.*Postgres|PendingIDLookup' -count=1`
- `cd apps && pnpm --filter @kandev/web test -- --run hooks/domains/session/use-clarification-group.test.ts hooks/domains/session/use-clarification-group.timeout.test.ts hooks/domains/session/use-clarification-group.regressions.test.ts components/task/chat/clarification-input-overlay.test.tsx`
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm run lint`
- `cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet`
- `cd apps/web && pnpm e2e:raw --project=chromium tests/chat/clarification-submit-failure.spec.ts`
- `cd apps/web && pnpm e2e:raw --project=mobile-chrome tests/chat/mobile-clarification.spec.ts`

## Results

Implemented across the SQLite/PostgreSQL persistence layer, clarification
resolver and HTTP handler, and shared desktop/mobile web hook. Verification
includes focused and race-enabled backend tests, frontend unit/type/lint/
localization checks, and the managed Chromium (2 passed) and Pixel 5 (8
passed) E2E scenarios. The PostgreSQL planner test is environment-gated and
was skipped because no `KANDEV_TEST_POSTGRES_DSN` was available.
