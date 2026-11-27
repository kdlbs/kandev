---
created: 2026-10-11
status: complete
requirements:
  - REQ-TASKS-PORTABLE-ADOPTION-001
  - REQ-TASKS-PORTABLE-ADOPTION-002
  - REQ-TASKS-PORTABLE-ADOPTION-003
system_design:
  - ../../specs/tasks/system-design/portable-workspace-adoption.md
---

# Portable workspace adoption implementation

## Overview

Approved Kandev design 531dfb46 and reproduced gate b8b95a39 establish the distinct
core adoption gap. Preserve unrelated guarded-ACP commit cfd6bdf65a under
`preserve/portable-adoption-guarded-acp-cfd6bdf65a`. Task branch base is upstream
00b5d599968a713a7c24318a17128a6b4b1dc431. Isolated probes remain local.

## Ordered tasks

1. [Inspection](task-01-inspection.md): pinned same-root filesystem and origin proof.
2. [Publication](task-02-publication.md): claim, selection CAS, atomic receipt, replay.
3. [Operator API](task-03-api.md): service authorization, routes and resume identity.
4. [Verification](task-04-verification.md): regression, documentation, commit and side sessions.

MAIN alone writes source. Independent Kandev Review and QA inspect the frozen
committed candidate. No PR or deployment work is included in this continuation.

## Tests

Real Git, SQLite and env-gated PostgreSQL fixtures cover HTTP preview/apply,
identity/content preservation, old marker provenance, stale sessions/inventory,
foreign paths/origins/branches, active consumers, rollback and operation replay.
Use named TestWorkspaceAdoption tests in affected worktree, task handlers and
repository packages; run with race for final behavior checks. Test logs and probe
failures are local evidence under .kandev/portable-adoption/artifacts.

## Verification Results

Step 1 isolated gate passed against current main 1bba2c18 in the previous turn.
Implementation tasks 1-3 are complete. MAIN targeted race checks passed across
worktree, task handlers, repository and backend composition. SQLite and task-owned
PostgreSQL 18.6 publication, rollback, schema replay and exclusive claim checks
passed. Independent Review session 34c401ff-c5c1-4345-90bc-156eac6b5e7c and QA
session 16e162e7-dce3-4341-8968-fc7368b03559 passed source candidate
7cb27830c637ba2969c99271b9e982f3a29093ea after MAIN fixed replacement-ancestry
and escaped-ref findings. QA reran 37 focused tests and four independent probes
(21 scenarios), with no failures, skips or race warnings, including PostgreSQL.
Review reran the original two failure probes and committed metadata regressions.

QA command: `go test -C apps/backend -tags fts5 -race ./internal/task/handlers
./internal/task/repository/sqlite ./internal/worktree ./internal/backendapp
-run 'TestWorkspaceAdoption|TestMainCheckout|TestManager_AdmitTaskRecovery|TestRecoveryOperationLockIsExclusiveAndReleasesForAdoption'
-count=1 -v`, using the private task-owned PostgreSQL fixture. Reports and logs:
`.kandev/portable-adoption/review/round2-report.txt` and
`.kandev/portable-adoption/qa/round2-report.md`. Final documentation-only candidate
SHA and both sessions' delta verdicts are recorded in Kandev plan metadata.

Acceptance is synthetic HTTP/Git/SQLite/PostgreSQL and manager-level continuity.
Real agent resume, process kill/restart, full auth-enabled service, unsupported
platforms, release binary and deployment acceptance remain unverified.
No submodules or live deployment involved. No PR or overlay retirement claim.

## PR repair round

Inherited Git repository selectors could redirect adoption proof reads after
checkout health inspection. The correction uses the same sanitized environment
and explicit metadata selection as healthy-checkout inspection, retaining raw
object ancestry. HTTP regression coverage includes preview/apply/reuse and every
operator route. The adoption persistence table is registered for readiness.
Delivery documentation now has valid work-order and system-design relationships;
the trusted PR evaluator reports covered. Targeted independent revalidation for
this repair is recorded in Kandev task metadata before any new acceptance claim.

Receipt re-attestation is an unresolved contract decision sent to the Coordinator.
The current fail-closed validation and immutable one-per-slot receipt are
retained; no automatic refresh or stale authority relaxation is introduced.
