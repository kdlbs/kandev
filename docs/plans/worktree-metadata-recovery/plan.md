---
created: 2026-09-10
status: in_progress
requirements:
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-001
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-002
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-003
system_design:
  - ../../specs/tasks/system-design/worktree-metadata-recovery.md
legacy_specs: []
---

# Implementation plan: Worktree metadata recovery

## Overview

This package adds the compatibility safeguards requested for contributor PR #3137.
It retains the snapshot and sibling-worktree approach. Work proceeds sequentially:
selected-environment admission, exclusive mutation authority, then integrated
recovery evidence and public documentation.

The package started as a design handoff against PR head
`22c57ef94fa24209855097cbc70ace5a5d2d09a2`. Production and regression changes
are now present locally. Work-order status and verification gaps remain tracked
below.

## Scope

### In scope

- Repo-free tasks, executor transitions, remote executors, and explicit environment selection.
- Multi-repository and multi-branch inventory preservation.
- Shared-session exclusion, ownership generations, restart claims, and safe publication.
- Requirements, system design, proposed ADR, public guidance, and targeted regression evidence.

### Out of scope

- Remote filesystem recovery, lost Git objects, and automatic snapshot deletion.
- New UI controls or provider conversation behavior.
- Merging the PR or changing its purpose.

## Technical approach

`executor_execute.go` and `executor_resume.go` replace task-wide admission with
selected-environment admission. `event_handlers_automation.go` connects the new
request contract to the manager. Empty inventories and non-Worktree executors
return before any host filesystem inspection.

The task repository provides an environment-scoped durable claim and migration.
All consumers that can start or restore a runtime must respect that claim before
external startup. Session attachment, cleanup, and ownership changes use the same
serialization boundary. The claim is not a destructive cleanup job.

The worktree manager preflights every selected slot, then performs existing
snapshot recovery under that claim. `CompareAndSwapWorktree` gains operation and
generation authority. The executor reloads canonical inventory before launch.

The implementation now scopes admission to the selected environment, persists an
environment claim, guards runtime and repository mutations, validates the recorded
branch, and publishes through a claim-aware compare-and-swap. It does not recover
remote executor filesystems or replace a lost branch automatically.

## Evidence

| Criteria | Planned evidence |
| --- | --- |
| 001.1-001.5 | `recovery_admission_scope_test.go` and selected executor launch/resume integration tests |
| 002.1-002.5 | `recovery_rematerialization_test.go` and `recovery_review_test.go`, including recorded-branch refusal and retained-file cases |
| 002.6-002.7 | Complete selected-inventory preflight in `recovery_admission.go`; multi-slot recovery uses stable ordering and retains earlier completed repairs |
| 003.1-003.4 | `task_environment_recovery_test.go`, including replay, competing claims, environment mutation, runtime startup, cleanup, ownership transfer, and stale release |
| 003.3 | `store_test.go`: claim-aware compare-and-swap success and stale-claim rejection |
| 003.5 | `executor_worktree_recovery_integration_test.go` and existing launch/resume refusal tests |

Each numeric criterion belongs to
`AC-TASKS-WORKTREE-METADATA-RECOVERY-<requirement>.<criterion>`.
Task 03 owns cross-layer evidence from selected launch through durable inventory
and lifecycle request. A helper-only test does not prove production wiring.

## End-to-end evidence

The worktree recovery suites use real Git checkouts. The executor integration tests
exercise the production launch and resume wiring with a recording admission
boundary and assert the selected durable slot identity. A full cross-layer test
that combines a real SQLite claim, real Git recovery, and lifecycle runtime startup
is still a verification gap.

This change does not modify rendered UI. Existing launch and resume error surfaces
remain the presentation contract. No layout preview or artificial browser test is
required for the backend-only change.

## Work orders

- [x] [Task 01: Scope recovery admission](task-01-scope-admission.md)
- [ ] [Task 02: Claim recovery authority](task-02-claim-authority.md)
- [ ] [Task 03: Prove recovery integration](task-03-prove-integration.md)

## Verification results

Implementation checks pass locally. The PostgreSQL concurrency check and a full
real-Git-to-lifecycle integration test remain outstanding.

Design-package checks on 2026-09-10:

- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `node --test scripts/validate-public-docs.test.mjs`: 62 tests passed.
- `node scripts/validate-public-docs.mjs`: 46 published pages validated.
- `git diff --check`: passed.
- `git diff --check -- docs/plans/worktree-metadata-recovery`: passed.
- `git status --short -- docs/plans/worktree-metadata-recovery`: new package
  present as untracked files. No commit or push in this design turn.

Implementation checks on 2026-09-11:

- `go test -vet=off ./internal/worktree -count=1`: passed.
- `go test -vet=off ./internal/task/repository/sqlite -count=1`: passed.
- `go test -vet=off ./internal/task/repository/sqlite -run 'TestTaskEnvironmentRecoveryClaim|TestPostgresTaskEnvironmentRecoveryClaim' -count=1 -v`: SQLite claim tests passed; PostgreSQL test skipped because `KANDEV_TEST_POSTGRES_DSN` was not set.
- `go test -vet=off ./internal/orchestrator/... ./internal/worktree -count=1`: passed.
- `go test -vet=off ./internal/agent/runtime/lifecycle -count=1`: passed with the normal temporary directory.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `node --test scripts/validate-public-docs.test.mjs`: 62 tests passed.
- `node scripts/validate-public-docs.mjs`: 46 published pages validated.
- `git diff --check`: passed.
- `gofmt -l` over `apps/backend/internal`: no files reported.

The public-page addition is explanatory guidance about metadata loss. The page
remains a how-to guide and documents only the implemented automatic safeguards.

## Risks

- A late database write does not exclude an already-started external runtime.
  The claim must guard the actual startup boundary.
- A PostgreSQL test that skips without a DSN does not prove concurrency safety.
- A crash can retain a claim. Recovery must reconcile it without timeout-based release.
- A later slot can fail after an earlier slot commits. The design deliberately
  retains that completed repair instead of removing preserved files.
- The proposed ADR narrows the broad replacement-branch wording in the accepted
  branch-loss decision. It does not authorize fallback to a base branch.
- Existing public docs must not promise these safeguards until implementation
  and its targeted evidence are complete.

## Delivery

After the implementation request, execute each work order with TDD. Refresh the
PR head and review threads before editing and before SSH push. Preserve contributor
changes and resolve only review threads that the changes actually address.

The user already authorized a normal commit, SSH push, and one explanatory PR
comment. After publication, wait for the requested fifteen-minute interval, then
run PR fixup. Signal workflow completion only after the current step is complete.
