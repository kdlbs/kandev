---
status: done
created: 2026-10-23
requirements:
  - REQ-OFFICE-AUTOMATION-TARGETS-004
system_design:
  - ../../specs/office/system-design/automation-target-modes.md
---

# feat(automation): support optional parent tasks

Support an optional validated parent for both automation target modes, preserve retry snapshots, clear future continuation when the parent changes, and expose explicit relationships.

## Delivery order and dependencies

The implementation is one existing PR slice. Preserve its native dependencies and existing feature defaults, then verify the narrow affected paths. No dependent PR is imported into this package.

- [Implementation and verification](task-01-implementation.md)

## Scope and risks

Changing ordinary board filtering, rewriting old tasks/history, and exporting instance-local parent IDs. These remain outside this delivery slice. Preserve existing files, configuration and history; report unavailable integration coverage without inventing a successful result.

## Verification strategy

Reuse the completed focused implementation checks recorded below and the existing documentation-coverage evaluator. Only remaining review fixes require their affected checks to run again.

The review follow-up covers stale-parent disable, current continuation binding
under older firings, parent-compatible reuse, and typed server search. It reuses
the current revision and claim/store paths and adds no runtime scheduler, schema,
automation activation or permanent test suite.

## ASCII UI preview

UI-01: Existing affected surface, after this change.

```text
Automation editor
  Parent task (optional): [Search tasks]
  Target: hidden run | normal task
  [Save]

Clearing the picker removes future parent assignment. Desktop and phone keep the existing editor and save flow.
```

This sketch records the affected structure; spacing is illustrative. It does not add a new control or approval requirement.
