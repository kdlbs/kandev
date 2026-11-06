---
status: done
created: 2026-10-23
requirements:
  - REQ-TASKS-WORKFLOW-VISIBILITY-PATCH-001
system_design:
  - ../../specs/tasks/system-design/workflow-visibility-patch.md
---

# feat(workflows): support optional visibility updates

Transport optional workflow hidden presence through native update routes and persist a visibility-only edit without replacing other fields.

## Delivery order and dependencies

The implementation is one existing PR slice. Preserve its native dependencies and existing feature defaults, then verify the narrow affected paths. No dependent PR is imported into this package.

- [Implementation and verification](task-01-implementation.md)

## Scope and risks

Task archive/deletion, new filtering policy, and workflow-definition replacement. These remain outside this delivery slice. Preserve existing files, configuration and history; report unavailable integration coverage without inventing a successful result.

## Verification strategy

Reuse the completed focused implementation checks recorded below and the existing documentation-coverage evaluator. Only remaining review fixes require their affected checks to run again.
