---
id: "01-update-one-retry-notice"
title: "Update one retry notice"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.9
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.10
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.11
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.25
system_design:
  - ../../specs/platform/system-design/provider-error-recovery.md
---

# Task 01: Update One Retry Notice

## Summary

Update the existing retry message as each attempt is scheduled.
Prove that connected viewers and reloads show one current banner.

## In scope

- Task-service message updates, duplicate consolidation, and failure handling.
- Existing cancellation and terminal cleanup guarantees.
- Backend, store, component, desktop, and phone regression evidence.

## Out of scope

Generic transcript warnings, policy changes, Office routing, and new UI copy.

## Acceptance

- Consecutive attempts retain one persisted ID with latest metadata and emit
  an update event. Legacy duplicates consolidate without changing unrelated rows.
- Store errors do not create extra notices. Cancellation and terminal cleanup
  cannot be undone by stale writes. Existing authorization remains first.
- Desktop and phone show one current card after attempt advancement and reload.
  Cancel removes it and retains the existing manual recovery actions.

## ASCII UI preview

See the [full before/after view](plan.md#ascii-ui-preview).

UI-01: task chat, desktop and phone, attempt 2 waiting (.9, .25):

```text
[ ! Model at capacity  Retrying in 0:10   Cancel ]
[   Provider: codex-acp                         ]
[   Attempt 2 of 5                              ]
```

Reuse the existing inline composition. The chat owns scrolling. Phone text
wraps and Cancel stays reachable. Running and terminal states retain existing
visibility rules (.10, .11). No extra card is appended.

## Implementation sequence

1. Add the same-ID backend regression and demonstrate the two-row failure.
2. Add the narrow update dependency and implement the design's selection rules.
3. Preserve existing guards and cleanup. Add error, mixed-row, and race tests.
4. Seed legacy duplicates directly in cleanup tests. Add store and rendered assertions.
5. Run the exact checks below and record each result in both package files.

## Verification

Run sequentially from the repository root. Use managed E2E builds.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/orchestrator -run 'Test.*Transient.*' -count=1)
(cd apps/backend && go test -race ./internal/orchestrator -run 'Test.*Transient.*' -count=1)
(cd apps/web && pnpm exec vitest run components/task/chat/messages/action-message.test.tsx lib/ws/handlers/messages.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/session/transient-retry.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-transient-retry.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/event_handlers_transient.go`
- `apps/backend/internal/orchestrator/event_handlers_transient_resolution_test.go`
- `apps/web/lib/ws/handlers/messages.test.ts`
- `apps/web/e2e/helpers/transient-retry.ts`
- `apps/web/e2e/tests/session/transient-retry.spec.ts`
- `apps/web/e2e/tests/session/mobile-transient-retry.spec.ts`

## Dependencies

None. Reuse the task-service-backed fixture and existing message update pipeline.

## Risks

Guard reentrancy, cancellation races, and old-turn visibility require focused proof.
Do not introduce database I/O under the global runtime-state mutex.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/provider-error-recovery.md), criteria .9-.11 and .25.
- [Design](../../specs/platform/system-design/provider-error-recovery.md#interactive-transient-retry-notice-lifecycle).
- `newPersistentTransientRetryTestService`, `TransientRetryNotice`, and
  `createMessagesHandlerRegistration` in the existing source and tests.
- [Earlier cleanup work order](../stale-transient-retry-notice/task-01-retire-stale-transient-retry-notices.md).

## Results

Implemented the single persisted retry-notice lifecycle. The orchestrator
updates the newest exact task and session match, replaces retry metadata as a
unit, removes legacy duplicates after a successful update, and leaves failed
storage operations eligible for later cleanup. Per-session serialization and a
retirement fence prevent cancellation or terminal cleanup from being undone by
late provider events. The WebSocket update path upserts a reused retry row when
the row is outside the loaded transcript window, and the desktop and mobile E2E
suites verify one current notice through advancement, reload, and Cancel.

The review remediation adds reference-counted guard ownership and a bounded
five-minute retirement fence. Active prompt/retry state remains owned, normal
successful-turn cleanup releases it, and deletion/terminal cleanup keeps only
the short fence needed to reject late provider events. New prompt evidence
opens that fence only after its execution identity is complete. Retry entries
are reserved before completion but armed after the failed turn is completed and
the session is parked. Deterministic backend tests cover session churn,
deletion, concurrent guard users, cancellation and terminal late failures,
stale-event identity fencing, new-prompt fence reset, and concurrent failures
that share one notice and timer entry; frontend tests cover a reused row missing
from the loaded transcript.

- `go test ./internal/orchestrator -run 'Test.*Transient.*' -count=1`: passed.
- `go test -race ./internal/orchestrator -run 'Test.*Transient.*' -count=1`:
  passed.
- `go test ./internal/orchestrator -count=1`: passed.
- `make build`: passed.
- `make lint`: passed with 0 issues.
- Changed-file Go lint with `--new-from-rev`: passed with 0 issues.
- Focused Vitest: 50 tests passed in 3 files.
- Web lint: passed with 0 issues.
- `pnpm run typecheck`: passed.
- Managed Chromium E2E: 4 passed.
- Managed mobile-chrome E2E: 1 passed.
- `python3 scripts/list-docs.py validate`: passed, 272 decisions and 932
  specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.

Targeted lint for the changed E2E files passed. The repository-wide
`pnpm run lint:e2e-sleeps` command still reports pre-existing violations in
untouched files.

CI fixup also removed two unrelated E2E blockers exposed by the first PR run.
The workflow settings test now dismisses the shared picker explicitly after a
model selection, because dependent options can keep it open. The LSP file
opening helper now uses exact file search when virtualization leaves a valid
file outside the mounted tree rows. The archive cleanup regression seeds 48
preceding files so that path is exercised deterministically.

- Workflow settings focused Chromium E2E with retries disabled and
  `--repeat-each=3`: 3 passed.
- LSP file intelligence focused archive cleanup E2E with retries disabled: 1
  passed with the virtualized-tree regression fixture.
- Full LSP file intelligence E2E with retries disabled: 13 passed.
- Targeted ESLint for the three changed E2E files: passed.
- Web TypeScript typecheck: passed.
