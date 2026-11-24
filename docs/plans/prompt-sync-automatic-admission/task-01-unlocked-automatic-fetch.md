---
id: "01-unlocked-automatic-fetch"
title: "Fetch automatic Prompt Sync runs outside the lock"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PROMPT-SYNC-005
acceptance_criteria:
  - AC-INTEGRATIONS-PROMPT-SYNC-005.5
system_design:
  - ../../specs/integrations/system-design/prompt-sync.md
---

# Task 01: Fetch automatic Prompt Sync runs outside the lock

## Summary

A due automatic Prompt Sync waits for GitHub background admission without
holding the service lock. It then applies under a guard that drops the result
if anything changed during the wait.

## In scope

- `SyncIfDue`: start a cancelable `automaticRun` under `TryLock`, then unlock.
  Fetch and wait out admission deferrals with `retryDeferred`. Other locked
  mutations cancel the run. Lock again and check `finishAutomaticLocked`,
  then record or apply.
- Extract the shared apply-and-record tail from `syncLocked` so manual and
  automatic runs call the same function.
- Discard the result, record nothing, and log `prompt sync automatic result
  superseded` when the guard fails. Record nothing on context cancellation.

## Out of scope

- Manual runs, Save-and-sync, the source check, and the GitHub coordinator.
- New status fields, UI, metrics, or API changes.

## Acceptance

- Under steady principal-wide background pacing, one `SyncIfDue` call
  completes the run. It reads the listing once and each file once.
- While the automatic fetch waits, **Sync now** and version Save complete. A
  manual result, configuration save, removal, or credential change during
  the wait discards the automatic result.
- Poller stop during the wait returns promptly and records nothing.

## Verification

Write `TestAutomaticSyncCompletesUnderBackgroundPacing` first in a new
`automatic_admission_test.go`. Use a fake provider that defers the first read
of each file with `AdmissionDeferredError`. Confirm that it fails on current
code. Then add the concurrency, guard, and cancellation tests listed in the
[plan](plan.md#tests).

```bash
(cd apps/backend && go test -trimpath -tags fts5 -count=1 -run 'TestAutomaticSync' -v ./internal/promptsync)
(cd apps/backend && go test -trimpath -tags fts5 -count=1 -race ./internal/promptsync)
(cd apps/backend && go test -trimpath -tags fts5 -count=1 ./internal/github ./internal/workflowsync)
bash scripts/lint-go-changed apps/backend/internal/promptsync/service.go apps/backend/internal/promptsync/automatic_admission_test.go
```

## Files likely touched

- `apps/backend/internal/promptsync/service.go`
- `apps/backend/internal/promptsync/automatic.go` (new)
- `apps/backend/internal/promptsync/automatic_admission_test.go` (new)
- `apps/backend/internal/promptsync/admission_deferral_test.go`

## Dependencies

None.

## Risks

- Keep functions within the Go limits (80 lines, complexity 15). Split the
  fetch, guard, and tail into helpers.
- `goleak_test.go` checks the package for leaked goroutines. Each test must
  wait for its automatic-run goroutine before it ends.

## Inputs

- [Automatic runs](../../specs/integrations/system-design/prompt-sync.md#automatic-runs)
- `workflowsync/automatic_continuation_test.go` (`pacedWorkflowGitHub`
  fixture pattern)
- `promptsync/admission_deferral_test.go` (`coordinatorBackedGitHub`)

## Results

Completed.

- Before the fix, the regression test failed because no prompt was applied.
  After the fix, it passes.
- `go test -race ./internal/promptsync` passed. The four new tests also
  passed five times in a row under `-race`.
- `./internal/github`, `./internal/workflowsync`, and `./internal/reposync`
  passed.
- `scripts/lint-go-changed` reported 0 issues on the changed Go files.
- The specification lint and catalog validation passed.
- The automatic fetch uses `retryDeferred` instead of blocking coordinator
  admission. The [plan](plan.md#technical-approach) records why.
- Review fixes added cancellation under the lock and live fingerprint checks
  that include a removed connection. The superseded-wakeup and
  credential-change tests passed five times in a row under `-race`.
  Changed-file lint reported 0 issues.
