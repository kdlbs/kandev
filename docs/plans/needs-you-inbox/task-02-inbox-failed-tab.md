---
id: "02-inbox-failed-tab"
title: "Inbox Failed tab delivery"
status: done
wave: 2
depends_on: ["01-needs-you-inbox-delivery"]
plan: "plan.md"
requirements:
  - REQ-UI-INBOX-FAILED-001
acceptance_criteria:
  - AC-UI-INBOX-FAILED-001.1
  - AC-UI-INBOX-FAILED-001.2
  - AC-UI-INBOX-FAILED-001.3
  - AC-UI-INBOX-FAILED-001.4
  - AC-UI-INBOX-FAILED-001.5
  - AC-UI-INBOX-FAILED-001.6
  - AC-UI-INBOX-FAILED-001.7
  - AC-UI-INBOX-FAILED-001.8
  - AC-UI-INBOX-FAILED-001.9
  - AC-UI-INBOX-FAILED-001.10
  - AC-UI-INBOX-FAILED-001.11
  - AC-UI-INBOX-FAILED-001.12
  - AC-UI-INBOX-FAILED-001.13
  - AC-UI-INBOX-FAILED-001.14
  - AC-UI-INBOX-FAILED-001.15
  - AC-UI-INBOX-FAILED-001.16
  - AC-UI-INBOX-FAILED-001.17
  - AC-UI-INBOX-FAILED-001.18
  - AC-UI-INBOX-FAILED-001.19
  - AC-UI-INBOX-FAILED-001.20
  - AC-UI-INBOX-FAILED-001.21
  - AC-UI-INBOX-FAILED-001.22
  - AC-UI-INBOX-FAILED-001.23
  - AC-UI-INBOX-FAILED-001.24
  - AC-UI-INBOX-FAILED-001.25
  - AC-UI-INBOX-FAILED-001.26
  - AC-UI-INBOX-FAILED-001.27
  - AC-UI-INBOX-FAILED-001.28
  - AC-UI-INBOX-FAILED-001.29
  - AC-UI-INBOX-FAILED-001.30
system_design:
  - ../../specs/ui/system-design/inbox-failed-bucket-01.md
---

# Task 02: Inbox Failed Tab Delivery

## Summary

Add the Inbox's second bucket: a Failed tab that lists tasks in the terminal
`FAILED` state, deliberately off the existing Needs-you badge. The Needs-you
tab's rows, ordering, count, answer affordances, dismiss/snooze, and error
state are untouched; the only permitted delta to that tab is one added empty
state copy clause (`AC-UI-INBOX-FAILED-001.23` / `.4`).

## In scope

- Backend: `internal/failedinbox` (bounded list read, reason resolution),
  `internal/task/repository/sqlite/failed_inbox_query.go`
  (SQLite/Postgres-compatible query), and flag-gated route registration
  (`features.needsYouInbox`, reusing the existing Inbox flag rather than
  adding a new one).
- Frontend: `failedInbox` store slice and selectors, `inbox-tab-strip`,
  `failed-inbox-tab-panel`/`-row`/`-empty-state`/`-error-state`,
  `useFailedInboxController`, i18n in all five shipped locales.
- Desktop and mobile E2E coverage for listing a failed task without inflating
  the sidebar's Needs-you count (AC .29).
- A workspace-revision fix (`office-routes.tsx`, `settings-routes.tsx`) for a
  Zustand hydration race the tab-strip's workspace-switch handling surfaced:
  capture `activeId` before `hydrate()`, keep `hydrate()`'s own `activeId`
  unchanged, then call `setActiveWorkspace()` explicitly when it differs, so
  `activeIdRevision` stays accurate.

## Out of scope

Re-ask, the history bucket, cross-workspace rows, and any change to the
current-turn predicate's meaning or the Needs-you tab's own row/ordering/count
behavior. See the requirements document's own "Out of scope" section for the
full list.

## Acceptance

All 30 acceptance criteria under `REQ-UI-INBOX-FAILED-001`; see
[requirements](../../specs/ui/requirements/inbox-failed.md) for the
full text. Notable ones with dedicated regression coverage:

- AC .14/.29: the sidebar's Needs-you count never moves when a failed task
  exists, checked before the Failed tab is ever selected -
  `apps/web/e2e/tests/chat/inbox-failed-tab.spec.ts`.
- AC .16: a re-visited workspace's stale Failed badge clears even when the
  workspace switch happened while the Inbox was closed throughout -
  `inbox-failed-tab.spec.ts` ("clears a re-visited workspace's stale badge...
  Inbox closed").
- AC .4/.23: the Needs-you empty state gains exactly one added copy clause and
  no other behavior change - `needs-you-inbox-empty-state.test.tsx`.
- AC .18: the open-task control stays reachable at every viewport width (no
  actions-menu fallback) - `failed-inbox-row.tsx` / `failed-inbox-row.test.tsx`.

## Verification

- `go build ./...`, `go vet ./...` clean (apps/backend).
- `go test ./internal/failedinbox/... ./internal/task/repository/sqlite/...`
  - all packages `ok`.
- `make -C apps/backend lint` - 0 issues.
- `pnpm run typecheck`, `pnpm exec eslint` (touched files) - clean.
- `pnpm exec vitest run` (targeted `failed-inbox`, Office/Settings
  workspace-revision suites) - all passing.
- `pnpm run i18n:check` / `pnpm run i18n:ratchet` - five locales complete, no
  em-dash violations, guard allowlist intact.
- `pnpm e2e:run` - desktop `inbox-failed-tab.spec.ts` and mobile
  `mobile-inbox-failed-tab.spec.ts` both pass with a full rebuild.
- `python3 scripts/lint-spec-files.py --all` - all specification files
  passed.
- Full local-suite pre-existing failures (macOS `/var`-symlink worktree
  issues, no local Docker daemon, wall-clock-timing tests under host load)
  proven pre-existing against `git merge-base HEAD origin/main` in a scratch
  worktree; unrelated to this branch's diff.

PR: https://github.com/kdlbs/kandev/pull/3683
