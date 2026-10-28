---
id: "01-finalize-cache-aliases"
title: "Finalize cache aliases after physical cleanup"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-003
acceptance_criteria:
  - AC-TASKS-RUNTIME-CLEANUP-001.2
  - AC-TASKS-RUNTIME-CLEANUP-001.3
  - AC-TASKS-RUNTIME-CLEANUP-001.58
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.4
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.6
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.7
system_design:
  - ../../specs/tasks/system-design/runtime-cleanup.md
  - ../../specs/tasks/system-design/workspace-repository-placement.md
---

# Task 01: Finalize cache aliases after physical cleanup

## Summary

Remove every cached alias of a successfully removed physical worktree without
changing shared-reference retention or standalone reference release. Prove that
two-session attachment rollback can retry successfully through the same live
service/manager and that committed retries remain exact no-ops.

## In scope

The [plan's technical approach](plan.md#technical-approach), five-case public
service failure/control matrix, focused alias/isolation tests, existing failure
controls, and the exact verification below. Independently author all new tests
using shipped public fixtures.

## Out of scope

No frontend/UI/mobile or MCP question files; mobile-parity is inapplicable.
No reuse guard, branch policy, placement, SQL/store, migration, reference count,
ownership lock, retained-checkout, or general lifecycle change.

## Acceptance

1. Independently authored public-service RED fails on the expected live
   two-session retry after proving complete rollback for both relevant failure
   points. After the fix the same manager succeeds, durable/physical inventory
   agrees, exact committed retry has no effects, and single-session/rebuilt
   controls pass. No restart/reconstruction is used in either causal case.
2. Successful full removal invalidates all aliases of only the removed nonempty
   physical ID, including a cleanup projection different from creator; sibling
   repositories/branch slots and other physical IDs survive. Empty IDs cannot
   combine unrelated records. Reference-only release, shared active reference,
   audit/removal error, and metadata-release failure preserve existing behavior.
3. The scoped race/lint/document checks pass, all command handles actually join,
   and results are recorded before marking this order done. Production changes
   remain limited to removal finalization; no guard or persistence workaround.

## Files owned

Production after release:

- `apps/backend/internal/worktree/manager_cleanup.go`

New permanent tests after release:

- `apps/backend/internal/worktree/manager_cleanup_cache_test.go`
- `apps/backend/internal/backendapp/workspace_source_materializer_live_retry_test.go`

Documentation:

- `docs/specs/tasks/requirements/runtime-cleanup.md`
- `docs/specs/tasks/system-design/runtime-cleanup.md`
- `docs/plans/live-attachment-retry-cache/plan.md`
- `docs/plans/live-attachment-retry-cache/task-01-finalize-cache-aliases.md`

Existing shipped fixture/test files, `manager_state.go`, `store.go`, lifecycle
consumers, active attachment requirements and placement design are read-only
inputs. If these boundaries cannot deliver the fix, reconcile the package before
expanding scope.

## TDD sequence and execution gate

Implementation followed a later explicit request to implement this package.

After release mark `in_progress`. Write
`TestWorkspaceSourceMaterializer_LiveAttachmentRetry` first in the new file.
Force second inventory insert and second-session rescan failures with existing
test mechanisms; assert rollback before retry. Causal RED must be a runtime
retry failure (expected `ErrBranchUnrecoverable`), not compilation, setup,
failpoint, or rollback assertion failure. Run the controls in that same focused
test and record their separate results. Add the manager cases named in the
plan, then apply the minimal finalization fix. Run focused GREEN and the affected
families once; repeat only for a change/failure that justifies it.

## Verification

Commands run from repository root; every backend command roots itself. Select
the verified existing tools in this task shell without changing global config:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
go version
node --version
```

Run heavy checks sequentially.
Record current available memory before launch. Tests use CPU2/512MiB; scoped
lint uses its separate canonical 3GiB budget, not the test limit.

Focused RED before source changes; repeat the identical command for GREEN:

```bash
(cd apps/backend && timeout --signal=TERM --kill-after=10s 300s env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -race -tags fts5 -p 1 ./internal/backendapp -run '^TestWorkspaceSourceMaterializer_LiveAttachmentRetry$' -count=1 -timeout=180s -v)
```

Affected race tests include every new test and existing physical/shared-reference,
release-failure, branch-recovery and persisted attachment controls:

```bash
(cd apps/backend && timeout --signal=TERM --kill-after=10s 300s env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -race -tags fts5 -p 1 ./internal/backendapp ./internal/worktree -run '^Test(WorkspaceSourceMaterializer_(LiveAttachmentRetry|PersistedAttachment.*)|CleanupWorktrees.*|CleanupArchivedWorktree.*|RemoveByID.*|ReleaseWorktreeReference.*|Create_RestoresReleasedWorktreeAfterArchive)$' -count=1 -timeout=180s -v)
```

Canonical scoped lint after implementation (all three actual changed Go paths;
adjust only to the actual A/M list if authorized scope changes). It expands to
the two complete owning packages. Do not add full-backend lint:

```bash
timeout --signal=TERM --kill-after=10s 360s env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=3GiB bash scripts/lint-go-changed apps/backend/internal/worktree/manager_cleanup.go apps/backend/internal/worktree/manager_cleanup_cache_test.go apps/backend/internal/backendapp/workspace_source_materializer_live_retry_test.go
```

No persistence mutation is planned, so SQLguard/store conformance are not a
default gate. If an authorized actual persistence change occurs, add and run
these bounded commands, documenting its reason. PostgreSQL runtime requires an
actual DSN and executed applicable cases; an env-gated skip is not coverage:

```bash
(cd apps/backend && timeout --signal=TERM --kill-after=10s 300s env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go run -trimpath -p 1 ./cmd/sqlguard ./internal)
(cd apps/backend && timeout --signal=TERM --kill-after=10s 300s env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -race -tags fts5 -p 1 ./internal/persistence/storeconformance -count=1 -timeout=180s)
```

Run these cheap design checks after final documentation changes:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/live-attachment-retry-cache
git status --short --untracked-files=all -- docs/plans/live-attachment-retry-cache
```

Use the repository preflight on actual modified AND added paths, including
untracked files, not just `git diff --name-only`. This reads working files and
does not stage or commit them. Run from repository root:

```bash
node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const entries = execFileSync('git', ['status', '--porcelain=v1', '-z', '--untracked-files=all'], { encoding: 'utf8' }).split('\0').filter(Boolean);
const changedFiles = entries.map(entry => {
  const code = entry.slice(0, 2);
  if (![' M', 'M ', 'A ', '??'].includes(code)) throw new Error(`Unexpected status ${code}`);
  return { filename: entry.slice(3), status: code.includes('A') || code === '??' ? 'added' : 'modified' };
});
const references = [
  'docs/plans/live-attachment-retry-cache/plan.md',
  'docs/plans/live-attachment-retry-cache/task-01-finalize-cache-aliases.md',
  'docs/specs/tasks/requirements/runtime-cleanup.md',
  'docs/specs/tasks/requirements/attach-workspace-sources.md',
  'docs/specs/tasks/system-design/runtime-cleanup.md',
  'docs/specs/tasks/system-design/workspace-repository-placement.md',
];
const fileContents = Object.fromEntries(references.map(path => [path, fs.readFileSync(path, 'utf8')]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
git diff --check
```

The design turn additionally runs a prospective preflight with the three planned
Go paths supplied as source triggers; label it prospective, never implemented.
Inspect actual untracked content for whitespace too (`git diff --check` alone
does not see it). Confirm exactly one manifest and one work order, existing
REQ/AC identities, every requirement declared by a referenced design, and every
order design included by the plan. Check current ownership of the proposed
paths before implementation.

## Dependencies

Merged PR #4807. No preceding work order.

## Risks

See the [plan](plan.md#risks). Preserve existing partial-failure behavior and
stable physical identity; do not make SQL session projection order an authority.

## Parallelism

`sequential`

## Inputs

- [Runtime cleanup requirements](../../specs/tasks/requirements/runtime-cleanup.md)
  and [design](../../specs/tasks/system-design/runtime-cleanup.md#worktree-cache-finalization).
- [Attachment requirements](../../specs/tasks/requirements/attach-workspace-sources.md)
  and [placement design](../../specs/tasks/system-design/workspace-repository-placement.md).
- `apps/backend/AGENTS.md`; manager cleanup/state/store source; shipped real
  inventory fixture; shared-reference, release-recovery, and cleanup safety tests.
- Existing implemented attachment inventory and branch/lifetime packages are
  read-only history.

## Results

Completed on 2026-10-26:

- Added the five-case public-service regression using shipped SQLite/Git
  fixtures. Both causal RED cases failed at live retry after complete rollback;
  all three controls passed. The identical focused GREEN command passed all
  five cases (backendapp 4.766s), including committed exact no-op and reopen.
- Added differing-session alias removal for both cleanup modes with/without a
  store, sibling repository/branch/physical-ID preservation, standalone release,
  shared reference, audit/removal failure, metadata-release failure, and empty-ID
  isolation. Corrected fixture metadata before the clean manager RED, which
  exposed all four stale-alias removal cases while its controls passed.
- Changed only successful finalization in `manager_cleanup.go`; standalone
  release, physical cleanup order, persistence, and reuse guards are unchanged.
- The exact affected race command above passed: backendapp 8.440s and worktree
  22.313s. Every new regression ran after the final production edit.
- The canonical scoped lint command above passed with zero issues (3GiB),
  separately from the CPU2/512MiB test commands. Conditional persistence checks
  were inapplicable; no PostgreSQL runtime claim is made.
- Catalog validation, 36 spec-linter self-tests, full spec lint, actual seven-path
  documentation coverage, and diff/added-file whitespace checks passed.

One frozen dependency installation completed with no manifest/lockfile change.
External receipts retain exact command exits, joins, and original fixture
failures; those fixture failures are not claimed as causal product RED.
