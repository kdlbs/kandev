---
created: 2026-11-24
status: complete
requirements:
  - REQ-INTEGRATIONS-PROMPT-SYNC-005
system_design:
  - ../../specs/integrations/system-design/prompt-sync.md
legacy_specs: []
---

# Implementation plan for Prompt Sync automatic admission

## Overview

Repair [issue #5468](https://github.com/kdlbs/kandev/issues/5468). An
automatic GitHub Prompt Sync never completes while other background GitHub
work is steady. `last_synced_at` stops advancing and new repository prompts
stay unsynced. **Sync now** still works.

This was item 6 in the
[Prompt Sync follow-up findings](../prompt-sync-follow-up-fixes/plan.md#other-issue-findings).
That plan named two options: resumable reads, or a guarded fetch outside the
service lock. This plan uses the guarded fetch.

[AC-INTEGRATIONS-PROMPT-SYNC-005.5](../../specs/integrations/requirements/prompt-sync.md#req-integrations-prompt-sync-005-automatic-sync-and-failures)
defines the outcome.
[Automatic runs](../../specs/integrations/system-design/prompt-sync.md#automatic-runs)
defines the control flow.

## Root cause

`Service.syncLocked` holds `Service.mu` for the whole run. Automatic GitHub
runs therefore use `WithNonBlockingGitHubAdmission` with
`WorkClassBackground`. `RateAdmission.tryAcquireBackground` returns
`AdmissionDeferredError` when the request is not admitted immediately.
`recordRunError` drops the run and leaves the next 60-second `Poller` tick to
retry.

Background admission is principal-wide and paced. `releaseBackground` sets
`nextBackgroundAt` to now plus at least one second after every background
request. So the first file download after the run's own listing is deferred
with `background_pacing` and a delay of about one second. That matches the
issue log (`delay: 0.9996`). Steady PR watches also keep the slot busy
(`background_in_flight`, `interactive_priority`). Each tick lists again and
defers again, so an automatic run with at least one prompt file cannot
finish.

Workflow Sync is not affected. Its scheduler keeps the continuation state and
waits for `AdmissionDeferredError` changes outside its execution pool
(`waitAutomaticJobAdmission`).

Evidence:

- Read-only trace of `promptsync/service.go` `syncLocked`,
  `promptsync/failure.go` `recordRunError`, and
  `github/rate_coordinator.go` `tryAcquireBackground` and `releaseBackground`.
- The earlier follow-up plan verified this with a PAT client and HTTP fixture
  on the real coordinator: two successful listings and zero file downloads.

## Requirement classification

The intended behavior was missing. REQ-INTEGRATIONS-PROMPT-SYNC-005 covered
backoff and suspension but not admission deferral. The design said only that
"a deferred run retries on the next tick", and pacing defeats that. This
package adds AC-INTEGRATIONS-PROMPT-SYNC-005.5. It updates the design's
control flow, automatic-run section, and observability logs. The
Workflow Sync equivalent is AC-INTEGRATIONS-GITHUB-RATE-003.7.

## Scope

- Split `SyncIfDue` into three phases: snapshot under the lock, an unlocked
  fetch that waits out admission deferrals, and a guarded apply under the lock.
- The guard discards the result after a configuration save, credential
  change, removal, or manual result during the fetch.
- Treat cancellation during the fetch (poller stop) as no result.

## Exclusions

- Manual runs, Save-and-sync, and the source check keep their current paths.
  Manual runs still return `AdmissionWaitError` without waiting.
- No new status field, UI indicator, metric key, or API change. A waiting
  automatic run leaves the stored status unchanged, as a deferred run does
  today.
- No resumable read cache. A failed or superseded run starts its next fetch
  from the listing.
- Workflow Sync and the GitHub coordinator stay unchanged.

## Technical approach

- `promptsync/automatic.go`: `SyncIfDue` takes a `dueSnapshot` under
  `TryLock`, fetches without the lock, then locks again and checks
  `currentAutomaticTarget`. The check compares `UpdatedAt`,
  `CredentialFingerprint`, the source fields, and the GitHub host. A
  canceled context or a failed check records nothing.
- `promptsync/service.go`: split `syncLocked` into `fetchRun` and
  `applyRun`. Manual and automatic runs share both. The automatic fetch keeps
  non-blocking background admission and wraps each list and read in
  `retryDeferred`. On `AdmissionDeferredError`, `retryDeferred` calls `Wait`
  and repeats only that read.
- Implementation note: the planned blocking coordinator admission was replaced
  with `retryDeferred`. The coordinator's HTTP client is unexported and rejects
  loopback, so `promptsync` tests cannot drive it. `retryDeferred` has the same
  wait semantics, matches Workflow Sync, and can be tested with deferral fakes.
  The guard does not re-check that the configuration is due. A credential
  reset clears due-ness only in memory, and `UpdatedAt` and the fingerprint
  already cover every write.

## Tests

New tests are in `promptsync/automatic_admission_test.go`. The `pacedGitHub`
fake defers the first read of every file, which models background pacing.

- `TestAutomaticSyncCompletesUnderBackgroundPacing` is the regression test.
  One `SyncIfDue` applies both prompts and records success with one listing and
  two attempts per file. It failed before the fix with `prompt "a" is applied
  by the paced automatic run`.
- `TestAutomaticSyncDoesNotBlockManualWorkWhileWaiting`: **Sync now**
  completes while the automatic read waits. The automatic result is then
  discarded.
- `TestAutomaticSyncDiscardsResultAfterChange`: table-driven over a
  configuration save, removal, and a credential change during the wait.
- `TestAutomaticSyncCancelledDuringFetchRecordsNothing`.
- `TestAutomaticRunDeferredByGitHubAdmissionIsNotAFailure` now asserts that
  the run waits without listing again or recording a failure.

## E2E tests

None. The repair is a backend scheduling path with no UI or API change. The
browser fixture cannot reproduce principal-wide pacing reliably. The Go
regression test uses the real coordinator.

## Work orders

- [x] [Task 01: Fetch automatic Prompt Sync runs outside the lock](task-01-unlocked-automatic-fetch.md)

## Risks

- A configuration may change between the guard and apply. The apply holds the
  mutex, so every writer is serialized behind it.
- `UpdatedAt` equality depends on the stored timestamp round-trip. Both
  values come from `store.Get`, so they use the same encoding.
- An automatic run can wait until a primary quota reset. The poller
  goroutine is dedicated, so ticks pause during that wait and status is
  unchanged. Stop cancels the wait.

## Verification

```bash
(cd apps/backend && go test -trimpath -tags fts5 -count=1 -race ./internal/promptsync)
(cd apps/backend && go test -trimpath -tags fts5 -count=1 ./internal/github ./internal/workflowsync)
bash scripts/lint-go-changed apps/backend/internal/promptsync/service.go apps/backend/internal/promptsync/automatic_admission_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```
