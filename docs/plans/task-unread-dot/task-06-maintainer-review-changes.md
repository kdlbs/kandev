---
id: "06-maintainer-review-changes"
title: "Apply maintainer review changes"
status: done
wave: 4
depends_on: ["05-unread-dot-e2e"]
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-UNREAD-DOT-001
acceptance_criteria:
  - AC-TASKS-TASK-UNREAD-DOT-001.2
  - AC-TASKS-TASK-UNREAD-DOT-001.5
  - AC-TASKS-TASK-UNREAD-DOT-001.6
  - AC-TASKS-TASK-UNREAD-DOT-001.17
  - AC-TASKS-TASK-UNREAD-DOT-001.18
  - AC-TASKS-TASK-UNREAD-DOT-001.19
system_design:
  - ../../specs/tasks/system-design/task-unread-dot.md
---

# Task 06: Apply maintainer review changes

## Summary

Apply the five changes requested on
[issue #4757](https://github.com/kdlbs/kandev/issues/4757#issuecomment-6481624047):
never-opened sessions notify, the open task keeps signalling unread work in its
other sessions, reading a session to its end always clears it, the hide delay
is shorter and lives behind the enable switch, and the backend cost is
measured and reduced where it was high.

## In scope

- Predicate: user-written rows never qualify; a cursorless session counts
  from its first qualifying message. The New divider keeps its first-visit
  rule.
- Summary: `unread_session_ids` (sorted, unique, bounded) replaces
  `unread_messages`; the repository reads every existing session, cursorless
  ones with a zero read rank; reconcile and restore compare and seed lists.
- Row display: `shouldShowUnreadDot` keeps the open task's dot while a session
  other than the displayed one is unread; owners pass `displayedSessionId`
  through the explicit switcher prop chain.
- Read acknowledgement: the chat panel acknowledges the newest loaded message,
  including rows the transcript hides; the divider maps a hidden cursor onto
  the newest rendered message before it.
- Settings: the hide delay defaults to 2 seconds, renders only while the dot
  switch is on, and its copy says it changes appearance only.
- Cost: a 1.5-second rise hold in the projector and a SQL candidate prefilter
  in the unread queries, with measurements recorded in the system design.

## Out of scope

- Session-level dots inside a task.
- Changes to the cursor write guard or the `mark-read` request.

## Acceptance

- Each listed AC has a test that fails when its behavior breaks.
- Measurements for viewed and unviewed streams and for long-history list loads
  are recorded in the system design's Cost section.

## Verification

```bash
(cd apps/backend && go test ./internal/task/statussummary/... ./internal/task/repository/sqlite/... ./internal/task/service/... ./internal/backendapp/... ./internal/user/... ./internal/settingscatalog/...)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/task/task-unread-dot.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-task-unread-dot.spec.ts)
```

## Files likely touched

- `apps/backend/internal/task/statussummary/` (predicate, model, projector,
  hold, tests)
- `apps/backend/internal/task/repository/sqlite/session_unread_state.go`
- `apps/backend/internal/task/service/service_status_summary_*.go`,
  `apps/backend/internal/backendapp/gateway.go`
- `apps/web/components/task/` (row display, visibility helper, chat panel),
  `apps/web/lib/session-unread-divider.ts`,
  `apps/web/components/settings/unread-task-dot-*.tsx`
- Locale catalogs, specs, and public docs

## Dependencies

`05-unread-dot-e2e`.

## Risks

- Enabling the dot marks every task whose sessions hold unread agent output,
  including tasks never opened before the setting existed.
- A long run of rows that pass the SQL filter but fail a metadata check is
  still paged into Go.

## Parallelism

`sequential`

## Inputs

- Maintainer comment on issue #4757.

## Results

Done. Unread `statussummary`, `repository/sqlite`, `service`, and `backendapp`
tests, the `user` and `settingscatalog` suites, web typecheck, `i18n:check`,
eslint on the changed files, and the affected Vitest suites passed. Both unread
e2e specs passed in the single-worker host runner (desktop 39.6s, mobile
16.9s). The cost measurements in the system design came from a throwaway
harness that was deleted afterwards; the SQL candidate prefilter reduced the
no-qualifying-row list load from 4.1s to about 150ms for 40 sessions of 5000
rows.

PR review follow-up: the hide delay control stays mounted while hidden so an
unsaved delay edit survives toggling the dot switch; both unread setting
targets and save contributors map to the Conversation tab; and both e2e specs
arm `watchWs().waitForEvent("task.status_summary.updated")` before seeding
instead of waiting on the UI with a long timeout.
