---
status: done
created: 2026-10-23
requirements:
  - REQ-AGENTS-RETAINED-CHECKOUT-001
system_design:
  - ../../specs/agents/system-design/retained-checkout-preparation-reconciliation.md
---

# fix(worktree): reuse retained checkouts at assigned task paths

Reuse an exact retained assigned checkout before refresh/creation, preserving valid registration or attaching an eligible unregistered checkout through normal materialization.

## Delivery order and dependencies

The implementation is one existing PR slice. Preserve its native dependencies and existing feature defaults, then verify the narrow affected paths. No dependent PR is imported into this package.

- [Implementation and verification](task-01-implementation.md)

## Scope and risks

Cleaning/resetting/moving checkouts, adopting conflicting slots, broad filesystem discovery, and new ownership proofs. These remain outside this delivery slice. Preserve existing files, configuration and history; report unavailable integration coverage without inventing a successful result.

## Verification strategy

Reuse the completed focused implementation checks recorded below and the existing documentation-coverage evaluator. Only remaining review fixes require their affected checks to run again.
