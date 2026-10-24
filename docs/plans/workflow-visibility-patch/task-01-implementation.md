---
id: "01-implementation"
title: "feat(workflows): support optional visibility updates"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-VISIBILITY-PATCH-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-VISIBILITY-PATCH-001.1
  - AC-TASKS-WORKFLOW-VISIBILITY-PATCH-001.2
  - AC-TASKS-WORKFLOW-VISIBILITY-PATCH-001.3
  - AC-TASKS-WORKFLOW-VISIBILITY-PATCH-001.4
system_design:
  - ../../specs/tasks/system-design/workflow-visibility-patch.md
---

# feat(workflows): support optional visibility updates

## Outcome and scope

Transport optional workflow hidden presence through native update routes and persist a visibility-only edit without replacing other fields.

Out of scope: Task archive/deletion, new filtering policy, and workflow-definition replacement.

## Implementation acceptance

1. The affected native entrypoint follows the linked requirement and design while preserving unrelated behavior and history.
2. Rejected, stale and failed operations retain their existing refusal or rollback semantics.
3. The focused implementation checks and linked documentation coverage have honest recorded results; unexecuted follow-up checks remain pending.

## Files and dependencies

The source files are the existing PR #4711 diff. This work order adds its delivery record and links existing requirements and system design; it introduces no runtime gate or permanent test.

- [System design](../../specs/tasks/system-design/workflow-visibility-patch.md)
- [Plan](plan.md)

## Verification and results

Validation:
- Registered REST/WS integration checks passed for hide/restore, omission/null, mixed fields, stored responses/events and built-in rejection without partial writes.
- Existing workflow field-update and system visibility tests passed.
- Normal repository commit hooks passed, including Go lint. The first attempt encountered the shared lint lock, and a subsequent cold analysis exceeded its time limit; the cached normal retry passed without bypassing checks.
- Temporary Go-overlay checks were removed; no permanent tests or CI jobs were added.

Documentation-only correction: git diff --check and the existing .github/scripts/pr-docs.cjs validateCoverage entrypoint with the candidate changed paths and the linked candidate documents supplied on stdin. No GitHub status is synthesized locally.
