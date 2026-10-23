---
id: "01-implementation"
title: "fix(worktree): reuse retained checkouts at assigned task paths"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-RETAINED-CHECKOUT-001
acceptance_criteria:
  - AC-AGENTS-RETAINED-CHECKOUT-001.1
  - AC-AGENTS-RETAINED-CHECKOUT-001.2
  - AC-AGENTS-RETAINED-CHECKOUT-001.3
  - AC-AGENTS-RETAINED-CHECKOUT-001.4
  - AC-AGENTS-RETAINED-CHECKOUT-001.5
  - AC-AGENTS-RETAINED-CHECKOUT-001.6
  - AC-AGENTS-RETAINED-CHECKOUT-001.7
  - AC-AGENTS-RETAINED-CHECKOUT-001.8
  - AC-AGENTS-RETAINED-CHECKOUT-001.9
system_design:
  - ../../specs/agents/system-design/retained-checkout-preparation-reconciliation.md
---

# fix(worktree): reuse retained checkouts at assigned task paths

## Outcome and scope

Reuse an exact retained assigned checkout before refresh/creation, preserving valid registration or attaching an eligible unregistered checkout through normal materialization.

Out of scope: Cleaning/resetting/moving checkouts, adopting conflicting slots, broad filesystem discovery, and new ownership proofs.

## Implementation acceptance

1. The affected native entrypoint follows the linked requirement and design while preserving unrelated behavior and history.
2. Rejected, stale and failed operations retain their existing refusal or rollback semantics.
3. The focused implementation checks and linked documentation coverage have honest recorded results; unexecuted follow-up checks remain pending.

## Files and dependencies

The source files are the existing PR #4722 diff. This work order adds its delivery record and links existing requirements and system design; it introduces no runtime gate or permanent test.

- [System design](../../specs/agents/system-design/retained-checkout-preparation-reconciliation.md)
- [Plan](plan.md)

## Verification and results

Validation: focused existing worktree/executor checks and disposable real-Git Create/reuse journeys passed, including registered and unregistered reuse, unchanged IDs, registration failure, foreign/main checkout preservation and conflicting-slot refusal. All normal commit hooks passed and temporary probes were removed. No standing tests, new hashes, CI or operating mutations were added.

Documentation-only correction: git diff --check and the existing .github/scripts/pr-docs.cjs validateCoverage entrypoint with the candidate changed paths and the linked candidate documents supplied on stdin. No GitHub status is synthesized locally.
