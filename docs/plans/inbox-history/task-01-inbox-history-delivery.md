---
id: "01-inbox-history-delivery"
title: "Inbox History delivery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-INBOX-HISTORY-001
acceptance_criteria:
  - AC-UI-INBOX-HISTORY-001.1
  - AC-UI-INBOX-HISTORY-001.2
  - AC-UI-INBOX-HISTORY-001.3
  - AC-UI-INBOX-HISTORY-001.4
  - AC-UI-INBOX-HISTORY-001.5
  - AC-UI-INBOX-HISTORY-001.6
  - AC-UI-INBOX-HISTORY-001.7
  - AC-UI-INBOX-HISTORY-001.8
  - AC-UI-INBOX-HISTORY-001.9
  - AC-UI-INBOX-HISTORY-001.10
  - AC-UI-INBOX-HISTORY-001.11
  - AC-UI-INBOX-HISTORY-001.12
  - AC-UI-INBOX-HISTORY-001.13
  - AC-UI-INBOX-HISTORY-001.14
  - AC-UI-INBOX-HISTORY-001.15
  - AC-UI-INBOX-HISTORY-001.16
  - AC-UI-INBOX-HISTORY-001.17
  - AC-UI-INBOX-HISTORY-001.18
  - AC-UI-INBOX-HISTORY-001.19
  - AC-UI-INBOX-HISTORY-001.20
  - AC-UI-INBOX-HISTORY-001.21
  - AC-UI-INBOX-HISTORY-001.22
  - AC-UI-INBOX-HISTORY-001.23
  - AC-UI-INBOX-HISTORY-001.24
  - AC-UI-INBOX-HISTORY-001.25
  - AC-UI-INBOX-HISTORY-001.26
  - AC-UI-INBOX-HISTORY-001.27
  - AC-UI-INBOX-HISTORY-001.28
  - AC-UI-INBOX-HISTORY-001.29
  - AC-UI-INBOX-HISTORY-001.30
  - AC-UI-INBOX-HISTORY-001.31
system_design:
  - ../../specs/ui/system-design/inbox-history.md
---

# Task 01: Inbox History Delivery

## Summary

Add a second, read-only "History" tab to the Inbox listing bundles that are
non-terminal, unarchived, not `parent_question` records, and unanswerable for
exactly one of three named reasons (`superseded`, `session_ended`,
`unreadable`). The read path is strictly additive, lives in
`apps/backend/internal/task/repository/sqlite` reusing that package's
unexported predicates, and its isolation is enforced by a new
source-text-scan architecture-lint rule. The count is a bundle count that
renders on the tab and never feeds the sidebar/Needs-you badge.

## In scope

- Backend: an additive Inbox History bundle read path
  (`ListInboxHistoryBundles`), a new HTTP read endpoint, and Postgres coverage
  for the dialect-sensitive query.
- `ARCH-INBOX-HISTORY-ISOLATION` architecture-lint rule scanning a closed
  sink-file set and a scoped source-file set.
- Frontend: History tab strip, state slice, row/detail components, and i18n
  in all five shipped locales.
- Desktop and mobile E2E coverage for a superseded bundle.

## Out of scope

Re-asking or resurfacing an unanswerable question, any change to the sidebar
or Needs-you unread badge, and any migration or backfill for bundles that went
stale before this shipped.

## Acceptance

All 31 acceptance criteria under `REQ-UI-INBOX-HISTORY-001`; see
[requirements](../../specs/ui/requirements/inbox-history.md) for the full
text. Notable ones with dedicated regression coverage:

- AC .2/.3: bundle-level eligibility and mutual exclusion between the Needs-you
  and History buckets, including `permission_group_key` disambiguation of a
  reused permission `pending_id` at both the SQL classification layer and the
  Go hydration layer.
- AC .5/.17/.22: the read's isolation, enforced by the architecture-lint rule
  plus a same-package fixture and a runtime test asserting the History
  controller never subscribes to the live WebSocket event stream.
- AC .14/.29: open-task and copy-id are the only actions, with no
  answer/dismiss/snooze affordance — `inbox-history-superseded.spec.ts` and
  `mobile-inbox-history-superseded.spec.ts` (44px touch target, no horizontal
  clipping).
- AC .16/.17: the History tab's own bundle count never feeds the sidebar or
  Needs-you unread badge — `app-sidebar-primary-nav.test.tsx`.
- AC .30: a row presents what was actually asked, keyed by
  `permission_group_key` so a reused `pending_id` cannot render merged or
  wrong content.

## Verification

- `make fmt`, `make typecheck` — clean.
- `make test` (backend Go) — two pre-existing failure clusters
  (`internal/worktree`, one `internal/task/service` gitflow test) proven
  pre-existing against `git merge-base HEAD origin/main` in a scratch
  worktree; unrelated to this branch's diff (macOS `/var/folders`
  symlink-resolution artifact).
- `make lint` — golangci-lint 0 issues, eslint clean, harness/spec/architecture
  lints clean.
- `pnpm --filter @kandev/web test`, `pnpm --filter kandev test` (CLI) — clean
  aside from one pre-existing, unrelated failure requiring a local Docker
  daemon.
- `make test-scripts` — one pre-existing failure (`scripts/pr-await.test.sh`),
  proven against the merge-base.
- `pnpm run i18n:check` / `pnpm run i18n:ratchet` — five locales complete, no
  em-dash violations, guard allowlist intact.
- `pnpm e2e:run` — desktop `inbox-history-superseded.spec.ts`,
  `needs-you-inbox-answer.spec.ts`, and mobile
  `mobile-inbox-history-superseded.spec.ts` all pass.
- `python3 scripts/lint-spec-files.py --all` — all specification files
  passed.

PR: https://github.com/kdlbs/kandev/pull/3679
