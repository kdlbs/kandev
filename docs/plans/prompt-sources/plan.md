---
status: done
created: 2026-10-23
requirements:
  - REQ-PROMPT-SOURCES-REVISION-001
system_design:
  - ../../specs/prompt-sources/system-design/revisioned-workflows.md
---

# Revisioned workflow prompt sources

Implement the existing revisioned workflow source contract on native workflow
storage and sync helpers, preserving task, session and history identity.

1. Keep immutable template revisions scoped to their workspace and preserve
   stable instance/workflow IDs across reimport and rollback.
2. Plan changed definitions using the existing portable-workflow conversion,
   comparison and step serializers, then apply them in one transaction.
3. Expose import, non-writing preview and visible drift through native routes.
   Require workflow definition-write scope for import/preview and the existing
   executor mutation authority when adopting or changing an existing workflow.
4. Attribute newly created workflows through the same trusted creator rule as
   ordinary native creation; preserve attribution on updates.
5. Enforce source locking at the existing definition mutation boundaries and
   disable the workflow card's existing edit controls for a locked source.
6. After the complete import transaction commits, publish only the changed
   workflow and step definitions through their existing lifecycle publishers.
   Preview, no-op and failed imports publish nothing.

## Verification boundary

Use the existing focused workflow, task-repository, authorization and schema
checks. Disposable checks exercise HTTP import/preview/diff, stable reimport,
rollback, occupied-step refusal, atomic failure, source locking and authority.
Do not add permanent test files or activate/import sources in an operating
workspace. PostgreSQL runtime coverage requires an available test DSN.

## Source provenance

The source topic is 5576b601a and its definition-permission correction
c50cb23d2. The current native integration additionally checks executor
mutation authority for an adopted workflow and preserves the native creator
when a source creates a workflow. This port includes those corrections without
bringing in execution-grade or explicit-profile-policy features.

## Work order

- [Implementation and verification](task-01-implementation.md)

## ASCII UI preview

UI-01: Existing affected surface, after this change.

```text
Workflow card
  Source locked: on
  [Edit definition: disabled]

An explicitly unlocked instance keeps the existing editor. Desktop and phone retain their current card composition.
```

This sketch records the affected structure; spacing is illustrative. It does not add a new control or approval requirement.
