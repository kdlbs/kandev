---
id: "01-preserve-legacy-admission"
title: "Preserve unchanged legacy checkout admission"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-001
acceptance_criteria:
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.1
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.3
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.5
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.6
system_design:
  - ../../specs/tasks/system-design/managed-clone-relocation.md
---

# Task 01: Preserve unchanged legacy checkout admission

## Summary

Distinguish unchanged registered legacy clones from actual source-clone moves.
Reuse a verified existing checkout while retaining all relocation safety checks.

## In scope

- Shared linked/main admission proof and real-Git regression matrix in the plan.
- Executor integration regression using the actual cloner and persisted state.
- Record red/green evidence and update this work order and plan after validation.

## Out of scope

New UI, live database edits, clone materialization during resume, broader transfer
support, and changes to provider credential policy.

## Acceptance

1. Healthy registered legacy clones pass with the computed destination absent
   or present; branch, index, submodule, ignored content, and session stay intact.
2. Wrong origin, arbitrary/foreign source, invalid registration, and missing
   registered workspace destination still refuse without mutation. Mixed slots
   validate every repository before launch.
3. Existing clean/dirty relocation and restart recovery tests continue to pass.

## Verification

Run the first regression before implementation and confirm the expected missing
managed destination error. Then implement and run all commands from repo root:

```bash
(cd apps/backend && go test ./internal/worktree -run 'TestManagerAdmitRecoveryReusesRegisteredLegacyClone' -count=1)
(cd apps/backend && go test ./internal/worktree ./internal/repoclone ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle ./internal/task/service)
(cd apps/backend && go test ./internal/worktree ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle -run 'LegacyClone|LegacyReuse|ManagedClone|ManagedMain|MainCheckout|WorktreeRecovery(Launch|Resume)Integration|RegisteredLegacy' -count=1)
(cd apps/backend && go test ./internal/worktree -run 'Relocat|PreservesLegacyCheckoutContent|ParseManagedGitOrigin' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

On macOS, use a canonical temporary root and isolated HOME/Git configuration.
The full package command is also the Linux CI gate. Existing macOS missing-checkout
`/dev/fd/3` errors, shell timeout fixtures, Docker preparation, and Unix socket
path-length failures are outside this change. If reproduced with the original
production file through a Go overlay, record them as baseline limitations; the
focused commands above and all new regressions must pass before delivery.

## Files likely touched

- `apps/backend/internal/worktree/managed_clone_relocation.go`
- `apps/backend/internal/worktree/managed_clone_legacy_reuse.go`
- `apps/backend/internal/worktree/managed_clone_legacy_reuse_test.go`
- `apps/backend/internal/orchestrator/executor/executor_legacy_clone_recovery_test.go`
- `docs/public/git-operations.md`

## Dependencies

None.

## Risks

Do not bypass recovery based on path strings or a missing destination alone.
Keep context cancellation and filesystem errors distinct from a verified match.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/managed-clone-relocation.md)
- [Design](../../specs/tasks/system-design/managed-clone-relocation.md), unchanged legacy admission.
- [Existing relocation plan](../managed-clone-relocation/plan.md), completed baseline.
- [Relocation ADR](../../decisions/2026-09-27-managed-clone-relocation-boundary.md)
- Existing real-Git helpers and relocation recovery store in the worktree tests.

## Results

Completed on 2026-10-01.

- Red: all eight initial real-Git legacy layout/provider/main-linked cases failed
  with `managed destination clone identity is invalid` before production edits.
- Green: all new legacy reuse/content/refusal/cancellation/case/symlink tests pass.
- SQLite-backed executor preflight passes with real cloner proof and unchanged
  persisted environment and session metadata.
- Focused worktree/executor/lifecycle admission suites passed. Existing clean and
  dirty relocation, restart, multi-repository refusal, origin parsing, and content
  preservation suites passed (separate worktree run: 12.718s).
- Full repoclone suite passed (17.929s); full task-service suite passed (55.158s).
- Full five-package command was run twice, first in the host environment and
  then with isolated HOME/Git configuration and canonical temporary paths.
  Remaining worktree/executor/lifecycle failures were macOS `/dev/fd/3` directory
  handling, a shell timeout fixture, Unix socket path length, and Docker daemon
  preparation. Representative failures in every affected package were reproduced
  with a Go overlay restoring the original production file. They are recorded
  baseline limitations, not passing checks; Linux CI retains the full suite gate.
- Changed-package golangci-lint: zero issues.
- Catalog validation, specification lint, 36 spec-linter tests, 62 public-doc
  validator tests, validation of 47 published pages, PR documentation coverage
  preflight, and `git diff --check` passed.

The initial case-alias fixture was corrected to keep request and persisted
repository paths consistent; the final case and symlink alias regressions pass.
No live installation or database was modified during implementation.
