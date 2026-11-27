---
id: "03-record-retention-preparation-failure"
title: "Record why a tool-payload retention backup preparation failed"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002
acceptance_criteria:
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.9
system_design:
  - ../../specs/system-page/system-design/tool-payload-retention.md
---

# Task 03: Record retention preparation failure

## Summary

When backup preparation fails, log the error through a new `Options.Log`
callback and keep a bounded copy in the persisted retention record. The status
response keeps returning only `backup_failed`.

## In scope

- `internal/system/toolretention/service.go`: `Options.Log` and a nil-safe
  `logFailure` helper. Clear the stored message on disable and on a new pending
  preparation in `Save`.
- `internal/system/toolretention/types.go`: `record.PreparationDetail`
  (`preparation_detail`), outside the embedded `Status`.
- `internal/system/toolretention/preparation.go`: set and log the message in
  `finishPreparation`. Cut it to 1,024 bytes without splitting a UTF-8
  sequence. Clear it on success and in `failPreparation`.
- `internal/system/system.go`: wire `Options.Log` to the backend logger at
  error level.
- Tests in `internal/system/toolretention/preparation_test.go`:
  - a failing backup is logged, the stored record keeps the message, and the
    status shows only `backup_failed`;
  - the cut keeps at most 1,024 bytes and valid UTF-8;
  - a later disable clears the stored message.

## Out of scope

UI, API response fields, scheduler failure counting, and the backup
verification path.

## Acceptance

- The new tests fail on the base branch (no log call, no stored message) and
  pass after the change.
- `GET` status output is unchanged for a failed preparation.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/system/toolretention -run '^Test(PreparationFailureRecordsCause|LimitPreparationDetail|DisableClearsPreparationDetail)' -count=1 -v)
(cd apps/backend && go vet ./internal/system/toolretention/... ./internal/system)
(cd apps/backend && go build ./...)
```

## Likely files

- `apps/backend/internal/system/toolretention/service.go`
- `apps/backend/internal/system/toolretention/types.go`
- `apps/backend/internal/system/toolretention/preparation.go`
- `apps/backend/internal/system/toolretention/preparation_test.go`
- `apps/backend/internal/system/system.go`

## Results

Implemented on 2026-10-12, verified on Windows 11 with Go 1.26.

- Red: on the base commit, `TestPreparationFailureRecordsCause` (then
  asserting only on the status JSON and the raw settings row) failed with
  `stored record keeps the failure cause`: the row held only `backup_failed`.
- Green: `TestPreparationFailureRecordsCause` now also checks the `Options.Log`
  call and `record.PreparationDetail`; it passes with
  `TestDisableClearsPreparationDetail`, `TestLimitPreparationDetail` (a
  three-byte rune at the limit is not split) and the existing
  `TestPreparationGatesInitialCleanup`. The last two new tests use new symbols,
  so they have no runtime Red on the base commit.
- The status JSON for a failed preparation still contains only the
  `backup_failed` code.
- `go vet ./internal/system/toolretention/... ./internal/system` and
  `go build ./...` pass.

### Review remediation

Invalid UTF-8 in an error message could survive the limiter and expand during
JSON serialization, leaving a decoded stored message larger than 1,024 bytes.
Normalize invalid sequences before truncation. Add regressions for the limiter
and the actual stored record while retaining the original callback error and
public status code. Additional tests cover clearing on retry, success, and
recovery, and preservation under stale revision and operation identities.

The review also identified failure logging before the guarded state transaction.
Log only after a matching failure commits, and suppress cancellation errors.
Regressions cover stale revision and operation completions, cancellation,
expired contexts, and rejected settings writes while retaining a positive
current-failure logging control.

Focused command:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/system/toolretention -run '^Test(LimitPreparationDetail|PreparationFailure|PreparationDetail|StalePreparationFailure)' -count=1 -v)
```

The tiny exact-source limiter proof fails before normalization, including a
3,072-byte decoded stored value, and passes afterward in the task sandbox.
The guarded logging proof also fails before the fix and passes afterward, using
exact production methods and an explicit transaction boundary fake.
Full SQLite persistence and service regressions run in hosted CI; no local
backend dependency rebuild is required.
