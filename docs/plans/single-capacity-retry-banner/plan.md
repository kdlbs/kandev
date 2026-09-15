---
created: 2026-09-15
status: implemented
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
system_design:
  - ../../specs/platform/system-design/provider-error-recovery.md
legacy_specs: []
---

# Implementation Plan: Single Capacity Retry Banner

## Overview

Reuse one persisted retry notice and update it through successive attempts.
One sequential work order covers persistence, event delivery, and rendered proof.
Platform owns this outcome because it owns the interactive retry lifecycle.

## Evidence and root cause

The supplied screenshot shows attempt 1 and attempt 2 together.
`createTransientRetryStatusMessage` always calls `CreateSessionMessage`.
`nextTransientAttempt` cancels the timer but does not remove the prior message.
`TestResetTransientRetry_ResolvesPersistedNotices` explicitly expects two rows
before cleanup. This source trace explains the screenshot without a runtime launch.
The linked task's filtered history returned no matching messages.

Minimal reproduction: send `/overloaded:9` in an isolated mock session and wait
for attempt 2. Current E2E checks both attempt labels but never asserts one card.

## Scope

### In scope

- Update one message across attempts, including its deadline and Cancel metadata.
- Consolidate legacy duplicates on the next successful notice write.
- Preserve cleanup, authorization, retry policy, and session isolation.
- Prove same-message updates through persistence, WebSocket delivery, and reload.

### Out of scope

- Generic no-output warnings and resumed-agent transcript entries.
- Provider classification, retry limits, scheduling policy, and Office routing.
- A new layout, retry component, or public API.

## Technical approach

Extend `TransientRetryMessageService` with existing task-service `UpdateMessage`.
Keep task-service mutation and event publication as the single persistence path.
Use the selection and error rules in the
[interactive lifecycle design](../../specs/platform/system-design/provider-error-recovery.md#interactive-transient-retry-notice-lifecycle).
Preserve the original ID, timestamp, and turn association. Select the newest
matching notice when repairing legacy duplicates so the live row remains inside
the newest-message hydration window. Replace retry metadata as a unit so
obsolete provider or model fields cannot leak into later attempts. Upsert a
reused retry update in the client when its row is absent from the loaded window.
Retain existing message update handling and countdown rerender behavior.
Audit existing per-session guards before changing event ordering. Do not hold
`taskRuntimeStateMu` across storage operations or introduce recursive guard acquisition.

The earlier [cleanup package](../stale-transient-retry-notice/plan.md) remains
implemented. Its cleanup tests must seed legacy duplicates directly through the
task service once the normal notice writer stops creating duplicates.
Its transport-loss coverage was subsequently changed by the provider projection
contract (.24); this package does not restore automatic retries for that path.

## ASCII UI preview

### UI-01: Task chat during capacity retry

Entry: an open task session on desktop or phone. The chat owns scrolling.

Before (supplied screenshot):

```text
[ Model at capacity   Retrying now...   Attempt 1 of 5   Cancel ]
[ Model at capacity   Retrying in 0:01   Attempt 2 of 5   Cancel ]
```

After (shared desktop and phone composition):

```text
[ ! Model at capacity  Retrying in 0:10   Cancel ]
[   Provider: codex-acp                         ]
[   Attempt 2 of 5                              ]
```

The same banner updates from attempt 1 to attempt 2. At zero, the countdown
shows the existing localized “Retrying now...” state. Existing running-state
visibility and terminal cleanup remain unchanged. Cancellation removes the
retry banner and exposes the existing manual recovery actions.

One banner, latest metadata, and one Cancel action are structural requirements.
Spacing and example values are illustrative. Phone uses the existing inline
`TransientRetryNotice` exemplar: reason first, provider second, attempt third.
The short status belongs beside the conversation and needs no new overlay.
Text wraps within the chat width. Cancel remains visible and touch-reachable.
No navigation, safe-area, or scroll-owner change is planned. Existing localized
copy and `role="status"` remain. The mobile rendered check verifies containment
and Cancel behavior. This view covers criteria .9, .10, .11, and .25.

## Tests

- Add `TestTransientRetryStatusMessage_UpdatesSameNotice` in
  `event_handlers_transient_resolution_test.go`: attempt 1 then 2 retains one ID,
  latest deadline, and one added event followed by an updated event (.25).
- Add cases for legacy duplicates mixed with unrelated status rows and another
  session; list/update/delete errors; cancel/write races; and terminal cleanup
  (.10, .11, .25). Directly seed duplicates for legacy cleanup tests.
- Existing `action-message.test.tsx` rerender coverage verifies updated attempt
  and countdown (.9, .25). Extend `lib/ws/handlers/messages.test.ts` to prove
  changed retry metadata updates the existing row rather than appending (.25).

## E2E tests

Strengthen `tests/session/transient-retry.spec.ts` in `chromium` and
`tests/session/mobile-transient-retry.spec.ts` in `mobile-chrome`.
Wait for attempt 2 through the actual event or backend state, then assert one
card with the latest attempt and one Cancel action. Reload and repeat the
assertions, accounting for a later valid attempt. Cancel and assert no retry
card remains and manual recovery is available (.9, .10, .11, .25).
Use causal waits rather than sleeps. Do not use `.last()` to conceal duplicates.
The mobile test also verifies no horizontal overflow and a reachable Cancel action.

## Work orders

- [x] [Task 01: Update one retry notice](task-01-update-one-retry-notice.md)

## Verification

Run sequentially from the repository root. Managed E2E commands rebuild artifacts.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/orchestrator -run 'Test.*Transient.*' -count=1)
(cd apps/backend && go test -race ./internal/orchestrator -run 'Test.*Transient.*' -count=1)
(cd apps/web && pnpm exec vitest run components/task/chat/messages/action-message.test.tsx lib/ws/handlers/messages.test.ts lib/state/slices/session/session-slice.update-messages.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/session/transient-retry.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-transient-retry.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Verification results

Implementation and verification completed. The retry writer now updates the
newest matching status message, consolidates duplicates after a successful
update, preserves storage error behavior, and serializes writes with
retirement. Its per-session guard uses reference accounting for concurrent
users and a bounded five-minute retirement fence, so ordinary session churn
and deleted sessions do not retain obsolete state. Prompt evidence opens a
retired lifecycle only after a new execution identity is installed, and retry
timers arm after the failed turn is completed and the session is parked. The
client upserts a reused retry row when it is outside the loaded transcript
window. Desktop and mobile E2E cover attempt advancement, reload, stable
message identity, Cancel cleanup, and mobile document containment.

- Focused Go tests: passed.
- Focused Go tests with `-race`: passed.
- Full `internal/orchestrator` package tests: passed.
- Backend `make build`: passed.
- Backend `make lint`: passed with 0 issues.
- Changed-file Go lint with `--new-from-rev`: passed with 0 issues.
- Focused Vitest: 50 tests passed in 3 files.
- Web lint: passed with 0 issues.
- Web `pnpm run typecheck`: passed.
- Desktop managed E2E: 4 tests passed.
- Mobile managed E2E: 1 test passed.
- Specification catalog validation: 272 decisions and 932 specifications
  validated.
- Specification lint: passed.
- `git diff --check`: passed.
- Review regressions: passed for session churn/deletion reclamation, guard
  reference accounting, cancellation and terminal late failures, stale-event
  identity fencing, new-prompt fence reset, concurrent failures sharing one
  notice and timer entry, and client upsert of a reused retry row.

The repository-wide `pnpm run lint:e2e-sleeps` command remains non-clean because
of existing violations in untouched files, including missing ESLint rule
definitions and pre-existing hand-rolled waits. Targeted lint for the changed
E2E files passed.

CI fixup additionally stabilized the existing workflow settings and LSP E2E
surfaces. The workflow picker test dismisses a picker that can remain open
while dependent options are present. The LSP helper searches for exact paths
when virtualization leaves a valid file outside the mounted tree rows; its
archive cleanup test forces that condition with 48 preceding files.

- Workflow settings focused Chromium E2E with retries disabled and
  `--repeat-each=3`: 3 passed.
- LSP file intelligence with retries disabled: 13 passed.
- LSP archive cleanup focused E2E with retries disabled: 1 passed with the
  virtualized-tree regression fixture.
- Targeted ESLint and web TypeScript typecheck: passed.

## Risks

- A delayed update must not recreate a notice after cancellation or deletion.
- Persistence errors can leave older metadata or legacy duplicates until cleanup succeeds.
- The retained message belongs to an earlier turn; rendered tests must prove it stays current.
- Mock fail-then-success behavior has known lifecycle limitations. Backend tests
  provide deterministic success cleanup evidence.

## Documentation impact

This package changes design intent only. Public documentation stays unchanged.
Existing provider-recovery requirements and design remain the authoritative pair.
