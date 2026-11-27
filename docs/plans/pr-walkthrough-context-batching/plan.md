---
created: 2026-10-02
status: implemented
requirements:
  - REQ-UI-PR-WALKTHROUGH-001
system_design:
  - ../../specs/ui/system-design/pr-walkthrough.md
legacy_specs: []
---

# Implementation plan: bounded PR context batching

## Scope

Repair context preparation for large pull requests under the existing trusted
Git-object contract. Preserve exact-head input, file and total byte limits,
manifest ordering, exclusion reasons, and the absence of contributor checkout.
This is a performance correction to the implemented portable runner package.
It adds no product behavior, permissions, configuration, or publication path.

The production history helper fetches with `--filter=blob:none`. Both Git
process creation and missing-object network requests must remain bounded.
A persistent reader alone does not prevent a separate promisor fetch per blob.

## Work orders

| Order | Work order | Status |
| --- | --- | --- |
| 01 | [Batch immutable context reads and promised blobs](task-01-batch-context.md) | done |

## Validation and release

Run the work order's exact checks. Merge through the normal PR checks after
review. The walkthrough workflow loads executable helpers from trusted main;
a feature branch cannot activate this correction for its own walkthrough.
PR #4147 remains unmerged pending normal review and explicit merge authorization.
