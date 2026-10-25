---
created: 2026-09-13
status: done
requirements:
  - REQ-OFFICE-COSTS-002
system_design:
  - ../../specs/office/system-design/costs-03.md
---

# Implementation Plan: Budget Claim Revision Fencing

This plan records the revision fence and atomic companion claim changes for
budget notifications. The work keeps the existing Office budget notification
contract and its idempotent claim store.

## Work order

- [Task 01: Fence budget claims to policy revisions](task-01-fence-budget-claims.md)

## Outcome

Budget claims carry the policy revision that earned them. Policy updates bump
the revision and discard old claims, stale evaluations cannot claim a newer
revision, and exceeded evaluations claim their alert companion in one
transaction.
