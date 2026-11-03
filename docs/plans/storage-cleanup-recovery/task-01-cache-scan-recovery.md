---
id: "01-cache-scan-recovery"
title: "Recover stale cache scans"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-GO-CACHE-004
acceptance_criteria:
  - AC-SYSTEM-PAGE-GO-CACHE-004.7
  - AC-SYSTEM-PAGE-GO-CACHE-004.8
  - AC-SYSTEM-PAGE-GO-CACHE-004.11
  - AC-SYSTEM-PAGE-GO-CACHE-004.12
  - AC-SYSTEM-PAGE-GO-CACHE-004.13
system_design:
  - ../../specs/system-page/system-design/go-cache-reclamation.md
---

# Task 01: Recover stale cache scans

## Summary

Revalidate retained traversal frames before resuming cleanup. Replace stale scan
state with fresh discovery while preserving deletion safety and bounded work.

## In scope

- Add `TestCleanupRestartsRecreatedShardBeforeResume` before production changes.
  Seed three files in shard `00`, cross a one-byte threshold, and exhaust a
  three-entry pass budget. Move the old shard outside the cache, then recreate
  the same file names. The resumed invocation must restart discovery safely.
  Current code produces repeated `go-cache parent changed after scan` errors.
- Pair the regression with unchanged-cursor completion and below-threshold replacement cases.
- Validate active retained frame identities, symlinks, and path/descriptor mounts through the anchored root.
- Restart once per invocation after an ordinary directory replacement or disappearance.
  Reuse the existing deadline and entry budget. Discard stale measured bytes.
- Stop and retire a cursor invalidated during removal. Return `cache_parent_changed`
  with `cache_scan_invalidated` and preserve successful unlink byte counts.
- Cover cancellation, repeated replacement, candidate directories whose frames
  closed, root replacement, symlink substitution, and unavailable mount identity.
- Keep fuzz corpus, ownership marker, top-level shard directories, and unrelated sentinels safe.

## Out of scope

Runner state changes, API schemas, schedule changes, automatic retry loops,
frontend rendering, live cleanup, and weakened identity checks.

## Acceptance

1. A recreated safe shard resumes through fresh discovery without the incident's repeated errors. Unchanged cursors retain progress.
2. Stale measurements never authorize replacement deletion. All safety sentinels survive unsafe replacements and mount failures.
3. Every path remains bounded and cancellable. A late identity mismatch retires the cursor with one issue and correct removed-byte accounting.

## Verification

Run from the repository root. Use `/tdd` and the scoped backend test conventions.
Run the named regression before implementation and retain its expected failure.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/system/storage/gocache -run '^TestCleanupRestartsRecreatedShardBeforeResume$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/system/storage/gocache -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/system/storage/... -count=1)
bash scripts/lint-go-changed apps/backend/internal/system/storage/gocache/cleanup_session.go apps/backend/internal/system/storage/gocache/cleanup_session_recovery_test.go
git diff --check
```

If the implementation needs additional Go files, include them in changed-file lint.
Keep all cache fixtures in private temporary directories.

## Files likely touched

- `apps/backend/internal/system/storage/gocache/cleanup_session.go`
- `apps/backend/internal/system/storage/gocache/cleanup_session_recovery_test.go` (new)
- `apps/backend/internal/system/storage/gocache/direct_cleanup_safety_test.go`

## Dependencies

None.

## Risks

Reopening a path without preserving its original identity can weaken containment.
Do not substitute modification-time checks for inode checks. A restart must not
reuse the old threshold or reset the deadline and work budget.

## Parallelism

`sequential`

## Inputs

- Design: Resumed scan validation and Direct deletion.
- [Incident evidence and reproduction](plan.md#evidence-and-root-cause).
- Existing `TestCleanupContinuesThresholdScanAcrossBoundedPasses` and same-device mount regressions.

## Results

Implemented on 2026-11-03 in `cleanup_session.go` with
`cleanup_session_recovery_test.go`. Retained frame chains are revalidated through
the anchored root before reuse. An ordinary directory recreation or removal
restarts fresh threshold discovery once with the original pass budget and
deadline and discards stale measurements. Symlink, root, and mount-identity
failures still fail closed. Removal-time identity mismatches retire the cursor,
return `cache_parent_changed` with reason `cache_scan_invalidated`, and keep
successful unlink bytes. Review remediation added regressions that retain an
above-threshold delete cursor and a partial-unlink cursor before replacing the
shard with below-threshold content. Both require fresh `BytesBefore`/`BytesAfter`
measurement and verify that replacement files survive. Review remediation added
mount-identity classification before the stale restart, parent-disappearance
invalidation, focused remainder-entry and directory-removal retirement tests,
and reservation of the stable `cache_parent_changed` issue when the bounded
sample is full.

Verification:

- `go test -trimpath -tags fts5 ./internal/system/storage/gocache -run '^TestCleanupRestartsRecreatedShardBeforeResume$' -count=1 -v`
  failed before the fix with repeated `go-cache parent changed after scan` errors
  and passed after it.
- `go test -trimpath -tags fts5 -race ./internal/system/storage/gocache -count=1` passed.
- `go test -trimpath -tags fts5 ./internal/system/storage/... -count=1` passed.
- Changed-file Go lint reported 0 issues.
- With retained-frame validation temporarily disabled, both replacement
  regressions fail with `cache_scan_invalidated` and stale `BytesBefore: 6`, so
  they exercise the discarded threshold authority rather than only a fresh scan.

Remaining issue-closure gate: mount-identity behavior on macOS and Windows and
validation against a physical alternate mount were not exercised in this Linux
environment. The Linux unit and race regressions cover the shared logic, but
native-platform and physical-device confirmation remains required before closing
the mount-safety acceptance criteria.
