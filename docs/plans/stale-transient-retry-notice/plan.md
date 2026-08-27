---
spec: docs/specs/platform/requirements/provider-error-recovery.md
created: 2026-08-27
status: implemented
---

# Implementation Plan: Stale Transient Retry Notice

## Overview

Retire the persisted yellow transient-retry status messages whenever the
orchestrator no longer owns a retry loop. This prevents a recovered retry card
from reappearing when the session becomes idle and makes a late Cancel action
durably dismiss the card for reloads, task switches, and concurrent viewers.

The implementation keeps retry scheduling in memory, performs transcript
mutations through the task service and event bus, and preserves the existing
manual recovery card only when Cancel actually stops an active loop.

## Scope

In scope:

- Attempt to delete every persisted session message whose metadata contains the
  boolean value `retrying: true` when an interactive retry ends. Successful
  cleanup removes every matching message; failures remain eligible for a later
  authorized cleanup.
- Resolve stale notices after an authorized late Cancel even when no retry loop
  remains.
- Preserve the existing red Resume and Start fresh recovery message for active
  cancellation.
- Cover successful recovery, active and late cancellation, event publication,
  reload durability, desktop, mobile, and transport-loss flows.

Out of scope:

- Provider classification, retry counts, backoff timing, or copy changes.
- A new frontend message state or mobile-specific retry component.
- Dynamic-route policy behavior outside concrete-profile interactive recovery.

## Technical approach

- Add a narrow orchestrator dependency for listing and deleting session
  messages. Inject the existing task service directly from backend application
  wiring; do not expand the create-only `MessageCreator` adapter.
- Add a best-effort resolver that lists the session transcript, strictly
  matches boolean `metadata["retrying"] == true`, and deletes all matches
  through the task service. Continue after individual failures and log every
  swallowed list or deletion error.
- Keep in-memory timer reset separate from durable I/O where required so task
  service operations never run while `taskRuntimeStateMu` is held. Invoke both
  parts at every true loop-ending path: success, exhaustion, terminal state,
  stop, cancellation, and shutdown.
- Skip the full transcript lookup on ordinary successful turns with no owned
  retry entry. Terminal, stop, and Cancel paths force the lookup for stale
  persisted notices.
- Do not resolve notices from `nextTransientAttempt`; that transition only
  cancels the superseded timer while retry ownership continues.
- Keep `authorizeTaskSessionPair` as the first operation in
  `CancelTransientRetry`. After authorization, always run durable cleanup. A
  late cleanup returns false and emits no recovery message; active cancellation
  returns true and retains `handleRecoverableFailure`.
- Use deletion rather than `retrying: false`. The task service publishes
  `session.message.deleted`, and the existing frontend WebSocket/store path
  removes the notice without falling through to the generic warning renderer.

## Test strategy

Follow red-green-refactor in the single work order. Add focused orchestrator
tests in a new file before production changes. Use a real task service and
event bus where practical so the assertions cover transcript persistence and
`session.message.deleted`, not only mocks.

Backend coverage:

- Successful retry resolution deletes every outstanding retry notice, leaves
  unrelated messages intact, and publishes deletion events.
- Authorized Cancel with no active loop deletes stale notices, returns false,
  and creates no red recovery message.
- Authorized Cancel with an active loop deletes stale notices, returns true,
  and still creates the red recovery message.
- Retry attempt advancement does not resolve the current notice.
- Listing and individual deletion failures remain non-fatal.

## E2E tests

- Strengthen the existing desktop active-cancel scenario in
  `apps/web/e2e/tests/session/transient-retry.spec.ts` to assert that the yellow
  notice disappears when the red recovery card appears. Successful recovery and
  transcript durability are covered by the task-service-backed backend test;
  the current mock-agent fixture does not produce a stable fail-then-recover
  response after the orchestrator tears down and relaunches the agent.
- Strengthen
  `apps/web/e2e/tests/session/mobile-transient-retry.spec.ts` with the same
  disappearance assertion through the existing phone interaction.
- Strengthen
  `apps/web/e2e/tests/session/transient-retry-transport-lost.spec.ts` so the
  transport-loss cancellation also proves that yellow and red cards do not
  coexist.

## Work orders

Wave 1:

- [x] [Task 01: Retire stale transient retry notices](task-01-retire-stale-transient-retry-notices.md)

## Verification gate

- Focused orchestrator tests pass with the new resolution test file.
- The complete backend test and lint suites pass.
- Changed-file `golangci-lint` passes against the PR base SHA.
- Desktop, mobile, and transport-loss transient-retry Playwright specs pass.
- Specification lint and `git diff --check` pass.

## Risks

- Performing task-service I/O under `taskRuntimeStateMu` could extend a global
  critical section. The implementation must split or move durable cleanup out
  of that lock.
- Reset has several terminal call sites. Tests and call-site review must prove
  that every true loop-ending path resolves messages while attempt advancement
  does not.
- A boolean metadata check must not treat string or numeric values as active
  notices.
- Deletion can race another cleanup. Missing messages remain a logged,
  non-fatal outcome, and every matching message is attempted.

## Verification results

Implemented and verified:

- Focused orchestrator regression tests passed, including durable cleanup,
  late and active Cancel, denied authorization, attempt advancement, and
  swallowed store errors.
- `rtk go test ./internal/orchestrator -count=1` passed (2,168 tests).
- `rtk go test -race ./internal/orchestrator -run 'Test(ResetTransientRetry_ResolvesPersistedNotices|ResetTransientRetry_WithoutActiveLoopSkipsTranscriptScan|CancelTransientRetry_NoActiveLoopResolvesPersistedNotice|CancelTransientRetry_ActiveLoopResolvesNoticeAndShowsRecovery|CancelTransientRetry_DeniedPairDoesNotResolveNotice|NextTransientAttempt_DoesNotResolveCurrentNotice|ResolveTransientRetryMessages_SwallowsStoreErrors|StopSession_ResolvesPersistedNotice)' -count=1` passed (8 tests).
- `rtk make -C apps/backend test` passed after clearing managed launcher
  configuration variables from the test process. The first unmodified run was
  affected by the workspace's injected `/root/.kandev/config.yaml` override.
- `rtk make -C apps/backend lint` and changed-code `golangci-lint` against
  `a5a15f12ea178f86cb8ca8c482bcc823cbe9d4a2` passed.
- Desktop Chromium transient-retry and transport-loss specs passed (7 tests);
  mobile Chromium transient-retry spec passed (1 test).
- Web typecheck and lint passed. Specification tests, full specification lint,
  gofmt, and `git diff --check` passed.
