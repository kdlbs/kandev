---
id: "01-publish-placement"
title: "Publish complete attachment inventory"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-003
acceptance_criteria:
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.2
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.4
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.6
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.7
system_design:
  - ../../specs/tasks/system-design/attach-workspace-sources.md
  - ../../specs/tasks/system-design/workspace-repository-placement.md
---

# Task 01: Publish complete attachment inventory

## Summary

Preserve the attachment's server-recorded path and position when physical worktree creation first
inserts canonical inventory. Prove the real persistence path and its existing rollback/retry
semantics without changing resume admission or repairing historical inventory.

## In scope

Carry path/position through request, record, insert and readback as described in the
[plan](plan.md#technical-approach). Preserve existing placement on conflict. Add the plan's named
SQLite/real-Git integration tests and shared store contract with an environment-gated PostgreSQL
case. Use public service attachment in compensation/retry coverage; no nil-store substitute.

## Out of scope

Legacy automatic repair, consumer/guard changes, migrations, timestamps, new UI/mobile work,
unproven dialog/send-error fixes, root expansion, and retained-checkout or lifecycle redesign.

## Acceptance

1. Real-store attachment records every new repository/branch slot with the selected task-relative
   path, nonzero order, and exact physical identity; nested and parent-root cases retain these
   values across reopening and fresh-manager reuse. Same-repository branches remain distinct.
2. A persistence or post-create batch failure retains all prior slots/resources and compensates
   new source rows, active inventory, filesystem/exclusion/runtime changes. Retry after manager reconstruction succeeds at
   the same placement; a committed exact retry is a no-op without duplicate slots or restarts.
3. Conflict writes preserve existing canonical path, position and row identity, including callers
   omitting placement. Resume remains fail-closed; all listed targeted checks pass or document
   the PostgreSQL skip without claiming runtime PostgreSQL coverage.

## Files owned

Production changes are limited to:

- `apps/backend/internal/backendapp/branch_materializer.go`
- `apps/backend/internal/worktree/worktree.go`
- `apps/backend/internal/worktree/manager_state.go`
- `apps/backend/internal/worktree/store.go`

New permanent tests after release:

- `apps/backend/internal/backendapp/workspace_source_materializer_inventory_test.go`
- `apps/backend/internal/worktree/store_placement_test.go`
- `apps/backend/internal/worktree/store_placement_postgres_test.go`

Documentation owned:

- `docs/specs/tasks/system-design/workspace-repository-placement.md`
- `docs/plans/attached-worktree-inventory/plan.md`
- `docs/plans/attached-worktree-inventory/task-01-publish-placement.md`

The active requirement file is reused without editing. Existing attachment writers, cleanup,
executor reuse, migrations, shared fixtures, public docs and other spec pairs are read-only inputs.
If meaningful regressions expose a required change outside these files, report the scope change
to ROOT before expanding it.

## TDD sequence

After explicit ROOT release, mark this work order `in_progress` and acquire the heavy lease.
Independently author `TestWorkspaceSourceMaterializer_PersistedAttachmentPlacement` first,
using existing fixture helpers as patterns, never private ROOT evidence. Run RED before production
edits. Expected failure: physical worktree exists and materialization succeeds, but the selected
new canonical row has empty path and position 0. A compilation/setup failure is not causal RED.

Add store insert/conflict and persisted rollback/retry coverage named in the plan. Make the four
production-file correction, then run focused GREEN and the affected regression families. Do not
add metadata repair to make old seeded corruption pass. Retain all command handles and record
actual exits; join owned commands before returning the lease. Only then mark `done` and synchronize
the plan. No delegation, new session, or model switch.

## Verification

Run from the repository root. Use task-local toolchain PATH only; no trust/global environment
changes. No dependency install is needed for these Go checks. Run heavy commands sequentially
under the ROOT lease, with a distinct task-owned quiet directory for each invocation.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export GOTOOLCHAIN=local
task4797_evidence="/home/jcfs/.kandev/loop-evidence/1a588848-c703-480a-bc22-1eb3f18f8b10"
mkdir -p "$task4797_evidence"
go version
node --version

# RED, before production edits. Repeat for focused GREEN; mktemp makes a fresh directory.
KANDEV_RUN_QUIET_DIR="$(mktemp -d "$task4797_evidence/focused.XXXXXX")" bash scripts/run-quiet backend-test -- timeout --kill-after=10s 300s env GOMAXPROCS=2 GOMEMLIMIT=512MiB go -C apps/backend test -trimpath -race -tags fts5 -p 1 ./internal/backendapp -run '^TestWorkspaceSourceMaterializer_PersistedAttachmentPlacement$' -count=1 -timeout=180s

# GREEN and existing affected behavior, after the correction.
KANDEV_RUN_QUIET_DIR="$(mktemp -d "$task4797_evidence/affected-green.XXXXXX")" bash scripts/run-quiet backend-test -- timeout --kill-after=10s 600s env GOMAXPROCS=2 GOMEMLIMIT=512MiB go -C apps/backend test -trimpath -race -tags fts5 -p 1 ./internal/backendapp ./internal/worktree ./internal/task/service -run 'TestWorkspaceSourceMaterializer_|TestWorkspaceSourceInventory|TestBranchMaterializer_|TestSQLiteStore_|TestAttachWorkspaceSources' -count=1 -timeout=300s

# Required persistence checks; PostgreSQL cases use the existing DSN gate.
KANDEV_RUN_QUIET_DIR="$(mktemp -d "$task4797_evidence/sqlguard.XXXXXX")" bash scripts/run-quiet backend-test -- timeout --kill-after=10s 300s env GOMAXPROCS=2 GOMEMLIMIT=512MiB go -C apps/backend run -trimpath -race -tags fts5 -p 1 ./cmd/sqlguard ./internal
KANDEV_RUN_QUIET_DIR="$(mktemp -d "$task4797_evidence/conformance.XXXXXX")" bash scripts/run-quiet backend-test -- timeout --kill-after=10s 600s env GOMAXPROCS=2 GOMEMLIMIT=512MiB go -C apps/backend test -trimpath -race -tags fts5 -p 1 ./internal/persistence/storeconformance -count=1 -timeout=300s

# Canonical changed-file lint expands to owning packages. Keep its canonical default 3GiB memory budget.
KANDEV_RUN_QUIET_DIR="$(mktemp -d "$task4797_evidence/lint.XXXXXX")" bash scripts/run-quiet backend-lint -- timeout --kill-after=10s 600s bash scripts/lint-go-changed apps/backend/internal/backendapp/branch_materializer.go apps/backend/internal/backendapp/workspace_source_materializer_inventory_test.go apps/backend/internal/worktree/worktree.go apps/backend/internal/worktree/manager_state.go apps/backend/internal/worktree/store.go apps/backend/internal/worktree/store_placement_test.go apps/backend/internal/worktree/store_placement_postgres_test.go

python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/attached-worktree-inventory
```

Every quiet invocation above creates a fresh task-owned directory outside the tracked checkout.
Record its chosen directory, actual exit and join; never prune shared or foreign logs. GNU timeout
bounds compilation as well as tests; retain the inner Go test deadlines. Keep tests at 512MiB
and canonical lint at its separate 3GiB default. The cache-compatible test flags are required on
RED, focused GREEN and affected tests.

Run the working-file documentation coverage preflight from the repository root. It reads actual
added/modified paths and all five contract documents from the working tree. The design-only
receipt used a prospective source trigger; implementation evidence must use actual paths.

```bash
node <<'NODE'
const fs = require('node:fs');
const {execFileSync} = require('node:child_process');
const {validateCoverage} = require('./.github/scripts/pr-docs.cjs');
const docs = [
  'docs/plans/attached-worktree-inventory/plan.md',
  'docs/plans/attached-worktree-inventory/task-01-publish-placement.md',
  'docs/specs/tasks/requirements/attach-workspace-sources.md',
  'docs/specs/tasks/system-design/attach-workspace-sources.md',
  'docs/specs/tasks/system-design/workspace-repository-placement.md',
];
const result = validateCoverage({
  changedFiles: execFileSync('git', ['status', '--porcelain=v1', '-z', '--untracked-files=all'], {encoding:'utf8'})
    .split('\0').filter(Boolean).map(line => ({
      filename: line.slice(3), status: /[?A]/.test(line.slice(0, 2)) ? 'added' : 'modified',
    })),
  fileContents: Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify(result));
if (!result.ok || result.status !== 'covered') process.exitCode = 1;
NODE
```

## Dependencies

None. ROOT design review, later explicit implementation release, and heavy lease are required
operating checkpoints. PR #4722 and PR #4798 are overlap checks, not code dependencies.

## Risks and impact

Initial/recreate callers may omit placement, so the conflict clause must preserve it. Real-store
cleanup can retain inactive history; prove exactly one active slot after retry and no surviving
failed active attachment. No public wire/schema/UI change is intended. Public docs and mobile
assessment are in the plan; no browser validation is required for this persistence-only change.

## Parallelism

`sequential`

## Inputs

- [Owning requirements](../../specs/tasks/requirements/attach-workspace-sources.md), REQ-001/003
  and the four full acceptance IDs in frontmatter.
- [Attachment design](../../specs/tasks/system-design/attach-workspace-sources.md) and
  [placement design](../../specs/tasks/system-design/workspace-repository-placement.md#canonical-inventory-publication).
- `apps/backend/AGENTS.md` and `.agents/skills/tdd/references/backend-tests.md`.
- `branch_materializer_test.go`: `newPersistingMaterializerWorktreeMgr`, `newMaterializerRepo`,
  `seedMaterializerTask`, `setupMaterializerScenario`.
- Existing `workspace_source_materializer_host_test.go`, service retry tests, store tests, and
  store PostgreSQL fixture conventions. `executor_environment_reuse.go` is read-only consumer context.

## Results

Implementation checks completed on 2026-10-26, sequentially under the ROOT heavy lease:

- Focused regression RED: exit 1, all three placements exposed empty canonical paths after
  successful materialization; no production edits preceded it. Focused GREEN: exit 0.
- Revised real-store matrix: exit 0, including strong two-session rollback and public service
  retry after manager reconstruction. The original same-manager failure is preserved separately.
- The exact affected-test command in Task 01: exit 0, backendapp 13.839s, worktree 4.204s,
  task/service 4.406s. Every new regression ran after the final production edit.
- The exact SQL guard command in Task 01: exit 0.
- The exact race-enabled store-conformance command in Task 01: exit 0, 52.313s.
- The exact canonical changed-file lint command in Task 01: exit 0 with its default 3GiB budget;
  available memory was 15439MiB at launch. New paths were intent-to-add for diff coverage.
- PostgreSQL cases skipped at the existing DSN gate; no PostgreSQL runtime coverage is claimed.
- Catalog validation, full specification lint, actual added/modified-path `validateCoverage`
  (`covered`, no errors, ten owned paths), and diff/working-file whitespace checks passed.

Command startup/join receipts and full logs are retained in task-owned external evidence under
`/home/jcfs/.kandev/loop-evidence/1a588848-c703-480a-bc22-1eb3f18f8b10/` in `red.ga1roypz`,
`focused-green.tiiu39k_`, `reconstructed-retry-2.aie_eh_m`, `affected-green.x78egpk6`,
`sqlguard.7l8qolah`, `conformance.7biuzk1n`, and `scoped-lint.b7kc0nav`.
All these commands actually joined, and their known owned process groups were absent.
The transient incomplete test fixture/import failures remain in their original receipts;
they are not product RED evidence. Normal frozen dependency install, commit hooks and hosted
validation are delivery evidence recorded in the Kandev task plan, not another product test gate.

The existing placement design remains draft for unrelated incomplete placement scope; the active
requirements are reused unchanged. No schema, public API, UI, localization or resume guard changed.


## Known deferred limitation

The two-session real-store fixture exposed an independent same-manager live retry failure: task-level
cleanup can project a different session than the creation cache key, leaving the creator session
with stale worktree cache state after cleanup. The next live retry can enter recreation of a deleted
generated branch and fail with `ErrBranchUnrecoverable`. ROOT approved preserving both-session
rollback assertions and proving retry after manager reconstruction against the same persisted DB,
which models the restart boundary relevant to #4797. Committed exact retry remains a no-op.
This package does not fix or claim coverage of successful same-manager multi-session retry.
The original fixture and actual native62766 exit1 log/receipt are retained in child-owned external
evidence; no persistent follow-up task or issue is created.
