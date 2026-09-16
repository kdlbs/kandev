---
created: 2026-09-01
status: done
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008
system_design:
  - ../../specs/agents/system-design/workspace-inventory-repair.md
legacy_specs: []
---

# Implementation plan: Workspace inventory repair

## Overview

Recover a reusable task environment whose durable `task_environment_repos`
inventory does not exactly match the repository/branch slots required by
session resume or fresh-session launch, while keeping the existing fail-closed
reuse validation as the admission guard. Repair fixes only provably stale or
missing server-owned inventory metadata; it never deletes, reseeds,
rematerializes, cleans, resets, or rewrites the preserved checkout.

## Scope

In scope: guarded candidate selection, preservation receipts captured before
repair, one exact-row repair transaction with an idempotency ledger, a
row-scoped post-repair attestation, one admitted resume/start attempt after a
committed repair, typed non-leaking refusals for every ambiguous case, and the
task-scoped WebSocket recovery surface.

Out of scope: automatic removal or reseed fallbacks, provider or workspace
source changes, session-history edits, and any caller-supplied path, branch, or
repository acting as authority.

## Technical approach

The executor owns admission and candidate identification.
`internal/task/repository/sqlite` owns the repair transaction, the receipt
ledger, and post-repair attestation. The orchestrator exposes the
task-scoped, idempotent `repair_workspace_inventory` recovery action and maps
refusals to typed public errors without host paths.

## Results

Implemented and verified: focused executor, orchestrator, handlers, worktree,
and sqlite suites pass, including the PostgreSQL same-key concurrency and
convergence test run against a disposable PostgreSQL fixture.

## Risks

- Reciprocal-identity proof must never trust caller-supplied paths; all
  authority comes from server-owned records.
- Repairs are single-row and append-only audited; no inventory row outside the
  proven identity is modified.

## References

- [Requirements](../../specs/agents/requirements/workspace-inventory-repair.md)
- [System design](../../specs/agents/system-design/workspace-inventory-repair.md)
- [Decision](../../decisions/2026-09-01-preserve-checkouts-during-inventory-repair.md)
- [x] [Task 01: Workspace inventory repair](task-01-workspace-inventory-repair.md)
