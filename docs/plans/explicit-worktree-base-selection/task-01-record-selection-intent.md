---
id: "01-record-selection-intent"
title: "Record base selection intent"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
acceptance_criteria:
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.27
system_design:
  - ../../specs/workspaces/system-design/worktree-base-selection.md
---

# Task 01: Record base selection intent

## Summary

Retain branch-bound selection intent through task creation, inheritance, and
base edits. Derive strict preparation policy unless trusted intent permits
repository-default recovery. This work order prepares the data contract.
Task 02 enables enforcement.

## In scope

- Add the versioned metadata model and zero-value-strict policy helper from
  the [selection contract](../../specs/workspaces/system-design/worktree-base-selection.md#selection-contract).
- Add an internal-only source field to `TaskRepositoryInput`. Capture intent
  before MCP/provider helpers fill defaults. Unknown source values stay strict.
- Record final branch and source in `buildResolvedTaskRepositoryRow`, after
  branch-policy resolution. Selected policy snapshots remain strict.
- Mark both service and MCP inheritance producers. Preserve checkout semantics.
- Preserve supplied-versus-default intent in MCP default filling and
  `AddBranchToTask`, including repository-ID, local-path, and remote-URL inputs.
- Preserve trusted intent through `backendapp/repository_selection.go` copies.
  Public JSON cannot grant permissive recovery.
- Make a manual base selection strict, including same-name selection, through
  the existing locked metadata mutation. Preserve unrelated metadata and
  comparison-target/manual-override behavior.
- Keep source metadata through database reopen and omitted repository updates.
  Supplied repository replacements derive fresh intent.
- For a permitted system fallback update, rebind existing permissive metadata
  under the current row lock. Never manufacture permission for an explicit,
  unknown, or mismatched selection.

## Out of scope

- Enabling the worktree-manager policy gate before Task 02.
- Public selection-source controls, columns, migrations, and guessed backfills.
- New provider retargeting, concurrent prepared-start repair, or UI behavior.

## Acceptance

1. Source intent survives every owned producer and persisted round trip.
   Explicit/default equality, malformed metadata, and legacy rows resolve strictly.
2. Manual reselection revokes permission without losing unrelated attachment
   metadata. An ordinary update that omits repositories retains the record.
3. Producer, update, and reopen regressions pass while existing worktree
   behavior remains compatible until the dependent work order.

## TDD and test matrix

Start with a service test using existing `CreateTask` inputs and inspecting the
expected metadata record. It must fail because intent is absent before this
change. Add the following focused test families as their boundaries change:

- `models/base_branch_selection_test.go::TestBaseBranchSelection_Policy`:
  all sources, exact branch binding, missing/unknown versions, wrong types,
  stale branch, and manual-override precedence.
- `service/service_base_branch_selection_test.go::TestBaseBranchSelection_Producers`:
  explicit branch, explicit repository-default name, service inheritance,
  branch-policy snapshot, and actual default omission.
- The same service file's `TestBaseBranchSelection_UpdateAndAddBranch`:
  explicit same-name edit, omission, replacement, and branch-add input forms.
- `mcp/handlers/base_branch_selection_test.go::TestBaseBranchSelection_MCPInputs`:
  parent/current-task inheritance and explicit input before default filling.
- `repository/sqlite/task_repository_base_branch_selection_test.go::TestBaseBranchSelection_Persistence`:
  JSON round trip, database reopen, manual/system updates, and unrelated metadata.
- `backendapp/repository_selection_test.go::TestBaseBranchSelection_RepositoryAdapters`:
  trusted conversion and public-input rejection of permissive source fields.

Use existing service and SQLite fixtures. Add small test files rather than
growing files beyond backend limits. Do not construct the expected provenance
on a fake launch request and treat it as producer evidence.

## Verification

Run from the repository root. Run each TDD regression before and after its
correction. The final commands are:

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/task/models ./internal/task/service ./internal/task/repository/sqlite ./internal/mcp/handlers ./internal/backendapp -run 'BaseBranchSelection|RepositoryBaseBranch|InheritParentRepositories|InheritedRepoInputs|ExplicitRepoInputsWithDefaults' -count=1)
(cd apps/backend && go run -trimpath ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/persistence/storeconformance -count=1)
bash scripts/lint-go-changed apps/backend/internal/task/models/base_branch_selection.go apps/backend/internal/task/models/base_branch_selection_test.go apps/backend/internal/task/service/service_requests.go apps/backend/internal/task/service/service_tasks.go apps/backend/internal/task/service/service_branches.go apps/backend/internal/task/service/service_base_branch_selection_test.go apps/backend/internal/task/repository/sqlite/task_repository_comparison_target.go apps/backend/internal/task/repository/sqlite/task_repository_base_branch_selection_test.go apps/backend/internal/mcp/handlers/handlers.go apps/backend/internal/mcp/handlers/base_branch_selection_test.go apps/backend/internal/backendapp/repository_selection.go apps/backend/internal/backendapp/repository_selection_test.go
git diff --check
```

Keep SQL dialect-neutral. If persistence tests support
`KANDEV_TEST_POSTGRES_DSN`, run and record that matrix when configured.
An unavailable PostgreSQL instance is not live PostgreSQL evidence.
If an implementation file moves, synchronize this command before completion.

## Files likely touched

- `apps/backend/internal/task/models/base_branch_selection.go` (new) and its tests.
- `apps/backend/internal/task/service/service_requests.go`.
- `apps/backend/internal/task/service/service_tasks.go`.
- `apps/backend/internal/task/service/service_branches.go`.
- `apps/backend/internal/task/service/service_base_branch_selection_test.go` (new).
- `apps/backend/internal/task/repository/sqlite/task_repository_comparison_target.go`.
- `apps/backend/internal/task/repository/sqlite/task_repository_base_branch_selection_test.go` (new).
- `apps/backend/internal/mcp/handlers/handlers.go`.
- `apps/backend/internal/mcp/handlers/base_branch_selection_test.go` (new).
- `apps/backend/internal/backendapp/repository_selection.go` and its tests.

Read `service_branch_update.go`, `service_task_repository_replacement.go`,
`project_repository_sources.go`, and `service_workspace_sources.go` as producer
controls. Change them only where their actual source handling requires it.
Include any changed files in the recorded lint command.

## Dependencies

None. Implement before Task 02.

## Risks

- Filling defaults before capturing omission loses intent.
- Source metadata that is not bound to the final branch can authorize a later edit.
- Conflating provenance with the existing manual comparison marker changes PR behavior.
- Full repository replacement and manual same-name edits can preserve stale permission.

## Parallelism

`sequential`

## Inputs

- [Plan and reproduction](plan.md#confirmed-root-cause-and-evidence).
- [Requirement AC .27](../../specs/workspaces/requirements/worktree-base-refresh.md#selected-base-identity).
- [Producers and persistence](../../specs/workspaces/system-design/worktree-base-selection.md#producers-and-persistence).
- `apps/backend/AGENTS.md` and `.agents/skills/tdd/references/backend-tests.md`.
- Existing `comparison_target.go` metadata helpers and
  `task_repository_comparison_target_test.go` locked-write tests.

## Results

Implemented and verified on 2026-11-08.

- Added the versioned `BaseBranchSelection` metadata contract and the
  zero-value-strict `BaseBranchFallbackPolicy` derivation in
  `internal/task/models/base_branch_selection.go`.
- Captured selection intent in task creation, both inheritance producers,
  MCP default filling, branch addition, workspace-source attachment, branch
  policy snapshots, explicit and system base edits, and replacement copies.
- A manual same-name edit rewrites the record as explicit. System fallback
  rebinds matching permissive intent and leaves every other record strict.
- `TestBaseBranchSelection_*` passes in models, service, SQLite, MCP handlers,
  and backendapp. SQL guard, store conformance, and `lint-go-changed` pass.
- `TestBaseBranchSelection_AddBranchInputForms` covers omitted and explicit
  bases for LocalPath, GitHubURL, and RepositoryID inputs; omission is captured
  before repository resolution can fill `BaseBranch`.
- A PR-linked input with an omitted base records explicit intent with a deferred
  branch, so its later target resolution stays strict. `TestBaseBranchSelection_Policy`
  covers the empty explicit binding.
- Full package suites for task service, SQLite, MCP handlers, and backendapp
  pass with the change.
