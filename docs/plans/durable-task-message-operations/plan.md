---
status: done
created: 2026-10-23
requirements:
  - REQ-PLATFORM-TASK-MSG-OPID-001
  - REQ-PLATFORM-TASK-MSG-OPID-002
  - REQ-PLATFORM-TASK-MSG-OPID-003
  - REQ-PLATFORM-TASK-MSG-OPID-004
system_design:
  - ../../specs/platform/system-design/task-message-operation-identity.md
---

# feat(mcp): add durable task message operation readback

Persist optional message operation identity, sender-scoped readback and durable once-only replay using existing delivery and queue settlement.

## Delivery order and dependencies

The implementation is one existing PR slice. Preserve its native dependencies and existing feature defaults, then verify the narrow affected paths. No dependent PR is imported into this package.

- [Implementation and verification](task-01-implementation.md)

## Scope and risks

New delivery paths, metadata authorization, unknown-outcome replay, and changing sends without operation identity. These remain outside this delivery slice. Preserve existing files, configuration and history; report unavailable integration coverage without inventing a successful result.

## Verification strategy

Reuse the completed focused implementation checks recorded below and the existing documentation-coverage evaluator. Only remaining review fixes require their affected checks to run again.
