---
created: 2026-10-21
status: done
requirements:
  - REQ-WORKSPACES-REPOSITORY-SETS-001
system_design:
  - ../../specs/workspaces/system-design/repository-sets.md
legacy_specs: []
---

# Implementation Plan: Repository-set Deletion During Initial Hydration

## Overview

Preserve live deletion during the initial list and recover every surviving set.
One sequential work order covers the store, actual HTTP/WS/hook boundary, and
owning documents. The user explicitly waived a separate planning checkpoint
for this bounded repair; artifacts precede permanent tests and production edits.

## Confirmed defect

At base `c4de9a179569527009454cddf2a4f433b7aea06e`, `removeRepositorySet`
returns before bumping the local revision when the workspace bucket is absent.
A pending initial GET captured at revision zero can therefore resurrect the
deleted set. Merely bumping the revision drops that response but the current
hook never recovers the complete unloaded view. Parent's real provider/API/WS
diagnostic on earlier main failed only the missing-bucket case, with cached
empty-bucket and other-workspace controls passing. Permanent RED must reprove
the behavior on this base with mixed deleted/surviving rows.

## Scope

Own the missing-bucket deletion revision and store-scoped initial-list recovery.
Preserve direct action/registered WS parity, boot-loaded caches, refresh failure
behavior, consumer lifecycle, and independent workspace/provider ownership.
No backend, protocol, persisted tombstones, Query migration, layout, copy,
plugin preferences, or live data changes. No delegation or delivery publication.

## Technical approach

Advance revision regardless of bucket presence, without fabricating a loaded
list. Share a transient in-flight promise per store/workspace in the hook;
read revision from the store before each attempt. Retry a successfully completed
but overtaken snapshot only while unloaded. Stop on errors; settle shared
loading even when the initiating consumer leaves. Reuse local request ownership
patterns rather than introducing a general cache resource. PR review identified
that a hung request could retain shared ownership indefinitely. Bound the whole
initial/recovery operation to 30 seconds with one abort signal and clear its
timer on settlement; exercise real transport abort and retry before delivery.

Open PR inventory and relevant patches refreshed before implementation:
1512 `a8655ad9` only removes older legacy workspace state; 3656 `e43f6612`
changes task draft application and AC 003.10; 4660 `addaa1df` changes secrets.
They do not repair this race. Keep these merge boundaries narrow. Existing
base-branch and concurrent-patch plans describe separate delivery and retain
their results. No new ADR is needed for extending existing local revisions.

## Tests and end-to-end evidence

AC-WORKSPACES-REPOSITORY-SETS-001.6/.7 maps to
`use-repository-sets-hydration.test.tsx`: real StateProvider, hook, list API,
fetchJson, registered deletion handler, and only a deferred fetch transport.
Assert immediate no-resurrection plus a complete current list containing
survivors; cover direct actions, missing/existing bucket, recovery failure and
retry, repeated events, unmount/disable/key switch, shared consumers and isolated
providers/workspaces. Existing slice, handler, API, and hook tests remain controls.

Mobile-parity's state-only exception applies: New Task's setup hook, New Subtask's
workspace section, and repository settings all consume the same list with no
viewport branch. Targeted integration tests and existing consumer tests prove
data behavior. No rendered UI change, preview, new phone E2E, browser or live
backend claim. Public docs already promise live reusable sets; no changed
terminology, command, configuration, API, or user workflow requires an update.

## Work orders

- [x] [Task 01: Fence deletion and recover the initial list](task-01-recover-initial-list.md)

## Verification results

Task 01 completed. Eight targeted suites passed after PR liveness remediation (82 tests),
including all 16 integration tests. Two deadline regressions first failed on
the pre-remediation source, then passed with a shared operation deadline. Scoped ESLint passed with zero warnings, Node 24 typecheck passed
with a 6 GiB V8 heap under the unchanged 8 GiB guard, and documentation
catalog, full spec lint, actual-path coverage, and diff checks passed.
See Task 01 for command receipts and limits. The initial delivery checkpoint stopped at a normal-hook commit for independent
parent validation. Later delivery authorization and exact-head CI/review evidence
are tracked in the external task plan.

## Risks

- Dropping a stale response alone loses surviving rows in an unloaded view.
- Per-hook cancellation can strand another consumer's loading state.
- Repeating failures must not start an automatic retry loop.
- Request ownership must remain local to the store and workspace.
- Stop after a clean ordinary-hook commit for independent parent validation.
