---
created: 2026-10-22
status: complete
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
system_design:
  - ../../specs/ui/system-design/task-navigation-responsiveness.md
legacy_specs: []
---

# Implementation Plan: Session File-Review Snapshot Reconciliation

## Overview

Preserve local review intent against an older initial read while loading
unrelated server rows. One sequential work order first adds causal RED through
the real hook/protocol/consumer, then repairs only initial-read reconciliation.
The caller explicitly authorizes package plus TDD in this turn.

UI owns client publication reconciliation; task storage and review APIs retain
their existing authority. Refine `.14` within the existing `.12` through `.14`
reader design, retaining mounted lifetime and captured-session contracts.

## Evidence and scope

At independently refreshed main `49f8cd6377ec4025f94c4cad2955adf025f49de6`,
`fetchSessionReviews` unconditionally replaces the session map. ReviewDialog
uses actions without a loading gate. Parent deferred-wire diagnostic produced
two intended assertion failures (acknowledged mark lost and reset resurrected)
and one untouched-read passing control. Parent evidence is durable under
`/home/jcfs/.kandev/tasks/so-we-had-a-previous_ve85xcv7/loop-receipts/attempt17-source/`;
it is source evidence, not permanent RED or browser/live-backend proof.

Own only `hooks/use-session-file-reviews.ts`, its focused test suite and this
owning requirement/design package. Preserve foreign edits and shared runtime.
Exclude arbitrary overlapping-write semantics, persistence, API/schema/security
changes, Query migration, generic state frameworks and mounted-reader redesign.
No new ADR: the local mechanism and alternatives fit the current system design.

## Technical approach

Follow [initial snapshot reconciliation](../../specs/ui/system-design/task-navigation-responsiveness.md#initial-snapshot-reconciliation-with-local-intent).
Track touched paths and a reset barrier for the outstanding initial read only.
Reconcile against latest cache; preserve untouched server rows and existing
mark rollback/unmark-reset failure policies. Release transient bookkeeping when
the read settles. Keep shared cache lifetime, coalescing and local publishers.

## Tests

`apps/web/hooks/use-session-file-reviews.test.tsx` owns `.14` regression proof:
pending/acknowledged mark, unmark and reset; unrelated snapshot rows; mark failure
before/after GET; unchanged-read control; shared reader with initiator retirement;
real ReviewDialog checkbox staying checked after an older empty GET. Existing
`.12`/`.13` reader lifetime tests remain the compatibility matrix. Deferred
protocol replies name the cause; no sleep or mocked coordination is permitted.

## Mobile, E2E and public documentation

Phone Changes uses MobileChangesPanel and phone Review uses
SessionMobileReviewDialog/TaskReviewDialogMount, sharing the existing hook.
This is state/data reconciliation with no layout, touch, scrolling, navigation,
copy or breakpoint change. The mobile-parity state/data exception permits the
real consumer and hook tests; no new browser or mobile E2E is needed. No rendered
structure change requires an ASCII layout preview.

Public docs assessment: `docs/public/sessions-and-review.md` already promises
session-owned marks and hash staleness. The repair restores that stated behavior;
`websocket-api.md` get/update/reset contracts remain unchanged. README and
screenshot catalog need no changes. Only internal documents change.

## Work orders

- [x] [Task 01: Reconcile initial review snapshots](task-01-reconcile-initial-snapshot.md)

## Delivery and resource limits

No delegation, persistent task/session creation, issue search or runtime/seed
mutation. Fresh frozen pnpm install precedes package checks. One Vitest worker,
zero retries, no full-suite overlap; Node 24 typecheck bounded at 8 GiB.
No Go changes or browser/live-backend claim. Keep durable receipts outside the
checkout at `/home/jcfs/.kandev/tasks/preserve-review-mark_3ari4908/receipts/attempt17/`.
Controller-to-child messages must explicitly set `delivery_mode=interrupt`;
reconcile owned live handles after interruption without duplicate execution.

Stop at `ready_for_parent_validation` with normal active pre-commit/commit-msg
hooks, clean committed head and exact diff/check receipts. No push/PR before
parent actual-diff review, independent focused run and separate explicit
same-session delivery authorization. Parent owns integration, merge and counts.

## Verification results

All implementation checks passed; exact commands/results are recorded in the
[work order](task-01-reconcile-initial-snapshot.md#results). Permanent RED:
10 intended failures and 19 passing controls; GREEN: 29 passes, one worker,
zero retries. Normal-hook commit and clean-head receipts are retained externally;
publication remains gated on independent parent validation.

## Risks

Whole-read rejection loses unrelated reviews; transient touched-path overlay
must include rollback deletions. Do not retire shared read state when one reader
unmounts. Existing arbitrary overlapping-write behavior is deliberately retained.
