---
spec: docs/specs/tasks/requirements/additional-session-workspace-reuse.md
design: docs/specs/tasks/system-design/environment-owned-git-status.md
created: 2026-08-30
status: complete
---

# Fix Plan: Keep Shared-Environment Git Status Authoritative

## Overview

The Changes panel caches Git status by task environment, but backend hydration
currently reads status by requested session. Two sibling subscriptions can
therefore write different workspace results into one frontend cache entry.

The fix makes the backend select status only from a session bound to the task
environment's canonical workspace. It also makes the frontend reject older
environment observations. A focused end-to-end test proves that dirty files
remain visible through reload and sibling hydration.

## Confirmed root cause

For task `c086d141-1966-4502-923a-c5ede67876d3`, two sessions shared task
environment `9d364cad-dde8-4e02-b29b-a07c83f66165` but recorded different
workspace paths. The canonical session had 91 dirty files. The older sibling
pointed at a clean checkout.

Both live Git requests timed out. `appendDBSnapshotGitStatus` then loaded a
snapshot by each requested session ID. The frontend mapped both events to the
same environment key. It stored the 91-file snapshot first and replaced it
147 milliseconds later with the sibling's clean snapshot.

PR #3167 fixes the recovery path that can preserve an incorrect session
workspace. It does not make current Git-status projection environment-owned or
order frontend status observations.

## Approach

1. Resolve eligible Git-status source sessions from the canonical task
   environment before live or snapshot hydration.
2. Route the canonical result through the requested subscription session ID.
3. Keep a monotonic timestamp watermark per environment and repository in the
   frontend store.
4. Extend the two-session E2E coverage to prove that Changes remains populated
   after reload and sibling hydration.

Do not add a database table, change the WebSocket action, prefer dirty state,
or hide the problem by removing sibling subscriptions.

## Task waves

### Wave 1: backend authority

- [x] [Task 01: Select canonical Git-status sources](task-01-canonical-status-source.md)

### Wave 2: ordering and user regression

- [x] [Task 02: Reject older environment status](task-02-status-ordering-and-e2e.md)

Task 02 depends on Task 01. The end-to-end result requires the backend authority
rule and the frontend ordering safeguard together.

## Verification strategy

- Backend unit tests reproduce same-environment sessions with different
  workspace paths and prove live and snapshot source selection.
- Frontend unit tests reproduce a newer dirty event followed by an older clean
  sibling event.
- A Chromium E2E test creates a dirty file in a shared environment and proves
  that it remains visible after reload and sibling-session hydration.
- Specification lint and `git diff --check` validate the delivery records.

## Risks and mitigations

- **Remote workspace paths:** compare the same effective workspace field that
  the task environment and session persist. Test host execution now and avoid
  host-only path resolution such as symlink evaluation.
- **Missing historical paths:** fail closed for environment-owned hydration.
  A later valid live update can recover the view.
- **Equal timestamps:** reject only strictly older valid timestamps. Preserve
  meaningful equal-time updates for compatibility.
- **Duplicate updates:** advance the timestamp watermark without invalidating
  the cumulative diff cache when Git content is unchanged.
- **PR #3167 overlap:** rebase before delivery and keep its recovery-path tests.
  This package owns projection authority and ordering, not workspace recovery.

## Mobile parity

Desktop and mobile Changes surfaces share the same environment-keyed store.
This fix changes no layout, navigation, control, gesture, or copy. The store
unit test and shared-state E2E evidence cover the mobile data contract. A new
mobile E2E scenario duplicates the same state assertion. It does not test a
mobile-specific interaction.

## Implementation results

- Task 01 selects live and persisted Git-status sources from the canonical task
  environment and keeps the requested session ID on the outgoing event.
- Task 02 applies per-environment and per-repository timestamp ordering, keeps
  duplicate-content watermarks without invalidating cumulative diffs, and adds
  the shared-environment Chromium regression.
- PR fixup coverage now verifies raw workspace provenance, inherited
  cross-task environment bindings, recovered executions, one deadline across
  live source probes, and one persisted fallback status per repository.
- The shared desktop/mobile store contract is covered without a duplicate
  mobile E2E case because presentation and interaction are unchanged.
- The exact RED/GREEN evidence and verification results are recorded in the
  two work orders.
