---
id: "01-preserve-branch-binding"
title: "Preserve proven branch bindings through title renames"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TURN-CHANGES-002
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-004
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-002.3
  - AC-TASKS-TURN-CHANGES-002.4
  - AC-TASKS-TURN-CHANGES-002.12
  - AC-TASKS-TURN-CHANGES-002.13
  - AC-TASKS-TURN-CHANGES-002.14
  - AC-TASKS-TURN-CHANGES-002.15
  - AC-TASKS-TURN-CHANGES-003.1
  - AC-TASKS-TURN-CHANGES-003.2
  - AC-TASKS-TURN-CHANGES-003.3
  - AC-TASKS-TURN-CHANGES-004.1
system_design:
  - ../../specs/tasks/system-design/turn-change-history-integrity.md
---

# Task 01: Preserve proven branch bindings through title renames

## Summary

Carry the verified old generated branch from title resolution to the Git mutation boundary.
Retain immutable branch-binding evidence so comparison accepts that rename and still rejects a genuine branch switch.
This work order delivers the complete backend path.

## In scope

- Read the plan evidence and the design's Generated-branch continuity section before implementation.
- Start TDD with `TestHandleGitRenameGeneratedBranchPreservesTurnComparison`.
  Capture start. Send `expected_generated_branch` and `new_name` to the existing real rename route. Capture end.
  Assert a complete direct comparison with zero files.
  Today's handler ignores the extra field, performs the rename, and fails this comparison assertion.
  A missing symbol or fixture error does not count as behavioral RED.
- Add optional `Provenance.HeadBindingID`, immutable JSON round-trip coverage, and strict validation for continuity use.
- Keep one synchronized live binding per cached checkout operator.
  Transfer it only across a verified generated rename under the capture lock.
  Preserve actual `HeadRef`, existing snapshot OIDs, user index, and Git state.
- Extract a shared branch-binding predicate for context and unborn-first-root eligibility.
  Keep all independent history gates.
- Add proposed process method `RenameGeneratedBranch`, client method `GitRenameGeneratedBranch`, and lifecycle method `RenameGeneratedBranchForSession`.
  Keep existing generic rename calls compatible.
- Replace the title runtime seam with one that requires the old generated branch and explicit primary marker.
  Pass the verified branch from `performTitleBranchRename`; retain current owner and repository admission.
- Preserve snapshot updates, failure outcomes, title acceptance, and scoped metadata publication.
- Cover no-op and failed rename, old-ref mismatch, multiple verified renames, local edits, dirty baselines, and exact exported content.
- Cover same-commit checkout, generic rename, malformed/missing proof, and conflicting target/operation/history evidence.
- Cover mixed repositories, non-owner sessions, explicit remote checkouts, and Local-executor preservation through existing title tests.
- Cover finalized receipt replay with a fresh operator and conservative process replacement between endpoints.
  Idempotent start capture must not hydrate a live binding from its old receipt.
- Use channel barriers to prove capture/rename ordering and race refusal.
  Keep the existing bounded capture and operation deadlines.

## Out of scope

Browser fixtures, public documentation, styling, new capture policies, migrations, and historical repair.
Do not add a persistent branch journal or infer evidence from Git reflogs.

## Acceptance

1. The real title path reaches guarded agentctl rename with the expected old branch, correct repository, and primary identity.
2. Proven renames compare the original endpoints correctly, including zero changes, edits, and later receipt replay.
3. Unproven or unsafe histories remain unavailable, and existing title ownership, snapshot, and capture invariants pass their regressions.

## Verification

Run from the repository root. The first command covers behavioral tests; the second checks application wiring by compilation.
New tests must appear in verbose output; a zero-match invocation is not regression evidence.

```bash
(cd apps/backend && go test -trimpath -race ./internal/common/turnchanges ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/mcp/handlers -run 'Test(ProvenanceHeadBinding|TurnCheckpoint|HandleGitRename|GitRenameGeneratedBranch|GitOperations_PostExpectedPathAndPayload|RenameGeneratedBranch|RenameBranchForSession|HandleSetTaskTitle)' -count=1 -v)
(cd apps/backend && go test -trimpath ./internal/backendapp -run '^$')
git ls-files --modified --others --exclude-standard -z -- '*.go' | xargs -0 -r bash scripts/lint-go-changed
git diff --check
```

The command includes existing history-adjustment tests so a valid binding cannot bypass unrelated safety checks.
Add new cases to focused files; do not exceed the repository's function and file limits.

## Files likely touched

- `apps/backend/internal/common/turnchanges/contract.go`
- `apps/backend/internal/common/turnchanges/provenance_binding_test.go` (new)
- `apps/backend/internal/agentctl/server/process/git.go`
- `apps/backend/internal/agentctl/server/process/git_generated_branch.go` (new)
- `apps/backend/internal/agentctl/server/process/turn_checkpoint_provenance.go`
- `apps/backend/internal/agentctl/server/process/turn_checkpoint_branch_binding.go` (new)
- `apps/backend/internal/agentctl/server/process/turn_checkpoint_branch_binding_test.go` (new)
- `apps/backend/internal/agentctl/server/process/turn_checkpoint_history.go`
- `apps/backend/internal/agentctl/server/api/git.go`
- `apps/backend/internal/agentctl/server/api/git_generated_branch_rename_test.go` (new)
- `apps/backend/internal/agent/runtime/agentctl/git.go`
- `apps/backend/internal/agent/runtime/agentctl/git_generated_branch_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/manager_workspace_branch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_generated_branch_test.go` (new)
- `apps/backend/internal/orchestrator/task_title_branch_runtime.go`
- `apps/backend/internal/orchestrator/task_title_branch_service_test.go` (existing stub adaptation)
- `apps/backend/internal/orchestrator/task_title_branch_binding_test.go` (new behavioral cases)

Existing MCP handler tests validate the acceptance boundary without a handler production change.
`backendapp/orchestrator.go` already wires the lifecycle manager; its compile check must satisfy the new narrow seam.

## Dependencies

None. Use the current history-integrity and title-renaming implementations.

## Risks

- Actual checkout state can differ from the stored generated branch.
  Reject that mismatch before mutation and never rename an unrelated current branch.
- An ID match must not skip other history checks or authorize sibling repositories.
- Replaced agentctl lacks live continuity evidence. Preserve an unavailable result instead of restoring an ID from a receipt.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md), including diagnostic evidence and compatibility matrix.
- [Repository interval requirements](../../specs/tasks/requirements/turn-changed-files.md#req-tasks-turn-changes-002-repository-interval-accuracy).
- [History design](../../specs/tasks/system-design/turn-change-history-integrity.md#generated-branch-continuity).
- [Binding decision](../../decisions/2026-10-30-turn-checkpoint-branch-binding.md).
- Existing helpers `setupHistoryTarget`, `captureHistoryEndpoint`, `compareHistoryEndpoints`, and `acceptedTurnCheckpointPair`.
- Existing title, API, transport, and lifecycle test fixtures.

## Results

Completed on 2026-10-30.

- Behavioral RED: `TestHandleGitRenameGeneratedBranchPreservesTurnComparison` reached the real
  rename route and failed with `history_unavailable` / `history_context_changed` and zero files.
  After the guarded path it passes with a complete direct zero-file comparison.
- Added `Provenance.HeadBindingID` and `SameBranchBinding`; per-operator live bindings transfer
  only across a verified generated rename, and receipts replay without live state.
- Added `RenameGeneratedBranch`, `GitRenameGeneratedBranch`, and
  `RenameGeneratedBranchForSession`; ordinary rename payloads remain unchanged.
- Regression coverage includes edits and exported content, same-commit switches, a verified rename
  followed by recreating the original branch name at the same HEAD, a verified rename followed by a
  generic rename and recreating the intermediate name, generic renames, missing/malformed IDs,
  target changes, active operations, expected-ref refusal, capture/rename serialization,
  fresh-operator receipt replay, process replacement, mixed repositories, and primary/repository
  scoping. Known binding IDs must match even when both receipts name the same ref; receipts without
  IDs keep the legacy equal-ref fallback.
- Verification: the work-order `go test -race` command passed for all seven packages;
  `backendapp` compiled; changed-Go lint reported 0 issues; `git diff --check` passed.
- Review follow-up: the targeted `TestTurnCheckpoint(BranchBinding|GeneratedBranch)` race run and
  `internal/common/turnchanges` race run passed after rejecting equal-ref/different-ID evidence.
- No explicit skips.
