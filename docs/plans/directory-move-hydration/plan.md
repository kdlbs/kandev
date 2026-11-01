---
status: done
created: 2026-10-23
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
system_design:
  - ../../specs/ui/system-design/file-browser-reply-freshness.md
---

# fix(web): hydrate accepted directory move destinations

Hydrate accepted moved directory destinations and reconcile pending hints against newer authoritative tree updates.

## Delivery order and dependencies

The implementation is one existing PR slice. Preserve its native dependencies and existing feature defaults, then verify the narrow affected paths. No dependent PR is imported into this package.

- [Implementation and verification](task-01-implementation.md)

## Scope and risks

File mutation semantics, failed move outcomes, expansion state, and workspace request fencing. These remain outside this delivery slice. Preserve existing files, configuration and history; report unavailable integration coverage without inventing a successful result.

## Verification strategy

Reuse the completed focused implementation checks recorded below and the existing documentation-coverage evaluator. Only remaining review fixes require their affected checks to run again.

## ASCII UI preview

UI-01: Existing affected surface, after this change.

```text
Files > destination/
  moved-directory/
    child.txt

A confirmed directory move refreshes its accepted destination. A newer authoritative removal keeps it absent.
```

This sketch records the affected structure; spacing is illustrative. It does not add a new control or approval requirement.
