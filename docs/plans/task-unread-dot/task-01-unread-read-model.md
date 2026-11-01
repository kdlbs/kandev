---
id: "01-unread-read-model"
title: "Build the unread read model"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-UNREAD-DOT-001
acceptance_criteria:
  - AC-TASKS-TASK-UNREAD-DOT-001.1
  - AC-TASKS-TASK-UNREAD-DOT-001.2
  - AC-TASKS-TASK-UNREAD-DOT-001.6
  - AC-TASKS-TASK-UNREAD-DOT-001.8
  - AC-TASKS-TASK-UNREAD-DOT-001.9
  - AC-TASKS-TASK-UNREAD-DOT-001.10
system_design:
  - ../../specs/tasks/system-design/task-unread-dot.md
---

# Task 01: Build the unread read model

## Summary

The task status summary carries `unread_messages`, derived from per-session
cursor and newest-message ranks that restore from the database. After a restart
or CAS rebase the aggregate matches the stored messages and cursors.

## In scope

- The qualifying-message predicate and its table-driven test; the Go membership
  set must exactly match `apps/packages/types/src/unread-message-visible-types.json`.
- That JSON is the canonical visible-type list. The frontend derives
  `VISIBLE_MESSAGE_TYPES` from it through `@kandev/types`, with the JSON package
  export added to `apps/packages/types/package.json`; a frontend test exercises
  every manifest type as visible.
- Microsecond-normalized ranks, `UnreadMessages` on `TaskStatusSummary`
  (semantic equality and JSON), rank state on `sessionObservation`, derivation
  in `deriveSummary`.
- Repository `ListSessionUnreadState` (bounded walk, caught-up shortcut), the
  `LoadSessionUnreadState` loader, `RebuildSession` rank fields, and
  `loadTaskSessionObservations` restore.
- Degrading a failed unread read to `UnreadObserved == false`, with
  `projectionState.unreadObserved`, an unread baseline seeded in
  `applySummaryBaseline`, and the
  `task_status_summary_unread_load_failures_total` counter.

## Out of scope

- Live event handling and the cursor event (task 02), list-load repair
  (task 03), and task-row/localization behavior.

## Acceptance

- A cursor-bearing session restores `newestKnown` with the newest qualifying
  rank, or the cursor rank when nothing qualifies after it; a cursorless
  session projects no field.
- Event-derived and database-derived ranks for the same message compare equal
  on SQLite and Postgres.
- A failed unread read keeps the other summary fields and the previous unread
  value: with a stored `true` aggregate, a failed read on cold restore or on CAS
  rebase still yields `true`.
- The Go predicate's accepted visible-type set exactly equals the shared JSON
  manifest; the web filtering test confirms every manifest type is rendered as
  visible. Adding or removing a type on either implementation side fails tests
  until the shared contract and both consumers agree.

## Verification

```bash
(cd apps/web && pnpm exec vitest run hooks/processed-message-filtering.test.ts)
(cd apps/backend && go test ./internal/task/statussummary/... ./internal/task/repository/sqlite/... ./internal/backendapp/...)
(cd apps/backend && make lint)
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `apps/backend/internal/task/statussummary/{model,projector,projector_events,projector_derive,rebuild,metrics_vars}.go`
- `apps/backend/internal/task/statussummary/{unread_predicate,unread_predicate_test,projector_unread_test,projector_unread_restore_test}.go`
- `apps/backend/internal/task/repository/sqlite/session.go` (or a message repository file) and its test
- `apps/backend/internal/task/repository/interface.go` (a new narrow `SessionUnreadStateRepository`, not added to `SessionRepository`)
- `apps/backend/internal/backendapp/gateway.go`
- `apps/packages/types/src/unread-message-visible-types.json` (shared exact contract)
- `apps/packages/types/package.json` (JSON export)
- `apps/web/hooks/processed-message-filtering.ts` and its test

## Dependencies

None.

## Risks

- Postgres parity: run the Postgres-gated tests when `KANDEV_TEST_POSTGRES_DSN`
  is set.
- A deleted-cursor session walks without a lower bound; keep pages bounded.
- `rebaseProjectionStateFromCurrent` must restore the rank fields through the
  loader, or a rebase drops them.

## Parallelism

`sequential`

## Inputs

- System design: Unread predicate, Ranks, Restore and repair.
- `UpdateTaskSessionLastReadMessageID` ordering, `restoreSessionObservations`,
  `applySummaryBaseline`, `useProcessedMessages` footer-action split.

## Results

Done. `statussummary/unread_predicate.go` holds the predicate, normalized
`UnreadRank`, and `SessionUnreadState`; the Go type set uses `models.MessageType`
constants and `TestUnreadVisibleTypesMatchSharedManifest` requires exact
equality with `apps/packages/types/src/unread-message-visible-types.json`, which
`processed-message-filtering.ts` imports through the new `@kandev/types` JSON
export. `sqlite/session_unread_state.go` implements `ListSessionUnreadState`
(cursor ranks via one join, 64-row backwards pages bounded by the cursor with
stored-value comparisons, no lower bound for a deleted cursor). The gateway
fills `RebuildSession.Unread` and degrades a failed read to
`UnreadObserved == false` plus `RecordUnreadLoadFailure`. Per-session ranks live
in `sessionObservation.unread`; `projectionState.unreadObserved` and
`unreadBaseline` keep a stored `true` on failed cold restore, CAS rebase, and a
nil loader. Verified by `go test ./internal/task/statussummary/...`,
`TestListSessionUnreadState`, and `vitest hooks/processed-message-filtering.test.ts`
(58 tests). Postgres variants skip without `KANDEV_TEST_POSTGRES_DSN`.
