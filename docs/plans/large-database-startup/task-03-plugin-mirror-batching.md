---
id: "03-plugin-mirror-batching"
title: "Plugin session-event mirror startup batching"
status: done
wave: 3
depends_on: ["01-lifecycle"]
plan: plan.md
requirements:
  - REQ-PLATFORM-STARTUP-LIFECYCLE-001
acceptance_criteria:
  - AC-PLATFORM-STARTUP-LIFECYCLE-001.4
  - AC-PLATFORM-STARTUP-LIFECYCLE-001.6
system_design:
  - ../../specs/platform/system-design/startup-lifecycle.md
---

# Plugin session-event mirror startup batching

## Scope

On a 2026-09-17 upgrade with a 4 GB conversation journal (669,423 messages,
2,945 sessions), the plugins provider's one-time mirror of every committed
journal event into `~/.kandev/plugins/.host/session-events.sqlite` ran for
about 4 minutes inside `initializing_services`, with no log line and no
`kandev.db` writes. It looked like a deadlock and was only identified from a
`kill -QUIT` goroutine dump. It completed, and both runs resumed correctly.

Measurement (`mirror_startup_cost_test.go`) found the cost is per-*event*, not
per-session: one `Begin`/`Commit` per event in the durable append path. Batching
by a fixed event count across session boundaries (256 events/commit) cuts
per-event cost from 0.442 ms to 0.011 ms, roughly 40x, matching the reported
wall-clock time. Per-session batching alone is only ~1.5x, because production
sessions carry ~2 events each.

## Exclusions

Retention/pruning, poison record semantics, the delivery dispatcher, cursor
handling, mirror database pragmas, the primary journal schema, and a
per-session startup progress UI (tracked separately). No new requirement or
acceptance criterion: existing AC-PLATFORM-STARTUP-LIFECYCLE-001.4 (bounded
periodic status during long phases) and AC-PLATFORM-STARTUP-LIFECYCLE-001.6
(cancellation stops further initialization admission) already cover this
behavior; the change makes the mirror sync conform to them.

## Acceptance

- Batch mirrored-event commits by a fixed event count across session
  boundaries instead of one transaction per event, preserving watermark/gap
  healing, poison records, terminal partitions, and per-partition failure
  isolation exactly.
- Thread a cancellable startup context into the boot-time sync instead of
  `context.Background()`.
- Stop accumulating the full mirrored-event slice at the boot call site, which
  discarded it.

## Files likely touched

`apps/backend/internal/plugins/conversation_journal.go`,
`apps/backend/internal/plugins/conversation_stream.go`,
`apps/backend/internal/plugins/provider.go`, `apps/backend/internal/backendapp`
(context threading through `provideServices`).

## Verification

Backend commands run from `apps/backend`.

- `go test ./internal/plugins/... ./internal/backendapp/... -count=1`
- `go test ./internal/plugins/ -run 'TestMirrorCommitBatchingFloor|TestMirrorStartupCostPopulated|TestAppendCommittedBatch.*|TestMirrorSyncBatch.*' -v -count=1`
- `make lint`

## Dependencies

Builds on [01: Startup lifecycle](task-01-lifecycle.md)'s bootstrap ownership
and cancellation contexts; does not modify them.

## Parallelism

Sequential. No delegation.

## Risks

A file-database rollback-journal commit is inherently serialized per writer;
batching trades commit count for larger per-commit undo/redo scope. Batch
appends are all-or-nothing with LIFO undo on partial failure, and a failed
session's remaining events in the same batch are skipped rather than healed
over, matching pre-existing per-session failure isolation.

## Results

Implemented cross-session fixed-count (256-event) batching in
`AppendCommittedBatch`/`persistAppendBatchLocked`, a cancellable boot context
threaded through `provider.go` and `backendapp.ProvideWithStoreErrors`, and a
non-accumulating boot sweep (`syncAllCommittedSessionEventsAtBoot`).

Verification passed:

- `go test ./internal/plugins/... ./internal/backendapp/... -count=1` — all `ok`
  (plugins 18.3s, backendapp 35.0s).
- `TestMirrorCommitBatchingFloor`: per-event 0.442 ms/event vs per-256-events
  0.011 ms/event (~40x). `TestMirrorStartupCostPopulated`: 2,945-session/5,890-event
  shape mirrors in 130.5 ms.
- `make -C apps/backend lint` — 0 issues (repo-wide).
- Full `go test -tags fts5 ./...`: unrelated pre-existing failures only
  (`internal/worktree` and others already tracked as environment-load-sensitive
  known failures), none overlapping this diff's packages; reproduced
  independently at the merge-base.

No spec file changed: REQ-PLATFORM-STARTUP-LIFECYCLE-002 does not exist at this
base, and the existing AC-001.4/AC-001.6 already cover the behavior.
