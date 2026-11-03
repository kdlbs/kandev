---
created: 2026-10-03
status: done
requirements:
  - REQ-TASKS-TASK-UNREAD-DOT-001
system_design:
  - ../../specs/tasks/system-design/task-unread-dot.md
legacy_specs: []
---

# Implementation Plan: Task unread dot

## Overview

Show a dot on task rows when a session of the task has an agent message the
user has not read, using the same cursor as the transcript's "New" divider; a
never-opened session counts from its first message. The backend projects the
task's unread session IDs into the existing task status summary, derived from
message and cursor ranks so event order cannot matter. The shared task row
decides display from local state (setting, open task, displayed session).
Order: the unread read model and its restore path, then the live projection,
list-load repair, and row dot (the last three after the read model), then
end-to-end coverage, then the maintainer-review changes in task 06.

The dot has an independent **Show unread dot on tasks** setting, defaulting off,
directly below **Show New divider in transcripts**. Its hide delay, shown only
while the dot is on, defaults to 2 seconds (whole seconds from 0 to 60);
selecting an unread row starts a presentation-only linger, without delaying the
read cursor. Desktop and phone rows share this behavior.

## Scope

### In scope

- `unread_messages` in the task status summary, fed by message events, a new
  read-cursor event, session removal, and restore and repair on list load.
- The dot on task rows for the desktop sidebar and the mobile task switcher,
  with its accessible name and localized copy in all seven locales.
- Updated setting copy and public documentation for the new behavior.

### Out of scope

- Unread counts, kanban card dots, tab-title or OS notifications.
- Changes to the read cursor write rules, `mark-read` request, or the divider.
- Quick Chat indicators.
- Session-level dots inside a task; the open task's row keeps the task dot
  while another of its sessions is unread.

## Technical approach

### Unread read model (task 01)

- `apps/backend/internal/task/statussummary/`: the qualifying-message
  predicate; `UnreadMessages` on `TaskStatusSummary` (and
  `SemanticEqual`/`SemanticJSON`); cursor and newest ranks on
  `sessionObservation` (microsecond-normalized); derivation in `deriveSummary`;
  rank fields on `RebuildSession`; `UnreadObserved` on
  `SessionObservationSnapshot`; `LoadSessionUnreadState` loader and the
  `task_status_summary_unread_load_failures_total` counter.
- Task repository `ListSessionUnreadState`, bounded below by the cursor rank,
  evaluating the Go predicate over loaded pages; the narrow
  `SessionUnreadStateRepository` interface it implements (not added to
  `SessionRepository`, so the handler test fakes keep compiling); wired into
  `loadTaskSessionObservations` (`backendapp/gateway.go`), degrading to
  unobserved on read failure. `projectionState.unreadObserved` and an unread
  baseline seeded in `applySummaryBaseline` keep a `true` aggregate when a
  restore or rebase read fails.
- `apps/packages/types/src/unread-message-visible-types.json` is the canonical
  transcript-visible type set. The web filtering Set imports that manifest
  through an explicit `@kandev/types` JSON export; the Go predicate keeps a
  membership set whose exact contents are checked against the same manifest.
  Task 01 tests the backend set equality and exercises each manifest type
  through `filterVisibleMessages`.

### Live projection (task 02)

- `apps/backend/internal/events/types.go`: `TaskSessionReadCursorChanged`.
- `service_sessions.go`: `MarkSessionRead` publishes it after every successful
  write with the persisted cursor and its `created_at`.
- Projector: handling for `MessageAdded`, the cursor event (raise, or replace on
  `reset: true`), `MessageDeleted`, and `SessionRemoved`; subscriptions in
  `Projector.Start`; `task_status_summary_unread_flips_total`; flip debug log.
- `backendapp/e2e_reset.go`: the e2e read-cursor route publishes the cursor
  event with `reset: true`.

### Repair on list load (task 03)

- `service_status_summary_rebuild.go`: a narrow `SessionUnreadStateRepository`
  interface type-asserted on the service's session repository (the
  `completionGateSummary` pattern, no setter), used by `rebuildMissingSummaries`
  and by `reconcileExistingSummary`/`summaryNeedsReconcile`. A CAS-rejected
  retry re-reads unread for that task.

### Row dot (task 04)

- `apps/web/lib/types/task-status-summary.ts`: `unread_messages`.
- `TaskSwitcherItem.hasUnreadMessages` (`task-switcher-types.ts`), set by
  `sidebarStatus` (`task-session-sidebar-item.ts`) and `sheetStatus`
  (`mobile/session-task-switcher-sheet-item.ts`).
- Display rule in `task-switcher-row.tsx` from the explicit
  `unreadMessagesEnabled` prop (threaded through `TaskSwitcherProps`,
  `buildTaskRowProps`, `TaskRowBaseProps`, and `task-switcher-tree.tsx`) and
  `isSelected`; `TaskItem` prop; `TaskUnreadDot` in
  `task-item-leading-badges.tsx`.
- `task:taskHasUnreadMessages` and updated `settings:unreadShort` copy in seven
  locales (`pnpm run i18n:zh-hant` for the Traditional Chinese pair, then
  `pnpm run i18n:pseudo`); `docs/public/configuration.md` and
  `docs/public/operations.md` via `/docs-maintainer`.

### Compatibility

| Surface | Behavior |
| --- | --- |
| Old summary rows | Decode with `false`; corrected by the list-load reconcile. |
| Old clients | Ignore the unknown field. |
| Divider disabled | Backend still projects; the independent unread dot setting controls display. |
| Nil projector loader | Preserve a stored unread aggregate as an unobserved baseline; omit only when neither an authoritative loader nor stored summary exists. |
| SQLite and Postgres | Ranks truncated to microseconds so event and database ranks agree. |

## ASCII UI preview

### UI-01: Task row with unread dot

Entry: task list (desktop sidebar or mobile switcher), a task other than the
open one, Show unread dot on tasks on. The dot is the last badge in the title line, the
same slot family as the agent-error icon. Structure (last badge, no extra line)
is required; spacing and color are illustrative within existing design tokens.

```text
Tasks
+----------------------------------+
| o  Fix login redirect            |   normal row
| o  Add retry to uploader   (*)   |   unread dot, role=img, labelled
| >  Refactor auth  (open)   (*)   |   opened unread task: dot lingers, then clears
| o  Flaky CI test            (!)  |   error icon; dot would follow it
+----------------------------------+
(*) small filled dot; aria-label "Task has unread messages"
```

Phone: the task switcher sheet lists the same rows through the same
`TaskItem`. The dot keeps its position, and the existing row height and touch
target do not change.

## Tests

| AC | Evidence |
| --- | --- |
| `.1`, `.2`, `.9` | `projector_unread_test.go` (statussummary, task 01): rank derivation, multi-session, no cursor |
| `.6` | `projector_unread_test.go` (task 02): cursor event before and after its own `MessageAdded` converges to read; a newer message re-arms. `unread_predicate_test.go`: equal ids are read before the time compare, and event and stored ranks normalize to the same microsecond. `repository/sqlite/session_unread_state_test.go` (task 02): live `MessageAdded`, then the cursor event, ends read and a replay does not re-arm, on SQLite and (gated on `KANDEV_TEST_POSTGRES_DSN`) Postgres. It lives in package `sqlite` because `statussummary` tests cannot import `sqlite` without a cycle. `service_status_summary_unread_test.go`: `MarkSessionRead` publishes the persisted cursor, including for a stale request. |
| `.7` | `projector_unread_test.go` (task 02): qualifying `MessageAdded` publishes an aggregate flip; forced lower cursor (`reset`) reloads ranks and re-arms |
| `.8`, `.11`, `.12` | `projector_unread_test.go` (task 02): failed task-wide observation followed by qualifying background `MessageAdded` and, separately, a cursor event reloads all sessions (including an unread sibling); `SessionRemoved` clears a session on successful reload. Its failed reload preserves the stale aggregate until a later successful reload clears a dot caused only by the deleted session. A `MessageDeleted` pending-request clear applies despite unread reload failure. Hidden (`hide_panel`) sessions are a frontend tab state that the projector never sees, so they keep contributing by construction. `projector_unread_restore_test.go` (task 01): failed read keeps a `true` baseline on cold restore and rebase, and a nil loader preserves the stored aggregate |
| `.8` | `service_status_summary_unread_test.go` (task 03): rebuild missing rows as unread, repair stale `false → true` and `true → false` existing aggregates, and resolve a rejected CAS using the winning row plus a fresh unread read so a racing mark-read remains `false`. |
| `.10` | `unread_predicate_test.go` (task 01): table over every visible type and excluded cases, plus exact equality between Go's accepted type set and `apps/packages/types/src/unread-message-visible-types.json`; `processed-message-filtering.test.ts` verifies each manifest type is visible in the frontend |
| `.1`, `.3`, `.4`, `.5`, `.13`, `.14`, `.15` | `task-switcher-unread-dot.test.tsx` renders rows through the shared `TaskSwitcher` chain (setting gate, open-task rule, localized `role="img"` name); `task-switcher-row-render-identity.test.tsx` and `task-switcher-render-stability.test.tsx` preserve provider-free rendering while passing the setting explicitly. The two row builders only copy the flag, so they get no separate mapping test. |
| `.16` | Playwright bounding-box equality assertion in both e2e specs |
| `.2`, `.18` | `unread_predicate_test.go` and `projector_unread_test.go` (task 06): user-written rows never qualify; a cursorless session with agent output is unread, including one created after restore. `session_unread_state_test.go` (task 06): every existing session is listed, cursorless sessions with a zero read rank. |
| `.5`, `.17` | `task-unread-dot-visibility.test.ts` (task 06): the open task keeps its dot only while a non-displayed session is unread; `task-switcher-unread-dot.test.tsx` renders it through the switcher chain. |
| `.6` | `session-unread-divider.test.ts` (task 06): the acknowledged id is the newest loaded message even when the transcript hides it; the divider cursor maps a hidden cursor onto the newest rendered message before it. |
| `.19` | `unread-task-dot-settings.test.tsx` (task 06): the delay row renders only while the dot switch is on and states that it changes appearance only. |
| Cost | `projector_unread_hold_test.go` (task 06): a rise followed by a prompt mark-read within the hold writes nothing; a rise that stays unread flushes once. |

## E2E tests

`apps/web/e2e/tests/task/task-unread-dot.spec.ts` (`chromium` project) covers
`.1`, `.2`, `.3`, `.5`, `.6`, `.7`, `.8`, `.13`, `.14`, `.16`, `.18`.
`apps/web/e2e/tests/task/mobile-task-unread-dot.spec.ts` (`mobile-chrome`
project) covers `.1`, `.15`, `.16`.

## Work orders

- [x] [Task 01: Build the unread read model](task-01-unread-read-model.md)
- [x] [Task 02: Project live unread updates](task-02-live-unread-projection.md)
- [x] [Task 03: Repair unread state on task list load](task-03-unread-list-repair.md)
- [x] [Task 04: Show the unread dot on task rows](task-04-task-row-unread-dot.md)
- [x] [Task 05: Unread dot end-to-end coverage](task-05-unread-dot-e2e.md)
- [x] [Task 06: Apply maintainer review changes](task-06-maintainer-review-changes.md)

## Verification results

- `go test` passed for `internal/task/statussummary/...`,
  `internal/task/repository/sqlite/...`, `internal/backendapp/...`, and
  `internal/events/...`. `internal/task/service` passed except eight local
  directory and repository-initialization tests that fail with "parent
  directory cannot be accessed" on an unmodified `HEAD` worktree in this
  environment; every unread, reconcile, and `MarkSessionRead` test passed.
  Postgres-gated tests skipped (no `KANDEV_TEST_POSTGRES_DSN`).
- `golangci-lint run` on the changed backend packages: 0 issues.
- Web: `tsc --noEmit`, `pnpm run i18n:check`, `pnpm run i18n:ratchet`, eslint
  and prettier on changed files, and vitest for the task switcher, row
  builders, and transcript filtering (223 and 58 tests) passed.
- E2E: `task-unread-dot.spec.ts` (`chromium`) and
  `mobile-task-unread-dot.spec.ts` (`mobile-chrome`) passed in the
  single-worker host runner.
- `python3 scripts/list-docs.py validate` and
  `python3 scripts/lint-spec-files.py --all` passed.
- Configurable-delay checks: focused user/settingscatalog Go tests, 25 focused
  web tests, both unread-dot E2E specs, i18n, generated catalog, and typecheck
  passed. Typecheck required `NODE_OPTIONS=--max-old-space-size=8192` in this
  environment.
- The rebased commit hook exposed existing `backendapp` duplicate `review` and
  `http` literals. Local constants preserve their values without bypassing
  lint. Repository-label tests moved to a sibling suite to keep the switcher
  test file below its existing line limit.


## Risks

- Exact parity with render-time filtering is no longer required: read
  tracking acknowledges the newest loaded message, so reading a session to its
  end clears it even when the transcript hides its newest rows (AC .6).
- A newly unread session's summary write is held for 1.5 seconds so a prompt
  mark-read of the displayed session coalesces with it. Measured costs are in
  task 06.
- Each CAS rebase and each list load reads unread state for every session of
  the listed tasks, including cursorless ones; task 06 measures the worst case.
- Enabling the dot marks every listed task whose sessions hold unread agent
  output, including tasks the user never opened before the setting existed.
- A hidden (`hide_panel`) session keeps the dot until reopened and read.
- The shared visible-message JSON manifest and Task 01 parity tests prevent the
  frontend and Go predicate from accepting different message-type sets.
