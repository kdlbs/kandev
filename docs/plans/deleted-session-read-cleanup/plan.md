---
created: 2026-10-27
status: implemented
requirements:
  - REQ-UI-DELETED-TASK-CLIENT-STATE-001
system_design:
  - ../../specs/ui/system-design/deleted-task-client-state.md
legacy_specs: []
---

# Implementation Plan: Deleted Session Read Cleanup

## Overview

Stop Git-read retry work after task deletion and classify a genuinely missing
session as permanent. The incident produced repeated errors immediately after a
`task.deleted` event because stale client reads treated `INTERNAL_ERROR` as a
transient failure. The frontend may retain a consumer after deletion; whether
that particular client received the event remains unknown.

## Scope

Retire only reads belonging to deleted task sessions, reject late results, and
return a permanent response for missing sessions even if the event was missed.
Preserve other sessions, shared environments, transient recovery, and existing
navigation. No visual or mobile layout change is intended.

## Technical approach

Connect the deletion handler and session-read coordinator using explicit session
identity. Clear retry timers, invalidate in-flight responses, and prevent still
mounted consumers from reviving retired reads. Map only the typed missing-session
error at the Git WebSocket handler boundary to `NOT_FOUND`; avoid exposing the
error detail. Keep other failures and workspace-not-ready responses unchanged.

## Tests

Exercise deletion with active retry timers and in-flight replies, including a
live session sharing an environment and the no-environment fallback. Exercise
both Git endpoints with missing and transient failures. Existing `NOT_FOUND`
frontend tests protect the missed-event path.

## E2E tests

Extend `apps/web/e2e/tests/task/deleted-task-client-state.spec.ts` to observe
WebSocket Git requests for a deleted task while another task remains usable;
run the chromium project. This covers AC-001.4 and AC-001.5 without changing
rendered UI.

## Work orders

- [x] [Task 01: Stop deleted session reads](task-01-stop-deleted-session-reads.md)

## Verification results

Go Git handler tests passed. Three frontend test files passed (36 tests), the
web typecheck and changed-file lint passed, and both Chromium scenarios passed
with retries disabled. The first final browser run hit a 401 during isolated
fixture setup; a subsequent run passed both tests. Specification catalog and
lint checks passed. Exact commands are recorded in the work order.

## Risks

A session can share its environment with another live session; cleanup must not
retire the survivor. A late deletion must not clear a replacement session. The
connection scope retains one deleted-session ID per observed deletion until the
scope is replaced, even after the associated read objects have been released.
