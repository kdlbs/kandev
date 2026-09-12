---
created: 2026-09-12
status: implemented
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007
system_design:
  - ../../specs/agents/system-design/session-recovery-failures.md
legacy_specs: []
---

# Implementation Plan: Resume Cancellation

## Overview

Preserve the saved conversation after an inconclusive resume failure. Prevent
cancelled startup work from dispatching a prompt or changing a newer attempt.
Reuse the existing recovery card for failures before dispatch.

Work proceeded sequentially: load classification, attempt cancellation, then
recovery projection and rendered regression coverage. The implementation is
complete.

## Evidence and root cause

The affected task is `df788894-8dec-4454-bbe2-2ad90fc187cf`.
Its session is `11fa851b-b8a5-48ff-8436-6c1de520ce35`.
Retained backend logs on September 11 show this sequence in Lisbon time:

| Time | Evidence |
| --- | --- |
| 18:20:03 | ACP loads the saved session. |
| 18:22:03 | Load returns an internal error containing `context deadline exceeded`. |
| 18:22:03 | Kandev falls back to `session/new`. |
| 18:23:16 | Cancellation marks the turn complete while startup continues. |
| 18:23:23 | A new user prompt arrives. |
| 18:23:35 | Startup publishes a new provider conversation identity. |
| 18:23:36 | One prompt dispatches and another receives the already-running rejection. |

The log is `/root/.kandev/logs/backend-logs-2026-09-11-000022.log`.
The task conversation records the generic failed-send message and no later
agent response. These records confirm the overlap but do not identify why the
provider load timed out. No claim about the provider's internal cause is needed
for this repair.

Current source still permits fallback after unclassified load errors.
`isTransportDeadErr` recognizes typed deadlines but misses this serialized one.
`ResumeTaskSessionWithOptions` detaches request cancellation. Manual prompt
admission performs a potentially blocking resume before its final dispatch
claim. Cancellation therefore needs explicit attempt ownership across these
boundaries. Existing execution generation checks alone do not identify a
cancelled pre-dispatch prompt on a reused execution.

## Requirement conformance

Existing recovery criteria 001.1, 001.2, 002.1, and 006.1 through 006.6 define
identity preservation and the recovery surface. Requirement 007 adds explicit
outcomes for inconclusive load failure and cancellation during startup.
The agents system owns both the requirement and design. Task queue policy and
backend cancellation projection remain dependencies.

The completed `contribution-resume-recovery` package supplies the recovery
owner. Its work orders remain complete. This package adds new coverage without
rewriting historical verification results. The resume queue package supplies
admission and Auto-run behavior, which this work must preserve.

## Scope

### In scope

- Positive classification of supported load fallback cases.
- Cancellation ownership across manual resume, lazy resume, and handler retry.
- Guarded token, state, callback, and prompt publication for the current attempt.
- Existing recovery projection for pre-dispatch resume failures.
- Deterministic backend tests and desktop/phone regressions.

### Out of scope

- Increasing timeouts or diagnosing provider internals.
- Changing confirmed missing-session or unsupported-load fallback policy.
- Reconstructing the affected task's old provider history or running its rebase.
- New queue behavior, schema, runtime flag, or recovery layout.
- Automatic replay after ambiguous provider acceptance.

## Technical approach

The [design](../../specs/agents/system-design/session-recovery-failures.md#proposed-attempt-isolation-requirement-007) defines the
attempt identity and cancellation boundary. Add narrow helpers outside the
large `task_operations.go` file. Reuse existing cancellation guards, generation
checks, failure stamps, and queue reservations. Release guards before waits.

The handler retry preserves the same operation identity. Cancellation
invalidates that identity before cleanup. Dispatch and publication reject an
invalidated identity even when the runtime later becomes ready.

## ASCII UI preview

### UI-01: Task chat recovery after a load timeout

Desktop:

```text
[Recovery failed]
[Retry] [existing secondary recovery actions]
[> Details]
```

Phone:

```text
[Recovery failed]
[Retry                               ]
[existing secondary recovery actions ]
[> Details                           ]
```

The illustration uses semantic labels, not final translated strings. The
existing recovery card is the required surface. Details identify the resume
load timeout. During Retry, equivalent actions remain disabled. During
cancellation, the existing cancellation progress remains visible until cleanup
settles. No new prompt starts from the cancelled attempt.

The transcript owns scrolling. Phone actions retain 44-pixel hit targets and
existing safe-area clearance. Details wrap without horizontal page overflow.
These structures cover AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.5 and .6.

## Tests

| Criteria | Regression evidence |
| --- | --- |
| 007.1 | `session_load_failure_test.go`: serialized deadline and internal-error cases never call `session/new`. |
| 007.2-.4 | `task_operations_resume_cancellation_test.go`: real service cancellation at continuation and provider-acceptance barriers, readiness cancellation, callback fencing before and after replacement on one execution, exact cleanup ownership, shutdown, browser disconnect, and parked Auto-run-off work. `manager_events_startup_test.go` verifies lifecycle origin publication. |
| 007.5 | `message_handlers_resume_readiness_test.go`: correlated recovery suppression, preserved resume/readiness errors, and the exact cancel-at-retry-dispatch barrier. |
| 007.5-.6 | Desktop delayed-resume cancel/retry and saved-session failure tests, plus mobile touch cancel/retry and failed-load recovery tests. Existing recovery specs retain lost-branch recovery, explicit replacement, reload persistence, keyboard access, touch targets, and overflow coverage. |

The new backend regressions were written before the production changes and
cover the serialized load failure, attempt identity fencing, and handler
deduplication. Existing browser suites cover the recovery card and desktop and
phone interaction contracts.

## E2E tests

The existing `session-recovery.spec.ts` and
`mobile-session-resume-recovery.spec.ts` cover the recovery card, explicit
replacement, reload persistence, keyboard access, touch targets, and overflow.
The added desktop and mobile delayed-resume tests cancel a real STARTING
session and submit a new prompt through the composer. The saved-session tests
exercise a provider that fails ACP resume and verify that the recovery action
remains usable without the old permanently pending label. Backend barrier
tests prove the cancellation and retry ordering through the real orchestration
paths.

## Work orders

- [x] [Task 01: Preserve identity after load failure](task-01-load-failure.md)
- [x] [Task 02: Cancel startup attempts](task-02-startup-cancellation.md)
- [x] [Task 03: Present recoverable resume failures](task-03-recovery-feedback.md)

## Verification results

Implementation checks on 2026-09-12:

- `go test -race ./internal/orchestrator -count=1 -timeout=20m`: passed, including the service/runtime race coverage.
- `go test -race ./internal/agent/runtime/lifecycle -count=1 -timeout=15m`: passed.
- `go test -race ./internal/task/handlers -count=1 -timeout=15m`: passed.
- `go test -race ./internal/orchestrator/executor -count=1 -timeout=15m`: passed.
- Focused lifecycle, orchestrator, and handler regressions for load classification, attempt fencing, callback origin, recovery correlation, and prompt retry: passed.
- `pnpm install --frozen-lockfile`: passed from `apps`.
- Focused Vitest for `task-launch-error-entry`: 18 tests passed.
- Web typecheck and `i18n:check`: passed.
- Desktop Chromium delayed-resume cancel/retry E2E: 1 test passed.
- Mobile Chromium delayed-resume cancel/retry E2E: 1 test passed.
- Mobile Chromium failed saved-session load E2E: 1 test passed.
- Existing desktop failed-resume E2E coverage remains in `session-recovery.spec.ts` and shares the same `--fail-on-resume` fixture.
- `make -C apps/backend build`: passed for the backend binaries and helper targets.
- Specification lint and `git diff --check`: passed.

Design validation on 2026-09-12:

- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/plans/resume-cancellation`: passed.
- `git status --short -- docs/specs docs/plans/resume-cancellation`: two amended specifications and the four new package files.

The work-order references resolve to requirement 007 and its six criteria.
The existing agents index already links both amended specification files.

The aggregate `make -C apps/backend test` command remains non-green because
the active Kandev environment injects `/root/.kandev/config.yaml` and port
settings into configuration tests, the Office FTS migration fixtures are
missing expected columns, and two unrelated process-probe tests are
environment-sensitive. It also reports the existing nil-executor async test
panic and one unrelated completed-task follow-up failure. The prescribed
affected-package race suite and the sanitized focused affected-package tests
pass.

## Risks

- Overbroad error matching can change supported provider fallback behavior.
- A cancellation guard held across startup can deadlock cleanup or block Stop.
- Cleanup by session ID alone can terminate a replacement execution.
- Error deduplication without attempt correlation can hide historical failures.
- Request disconnect must remain distinct from explicit cancellation.
