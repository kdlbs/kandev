---
id: "04-cleanup-conflict-diagnostics"
title: "Make cleanup identity conflicts actionable"
status: completed
wave: 2
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-002
acceptance_criteria:
  - AC-TASKS-RUNTIME-CLEANUP-001.9
  - AC-TASKS-RUNTIME-CLEANUP-001.32
  - AC-TASKS-RUNTIME-CLEANUP-001.45
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.1
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.2
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.3
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.4
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.5
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.6
system_design:
  - ../../specs/tasks/system-design/runtime-cleanup.md
  - ../../specs/tasks/system-design/worktree-inventory-repair.md
---

# Task 04: Make cleanup identity conflicts actionable

## Summary

Identify each conflicting resource and the recorded/current comparison that needs operator inspection.
Preserve the original cleanup evidence and required capped cascade retries.

## In scope

- Extend typed inspection errors with bounded worktree identity, comparison, and safe action metadata at the real audit boundary.
- Preserve wrapping and inspect every classified resource in joined errors, with deduplication and a fixed diagnostic bound.
- Add `cleanup_condition=reconciliation_required` for proven registration/commit conflicts, separate from the existing `retry_wait` job state.
- Retain safe actionable context in `LastError` through bounded error formatting without snapshot replacement or new SQL columns.
- Respect the claimed-transition result before emitting new attributed diagnostics. A stale attempt cannot claim that its result committed.
- Include current retry policy and next-attempt context. Keep capped cascade retry and existing transient-failure behavior.
- Document reason-specific inspection, historical evidence preservation, and the existing explicit maintenance route and platform limits.

## Out of scope

- Repairing the operator's live jobs, changing their expected commits, auto-adopting registrations, or force-deleting a checkout.
- A new terminal/blocked cleanup state, retry button, public API, persistence schema, or automatic repair command.
- Treating diagnostic fields or a repeated refusal as destructive authority.

## Acceptance

- A job containing two conflicts and one safe sibling reports both affected worktree IDs and their distinct comparisons/actions. The safe resource still progresses.
- After attempts beyond the normal limit and after restart, cascade-critical jobs retain original snapshots, manifests, retry eligibility, and identity checks.
- Explicit fixture reconciliation permits only verified safe progress or linked supersession. Unknown/transient failures and stale generations remain protected.

## Regression evidence

Add `TestCleanupConflictsExposeEveryAffectedResource` using real competing registration and changed-ref Git fixtures.
It must fail on the recorded base because the action/condition fields are absent and only the first joined inspection error is attributed.
Add `TestCleanupConflictRetainsCascadeRetryAcrossRestart` with persisted jobs after attempts 13 and 14.
Add `TestApplyCleanupPreservesConflictDiagnosticEvidence` in inventory repair with exact before/after snapshot and manifest comparisons.
Cover missing expected evidence, transient Git start/context errors, independent resources, stale attempt completion, and diagnostic overflow.
Do not restore a moved branch by discarding new work to make the test pass.

## Verification

Run the actionable-conflict regression first and record its expected failure.
After implementation, run this block from the repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/worktree ./internal/task/service -run 'CleanupConflict|CleanupInspection|CleanupWorktrees|RetryTaskResourceCleanupJob|RetryCascadeCleanupJob|Unverified' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/inventoryrepair -count=1)
bash scripts/lint-go-changed apps/backend/internal/worktree/manager_cleanup_audit.go apps/backend/internal/worktree/manager_cleanup.go apps/backend/internal/task/service/resource_cleanup_jobs.go apps/backend/internal/task/service/resource_cleanup_conflicts.go
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `apps/backend/internal/worktree/manager_cleanup_audit.go`
- `apps/backend/internal/worktree/manager_cleanup.go`
- `apps/backend/internal/worktree/manager_cleanup_conflicts_test.go` (new)
- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- `apps/backend/internal/task/service/resource_cleanup_conflicts.go` (new)
- `apps/backend/internal/task/service/resource_cleanup_conflicts_test.go` (new)
- Existing inspection and retry tests, only for changed diagnostic assertions
- `docs/public/git-operations.md`
- `docs/public/tasks-and-workflows.md`

## Dependencies

None. Execute after Task 03 in manifest order.
Existing audit-preserving inventory repair supplies the explicit reconciliation boundary.

## Risks

The real macOS jobs can require further operator investigation. The package cannot prove their safe repair from redacted issue evidence.
Generic Git errors must not become identity conflicts, and new error formatting must retain sentinel identity through `Unwrap`.
Cascade-critical cleanup must never become terminal solely because attempts accumulate.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md), [runtime cleanup requirement](../../specs/tasks/requirements/runtime-cleanup.md), and [inventory repair requirement](../../specs/tasks/requirements/worktree-inventory-repair.md).
- The referenced cleanup conflict diagnostic and explicit repair designs.
- `resource_cleanup_inspection_test.go`, `resource_cleanup_worktree_retry_test.go`, and `manager_cleanup_recovery_test.go`.
- Existing `internal/task/inventoryrepair` preservation fixtures.
- `.agents/skills/docs-maintainer/SKILL.md` and `docs/public/README.md` before public-doc updates.

## Results

- Extended `CleanupInspectionError` with bounded worktree identity, comparison, action, and evidence-presence metadata; the batch boundary supplies the exact input worktree ID.
- The retry transition now collects every classified conflict from joined errors, deduplicates them, caps traversal and diagnostics, and emits `cleanup_condition=reconciliation_required` only after a claimed transition commits.
- Retry diagnostics distinguish bounded attempt limits from unlimited cascade retry and separately report the capped delay schedule. Stale attempts publish no attributed diagnostic.
- Added worktree fixture coverage for two conflicts plus a safe sibling, bound/overflow coverage, and a real SQLite persistence test that reopens the service and resumes attempts 13 and 14 without changing the snapshot.
- Added a persisted inventory-repair test proving explicit reconciliation preserves the conflicted predecessor snapshot and diagnostic, links successor evidence, and keeps an independent absent worktree protected. Existing inventory-repair tests continue to cover the rollback and historical-evidence boundary.
- Updated `docs/public/git-operations.md` and `docs/public/tasks-and-workflows.md`; public-doc validation passed.
- Verification passed: focused race tests, inventory repair tests, lint, and `git diff --check`.
