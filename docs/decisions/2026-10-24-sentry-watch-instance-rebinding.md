# ADR-2026-10-24-sentry-watch-instance-rebinding: Mutable Sentry watch instance binding

**Status:** accepted
**Date:** 2026-10-24
**Area:** backend

## Context

ADR 0030 made a Sentry issue watch's instance binding immutable: changing it
meant creating a new watch. The constraint protects the watch's import history
(`sentry_issue_watch_tasks`), which is unique on `(issue_watch_id,
issue_short_id)`. Sentry short IDs are allocated per organization and project,
so regional instances with the same project slugs reuse short IDs for
unrelated issues. Carrying the history across instances would silently skip
new-instance issues.

Users run one application in several regions, each with a dedicated Sentry
instance, and want to move or copy a configured watch between instances.

## Decision

A watch's instance binding is mutable within its workspace through the
existing PATCH endpoint (`sentryInstanceId`). A change of the stored binding,
including an explicit binding of a legacy unbound watch, atomically:

- stores the new binding,
- deletes the watch's import history,
- clears `last_polled_at`, `last_error`, and `last_error_at`.

Existing tasks are kept unchanged. Always clear history when binding a legacy
unbound watch: its current resolved instance does not prove the origin of past
imports. Health changes and instance deletion can change legacy resolution,
even if only one instance remains now.

Reservation, task attachment, and release require both the event's polled
instance and a monotonic persisted scope revision to match the watch. Rebind
and Reset finalization advance that revision; update CAS compares the captured
revision so A->B->A cannot accept a stale update.
Updated edit clients also submit their immutable opening scope as an expected
revision. It is compared, never written: server pre-read CAS alone cannot
protect a draft opened before another completed rebind. Older callers may
omit the precondition and retain server-pre-read conflict semantics.

A Sentry-service-owned operation coordinator admits concurrent dispatches or
one exclusive rebind, Reset, or deletion, never both. Dispatch ownership spans
the actual asynchronous dispatch pipeline, not just bus delivery. Reset owns
the complete task-deletion operation without keeping a database lock across
I/O. Live-operation conflicts return 409 before destructive effects.
An empty SQL reservation is not proof of running dispatch; exclusive admission
allows safe cleanup after a failed attach or process exit. This relies on the
existing supported single-backend-replica runtime, including with Postgres.
All store mutations use parent-before-child locking.

Unbinding is not supported. ADR 0030's statement that the bound instance is
immutable is amended by this decision.

## Consequences

- Rebinding never suppresses a new-instance issue because of an old-instance
  short ID.
- Switching back to an earlier instance re-imports that instance's current
  matches, because its history was discarded.
- Reset after a rebind deletes only tasks recorded since the rebind. Sentry
  uses non-cascading task deletion, preserving non-recorded descendants,
  including older watch-created and archived tasks; direct surviving children
  are detached to top level. Recorded children are independent deletion
  targets. Other integrations keep their existing cascade behavior. Surviving
  watch-created tasks retain metadata and still count toward the in-flight cap.
- All three history mutations carry instance and scope revision across the
  orchestrator-to-Sentry boundary; migration/replay must preserve revisions.
- Reset, deletion, and rebind may require retry while dispatch is live. A
  paused Reset prevents rebinding for its full destructive duration.
- Abandoned empty reservations do not permanently block rebinding: only live
  process-owned dispatch admission does.
- The legacy bind may re-import issues from the same instance. This accepted
  cost is preferable to silently suppressing unrelated issues whose origin
  cannot be recovered.

## Alternatives Considered

### Keep the binding immutable

Rejected. Duplicate covers copying, but users also need to move an existing
watch without recreating its configuration and losing its row identity.

### Namespace import history per instance

Add an instance column to `sentry_issue_watch_tasks` and make uniqueness
instance-scoped. Switching back would not re-import, and Reset would still see
every task. Rejected for now: it needs a migration and backfill, ambiguous
handling for rows written by legacy unbound watches, and an instance-filtered
dedup read on every poll, for a switch-back case users have not requested.

### Keep legacy history when current resolution matches

Rejected. A healthy instance today may not be the one used by an earlier poll;
even a sole remaining instance does not establish history provenance.

### Instance-only guards and unfinished-reservation checks

Rejected. Instance equality misses A->B->A. An empty reservation cannot
distinguish slow live task creation from abandoned work, and Reset could erase
that reservation before creation resumes. Revision CAS and operation admission
are both needed: a revision alone cannot roll back an already-created task.

### Hold database locks through task creation and deletion

Rejected. Those calls perform nontransactional I/O and acquire other storage
locks. Process-owned admission constrains the operation while short parent-
first database transactions protect history and revision updates.

### Cascade every recorded task's descendants during Sentry Reset

Rejected. A current-history task can parent an old-instance or unrelated task;
tree-wide deletion would contradict the current-history boundary and the
confirmation's retention promise. Reuse the non-cascade deletion path, hardened
with an observed-child normalize/detach transaction under existing hierarchy
admission. A child arriving after enumeration must cause a safe conflict,
not be bulk-detached without workspace normalization. Keep the final
root-delete child check and conditional compensation; no new deletion engine
or database lock across lifecycle I/O is needed.
