---
created: 2026-09-15
status: complete
requirements:
  - REQ-UI-INBOX-HISTORY-001
system_design:
  - ../../specs/ui/system-design/inbox-history.md
legacy_specs: []
---

# Implementation Plan: Inbox History

## Overview

Add a second, read-only "History" tab to the Inbox that surfaces clarification
and permission bundles that became unanswerable (superseded, session ended, or
unreadable), per [requirements](../../specs/ui/requirements/inbox-history.md)
and [system design](../../specs/ui/system-design/inbox-history.md).

This feature was built through Kandev's own spec-driven-development workflow
(4 spec-review rounds, 4 build rounds, security/test/code review across
multiple rounds, then PR integration) with its running record kept in the
task's own plan store rather than as narrative here. This document exists to
satisfy the repository's PR documentation coverage gate
(`ci: enforce pull request documentation coverage`), which was not wired into
that workflow when this feature's spec canon was authored.

## Scope

### In scope

- A strictly additive backend read path in
  `apps/backend/internal/task/repository/sqlite`, reusing that package's
  unexported predicates (`currentTurnAuthority`, `nonTerminalSessionPredicate`).
- A new `ARCH-INBOX-HISTORY-ISOLATION` architecture-lint rule enforcing the
  read path's isolation via a source-text scan (closed sink-file set plus a
  scoped source-file set).
- A read-only History tab strip, row list, and clarification-detail expansion
  on the existing Inbox surface, with open-task and copy-id as the only
  actions.
- i18n in all five shipped locales.
- Desktop and mobile E2E coverage for a superseded bundle.

### Out of scope

Re-asking or resurfacing an unanswerable question, any change to the sidebar
or Needs-you unread badge, and any migration or backfill for bundles that went
stale before this shipped. See the requirements document's own "Out of scope"
section for the full list.

## Work orders

- [x] [Task 01: Inbox History delivery](task-01-inbox-history-delivery.md)

Single work order; no dependents.

## Verification results

See the work order for the full verification record. Summary: backend and
frontend build/lint/typecheck clean; targeted and full package test suites
pass (pre-existing, unrelated failures proven against merge-base in a scratch
worktree); desktop and mobile E2E specs for the History tab pass; `pnpm run
i18n:check` and `pnpm run i18n:ratchet` clean; `python3
scripts/lint-spec-files.py --all` passes.

PR: https://github.com/kdlbs/kandev/pull/3679
