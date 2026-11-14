---
id: "01-implementation"
title: "fix(web): hydrate accepted directory move destinations"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
acceptance_criteria:
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.4
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.15
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.16
system_design:
  - ../../specs/ui/system-design/file-browser-reply-freshness.md
---

# fix(web): hydrate accepted directory move destinations

## Outcome and scope

Hydrate accepted moved directory destinations and reconcile pending hints against newer authoritative tree updates.

Out of scope: File mutation semantics, failed move outcomes, expansion state, and workspace request fencing.

## Implementation acceptance

1. The affected native entrypoint follows the linked requirement and design while preserving unrelated behavior and history.
2. Rejected, stale and failed operations retain their existing refusal or rollback semantics.
3. The focused implementation checks and linked documentation coverage have honest recorded results; unexecuted follow-up checks remain pending.

## Files and dependencies

The source files are the existing PR #4709 diff. This work order adds its delivery record and links existing requirements and system design; it introduces no runtime gate or permanent test.

- [System design](../../specs/ui/system-design/file-browser-reply-freshness.md)
- [Plan](plan.md)

## Verification and results

Validation on Windows:
- Four existing focused Vitest suites passed (55 tests): apply-changes, refresh-freshness, move-settlement, and move-selection.
- Two temporary checks passed: successful collapsed-directory hydration with mixed file/failed moves, and rejection of an older destination reply. Temporary files were removed; no standing test suite was added.
- A review follow-up reproduced a deleted child reappearing after an older hydration reply. Both direct watch refresh and invalidate-then-refresh paths now pass, including release after commit; the same 55 existing tests passed again. These two disposable cases were removed.
- Changed-file ESLint, git diff --check, and normal commit hooks passed, including documentation/specification and architecture validation.

Documentation-only correction: git diff --check and the existing .github/scripts/pr-docs.cjs validateCoverage entrypoint with the candidate changed paths and the linked candidate documents supplied on stdin. No GitHub status is synthesized locally.

## ASCII UI preview

UI-01: Existing affected surface, after this change.

```text
Files > destination/
  moved-directory/
    child.txt

A confirmed directory move refreshes its accepted destination. A newer authoritative removal keeps it absent.
```

This sketch records the affected structure; spacing is illustrative. It does not add a new control or approval requirement.
