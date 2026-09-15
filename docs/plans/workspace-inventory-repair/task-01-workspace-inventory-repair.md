---
id: "01-workspace-inventory-repair"
title: "Workspace inventory repair"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.1
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.2
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.3
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.4
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.5
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.6
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.7
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.8
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.9
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.10
system_design:
  - ../../specs/agents/system-design/workspace-inventory-repair.md
---

# Task 01: Workspace Inventory Repair

## Summary

Add one guarded, identity-preserving recovery path for a reusable task
environment whose canonical inventory drifted, keeping
`validateReuseEnvironmentInventory` as the fail-closed admission guard.

## In scope

- Candidate selection from server-owned task, workspace, environment,
  repository, session, worktree, and Git metadata only.
- Preservation receipt captured before repair (identity, branch containment,
  clean/dirty summary and hashes, runtime state, revisions, timestamp).
- Single-row repair transaction plus append-only receipt record keyed by
  task-scoped idempotency identity.
- Row-scoped post-repair attestation before launch admission.
- One admitted resume/start attempt after a committed repair, with
  concurrency control against duplicate writers.
- Typed, non-leaking refusals for missing, stale, duplicate, conflicting,
  cross-task, cross-workspace, or ambiguous evidence.

## Out of scope

- Deleting, cleaning, resetting, reseeding, or rematerializing the preserved
  checkout in any case.
- Automatic fallback to a fresh checkout after any refusal or failure.
- Provider, workspace-source, or session-history mutations.

## Acceptance

- Reuse without an exact one-row match still fails closed.
- Repair proceeds only with an exactly-one reciprocal proven identity and
  records before/after evidence.
- Same-key retries return the stored receipt; different payloads conflict
  without mutation.
- Unauthorized callers receive no host paths or cross-workspace existence
  information.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator/executor ./internal/orchestrator ./internal/orchestrator/handlers ./internal/worktree
cd apps/backend && go test ./internal/task/repository/sqlite
```

PostgreSQL concurrency acceptance ran separately against a disposable
PostgreSQL fixture for
`TestPostgresRepairWorkspaceInventoryConcurrentSameKeyRetryConvergesToOneRepairAndOneDeduplicated`.

## Results

Implemented and verified. Focused backend suites pass at the delivered
revision; the inventory guard was proven intact for both reproduced mismatch
scenarios (`staging-py3` and `dev`).
