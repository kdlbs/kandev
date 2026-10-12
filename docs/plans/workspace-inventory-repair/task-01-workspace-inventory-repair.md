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

### Upstream conflict integration

Preserved managed-clone relocation error stamps and idle-suspension provenance
while merging upstream recovery changes. Explicit inventory repair skips the
mutating filesystem preflight until its executor proves preserved identity;
other recovery actions retain preflight admission. Added a service regression
for refusal before filesystem recovery and kept both handler action validations.
Updated the relocated-worktree test for the resume-options argument.

Validation: executor, orchestrator, handlers, worktree, SQLite repository, and
models package suites passed in local runs; the final focused inventory-repair
regression passed with a typed conflict and no filesystem preflight or launch.
Specification lint, documentation catalog, public documentation, and harness
validation passed. Current-head GitHub CI and review validation remain pending.

### Provider-restored settings integration

Merged the newer provider-restored settings policy through the shared recovery
options without weakening its explicit-resume-only validation. Inventory repair
retains strict settings; handler and service coverage reject attempts to combine
inventory repair with provider-restored settings. Existing provider-restored
eligibility checks and inventory preservation preflight ordering remain intact.

Validation: affected executor, orchestrator, and handler suites passed locally,
including the strict-policy regression. Documentation catalog and specification
lint and backend lint passed (zero issues). The provider-settings merge passed
normal commit hooks without bypass.

After integrating newer upstream changes, all six affected package suites and
the offline inventory-repair package and command suites passed. Backend lint
passed with zero issues on retry after its initial five-minute timeout. Harness,
documentation catalog, and specification validation passed. Remote delivery and
fresh GitHub CI remain pending. GitHub authentication has been restored; the
latest upstream integration passed all six affected package suites, backend
lint, documentation catalog, and specification validation before delivery.

### Architecture and security remediation

Preservation inspection disables Git fsmonitor and rejects external clean/process
filters before status can execute them. Receipt admission uses the environment
owner for inherited/shared-group checkouts, and explicit retries validate stored
key identity even when inventory is already valid. Durable environment recovery
claims cover proof, repair, and incomplete attestation; repair and attestation
transactions honor those claims. Local mutexes only serialize same-task retries.

Permanent regressions reproduced all four findings before fixes. Additional
SQLite tests cover live inherited sessions, orphan runtimes, concurrent child
admission, claim release after cancellation, and guarded metadata/attestation
writes. Existing crash fixtures now mark the failed writer terminal before
another session completes its attestation.

Validation: full executor and worktree race suites passed; targeted inventory,
callback, inherited-attestation, and cross-session regressions passed after the
final production edit. PostgreSQL inventory-repair regressions passed with
`go test -race ./internal/task/repository/sqlite -run 'TestPostgres.*WorkspaceInventory' -count=1`
against a disposable PostgreSQL 17 database. SQL guard, documentation catalog,
specification lint, and public-doc validators passed. Final broad checks and
exact-head GitHub CI/review verification are recorded in the Kandev task.
