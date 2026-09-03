# ADR-2026-09-02-task-owned-plan-comments: Persist Pending Plan Comments with the Task Plan

**Status:** accepted
**Date:** 2026-09-02
**Area:** backend, frontend, protocol, persistence

## Context

A task plan is shared by every Agent session in a task, but pending comments on
that plan were stored in browser `sessionStorage` under the selected session.
That mismatch made session navigation a data-ownership boundary. It first
caused comments to be deleted during editor projection reconciliation, and the
repair still left identical plan content showing different comments depending
on the selected tab.

The user may want either to send a message with the plan feedback to a chosen
non-primary session, or to use the comment's **Run** shortcut for the task's
primary session. Those are delivery choices, not reasons for the pending
feedback itself to belong to a session.

## Decision

Kandev persists one pending plan-comment collection against the task's current
plan. The backend is authoritative, and every authorized Plan view and task
session composer projects that same collection. Session creation, selection,
deletion, and primary promotion do not change it.

Pending comments become agent context only through explicit delivery. Ordinary
composer **Send** addresses the selected session. Plan-comment **Run** addresses
the current primary session. Run validates primary ownership when delivery is
accepted, so a tab selection or concurrent primary change cannot send feedback
to the wrong session.

The backend expands trusted persisted comment rows into the visible prompt and
consumes exactly those rows in the same transaction that accepts the direct
message or queued prompt. An acceptance failure preserves them. Successful
delivery removes them task-wide; the transcript or queue entry is the durable
record, not a second sent-comment archive.

Only plan comments adopt task ownership. Diff, file, pull-request,
walkthrough, and agent-message comments retain their current session-scoped
lifecycles.

## Consequences

- A plan shows the same pending annotations regardless of selected or primary
  session, reload, browser, or backend restart.
- Every session composer can deliberately send its message with the shared plan
  context, including to a non-primary session.
- **Run** has a stable task-level meaning and cannot accidentally follow the
  selected tab.
- Comment CRUD, synchronization, authorization, schema migration, and prompt
  formatting move to the backend.
- Direct-message and queue admission gain a common conditional-consumption
  boundary and caller-owned idempotency for comment-bearing queue entries.
- A concurrent losing send fails closed instead of silently dropping the
  comment context from its typed message.
- Legacy browser drafts require a one-time idempotent migration.
- The composer context item cannot act as a per-session detach toggle. Deletion
  remains an explicit action on the shared plan annotation.

## Alternatives considered

1. **Keep one browser-local collection per session.** Rejected because session
   identity remains unrelated to the shared plan and users still see different
   annotations on the same document.
2. **Keep one browser-local collection per task.** Rejected because reload in a
   new tab, another browser, or another authorized user still loses the shared
   state, and delivery cannot be consumed atomically.
3. **Copy each new comment to every existing session.** Rejected because new
   sessions would need backfill, editing would require fan-out, and independent
   copies would drift or be delivered repeatedly.
4. **Always send comments to every session.** Rejected because comments are
   pending user intent, not ambient broadcast context, and the user needs one
   deliberate destination.
5. **Make ordinary Send follow the primary session.** Rejected because the
   selected composer is an explicit addressing action. Only **Run** is defined
   as the primary-session shortcut.
6. **Delete comments in a follow-up request after message acceptance.** Rejected
   because crashes and concurrent sends create duplicate or ambiguous delivery
   windows. Prompt acceptance and consumption are one repository transaction.
