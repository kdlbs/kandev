---
created: 2026-10-01
status: complete
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-006
system_design:
  - ../../specs/system-page/system-design/managed-go-cache-launch-fallback.md
legacy_specs: []
---

# Fix Plan: Managed Go-cache Launch Fallback

## Overview

Remove the fatal lifecycle dependency on optional Go-cache preparation while preserving strict maintenance validation.
Implement the lifecycle correction first. Then prove recovery dispatch, filesystem preservation, and operator documentation.
The user explicitly authorized implementation. Work orders were completed sequentially; no delegation or host repair occurred.

## Evidence and root cause

The reviewed handoff is `/tmp/kandev-go-cache-handoff-crvi0G/delegation-prompt.md`.
Repository work uses this task's checkout rather than the handoff's `/root/kandev2` checkout.
Read-only source inspection confirms the same affected symbols and error propagation.

For task `d6a08c2a-4435-4f2d-a72c-906c219d6fea`, session `cca9bbad-296b-4041-98cc-4ddcd2b5a5db`, retained logs show:

- At 23:32:14 local time on 2026-10-01, initial launch failed on the managed-cache symlink validation.
- At 23:39:10 and 23:39:17, `fresh_start` reached the backend and failed on the same cache check.
- The active rotated evidence file is `/root/.kandev/logs/backend-logs-2026-10-01-000024.log`.
- A read-only settings query still shows enabled management and adopted path `/root/.cache/go-build`.
- At this planning inspection, that path exists as a real directory, not a symlink.
- Later logs show a retry at 23:40:37 and session `RUNNING` at 23:41:04.

The handoff reports an earlier symlink to `/tmp/kandev-go-build-preserved-20261001` and agent evidence of its creation.
This turn did not inspect unrelated transcripts, change the cache, or establish who repaired it.
Historical disk exhaustion remains unproven. Current disk capacity does not explain the earlier agent action.
Host recovery does not change the source defect.

`gocache.Provider.ExecutionEnvironment` rejects the selected symlink through `storage.ValidateNoSymlinkPath`.
`Manager.prepareManagedGoCacheEnvironment` wraps the error as `prepare managed Go cache`.
`launchInternal` and `prepareExecutionEnvironment` propagate it before runtime creation.
Start fresh changes conversation identity but still uses the install-wide adopted cache setting.

At planning time, the smallest reproduction used a temporary home, enabled settings, and an adopted path symlinked to a sentinel directory.
The pre-fix lifecycle returned the symlink error instead of permitting fallback; the planned red regression was `TestManagedGoCacheFallbackAdoptedRootSymlink`.
Historical logs and the source trace provided reproduction evidence for the design package. The completed implementation and regression results are recorded below and in the work orders.

A second defect becomes relevant once launch fallback exists:
`prepareExecutionCreateRequest` copies old metadata and replaces its cache key only after successful preparation.
Without explicit removal, `processEnvironment` can force that old path into later shell commands.

## Contract and ownership

[Storage maintenance requirements](../../specs/system-page/requirements/storage-maintenance.md) now include active requirement `REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-006`.
The prior requirements did not define fatal startup on optional cache failure.
This package fills that behavioral gap and corrects the lifecycle design.
Storage owns the durable setting and maintenance authority, even though lifecycle integration changes task availability.
Adjacent task launch recovery and agent resume contracts retain their existing conversation and admission semantics.

- [System design](../../specs/system-page/system-design/managed-go-cache-launch-fallback.md)
- [Decision](../../decisions/2026-10-01-optional-managed-go-cache.md)
- [Completed storage package](../storage-maintenance/plan.md)
- [Original managed-cache work order](../storage-maintenance/task-05-managed-go-cache.md)

The completed original package remains historical evidence.
Its links identify this follow-up without reopening its completed work or claiming that it covered fallback.

## Scope

### In scope

- Optional fallback for settings, validation, filesystem, and invalid-provider-output errors.
- Caller cancellation and wrapped provider cancellation/deadline preservation.
- One cache decision per execution, including workspace promotion and coalesced creation.
- Safe replacement of stale managed metadata during recovery.
- Ordinary environment resolution and fixed warning coverage.
- Backend recovery dispatch and conservative maintenance regressions.
- Public operations guidance describing fallback, retained settings, and strict maintenance safety.

### Out of scope

Host repair, cache data movement, persisted option changes, new toggles, dependencies, UI controls, and global path-safety changes.
No browser markup or localized copy changes are planned.

## Technical approach

Keep strict errors in `gocache.Provider` and shared path safety.
Change only the lifecycle caller's availability policy.
Before selecting a new cache decision, clear managed-only state and preserve request input.
Apply successful overrides during existing final environment composition rather than mutating input `Env` in the helper.
Resolve creation only after the existing-session lookup. Promotion uses its established runtime environment.
Recovery removes copied `managed_go_cache_path` metadata before adding a current successful path.

The warning uses a fixed message and finite reason values without raw error fields.
The provider is called once for a new execution and never by promotion or non-host executors.
Cancellation checks bracket provider preparation, including empty and successful returns.
No new schema, setting, registry entry, or environment tier is needed.

| Execution boundary | Cache result | Behavior | Evidence |
| --- | --- | --- | --- |
| Local, `local_pc`, worktree, legacy empty type | Valid absolute path | One managed override in every process environment | Lifecycle valid-path propagation |
| Same host-local types | Provider error or relative path | Ordinary environment, warning, launch continues | Real symlink provider and deterministic error fixtures |
| Disabled management or absent provider | No override | Ordinary environment, no skip warning | Disabled and absent-provider cases |
| Workspace promotion or coalesced join | Existing snapshot | Reuse environment and metadata, no second cache probe | Promotion and join regressions |
| Docker, remote Docker, SSH, Sprites, Kubernetes, plugin executor types | Host provider unsupported | Do not call host provider or retain host-managed metadata | Counted-provider negative cases |
| New recovery execution | Old metadata plus failed preparation | Rebuild normal definitions and discard old managed authority | Recovery environment and process assertions |
| Any new launch | Caller/provider cancellation or deadline | Return cancellation/deadline, no executor creation | Positive control plus channel-controlled cancellation |

## Test design

The work orders used this plan-time test design for TDD. Actual tests added and their command outcomes are recorded in the completed work orders; names below are targets, not claims that every proposed name exists.

| Acceptance criteria | File | Test targets |
| --- | --- | --- |
| `.1`, `.3`, `.8`, `.9` | `apps/backend/internal/agent/runtime/lifecycle/manager_managed_go_cache_test.go` | `TestManagedGoCacheFallbackAdoptedRootSymlink`, `TestManagedGoCacheFallbackAncestorSymlink`, `TestManagedGoCacheFallbackProviderError`, `TestManagedGoCacheFallbackInvalidOutput`, `TestManagedGoCacheFallbackWarning` |
| `.3`, `.4`, `.5`, `.7` | Same file | `TestManagedGoCacheFallbackPreservesEnvironment`, `TestManagedGoCacheDecisionResetsOnNewLaunch`, `TestManagedGoCacheDisabledAndAbsentProvider`, `TestManagedGoCacheSkipsNonHostExecutors` |
| `.4`, `.5` | `apps/backend/internal/agent/runtime/lifecycle/manager_managed_go_cache_launch_test.go` | `TestManagedGoCacheFallbackLaunch`, `TestManagedGoCacheRecoveryClearsStaleMetadata`, `TestManagedGoCachePromotionKeepsExecutionDecision`, `TestManagedGoCacheCoalescedJoinKeepsExecutionDecision` |
| `.6` | Same two lifecycle files | `TestManagedGoCacheCancellationPreventsLaunch` |
| `.4`, `.7` | Existing `manager_launch_test.go` and `process_runner*_test.go` | Retain successful propagation and process-environment behavior. Update helper assertions for final composition. |
| `.9` | `apps/backend/internal/system/storage/gocache/provider_launch_safety_test.go` | `TestExecutionEnvironmentRejectsUnsafeSymlinks`, `TestCleanupRejectsAdoptedRootSymlink`, `TestValidateAdoptionRejectsSymlinks` |
| `.9` | `apps/backend/internal/backendapp/storage_quarantine_controller.go`, `storage_maintenance_test.go` | Retain existing restore/delete checks. Add `TestManagedGoCacheQuarantineRejectsSymlinks` in `adapters_managed_go_cache_test.go`. |

IDs in this table use prefix `AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-006`.
New focused files avoid extending `manager_launch_test.go` (over 800 effective lines).
Keep each new file within the backend test-file limit.

## Backend recovery evidence (implemented)

Task 02 added end-to-end evidence through backend recovery dispatch in
`apps/backend/internal/backendapp/adapters_managed_go_cache_test.go`.
`TestManagedGoCacheRecoveryFlows` covers initial launch and real SQLite-backed
`Service.RecoverSession` dispatch for `fresh_start` and `resume` through the executor and lifecycle adapter.
It uses a real Go-cache provider and isolated temporary home, with fake external agentctl transport and
executor process creation. Assertions cover session progression, runtime creation, effective process
environment, token preservation/clearing under existing rules, cancellation, and a non-cache failure control.
The test uses real recovery dispatch, cache preparation, and lifecycle launch behavior.

## Work orders

| Order | Work order | Status | Dependency |
| --- | --- | --- | --- |
| 1 | [Task 01: Lifecycle fallback and execution state](task-01-lifecycle-fallback.md) | done | None |
| 2 | [Task 02: Recovery flows and maintenance safety](task-02-recovery-and-safety.md) | done | Task 01 |

## Exact verification

The work orders specified these verification commands; their actual outcomes are recorded below and in each work order:

```bash
(cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/system/storage/gocache -run 'ManagedGoCache|ExecutionEnvironment|CleanupRejects|ValidateAdoption' -count=1)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'ManagedGoCache' -count=1)
(cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/system/storage/gocache -count=1)
(cd apps/backend && go test ./internal/backendapp -run '^Test(ManagedGoCacheRecoveryFlows|ManagedGoCacheQuarantineRejectsSymlinks|QuarantineController.*GoCache)' -count=1)
(cd apps/backend && go test -race ./internal/backendapp -run '^TestManagedGoCacheRecoveryFlows$' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

The work orders assign these commands without scheduling unrelated suites.
The prospective production coverage preflight validates both work orders with the repository's coverage engine:

```bash
node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const root = 'docs/plans/managed-go-cache-launch-fallback/';
const orders = fs.readdirSync(root).filter(p => p.startsWith('task-')).map(p => root + p);
const paths = [root + 'plan.md', ...orders,
  'docs/specs/system-page/requirements/storage-maintenance.md',
  'docs/specs/system-page/system-design/managed-go-cache-launch-fallback.md'];
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({
  changedFiles: [...orders, 'apps/backend/internal/agent/runtime/lifecycle/manager_startup.go'],
  fileContents,
});
console.log(JSON.stringify({ ok: result.ok, status: result.status, errors: result.errors }));
if (!result.ok || result.status !== 'covered') process.exitCode = 1;
JS
```

This command checks prospective contract references and does not claim that production code changed during planning.
During implementation, the preflight was also run over the actual changed-file set with every referenced document loaded; it returned `ok: true`, `status: covered`.

## Verification results

Planning checks passed on 2026-10-01:

- `python3 scripts/list-docs.py validate`: 340 decisions and 1283 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check`: passed.
- Prospective production coverage preflight: `ok: true`, `status: covered`, both new work orders accepted.
- Actual documentation-only diff coverage preflight: `ok: true`, `status: exempt`.
- Catalog discovery includes the amended storage documents and new decision.

These are historical planning-stage results. At the 2026-10-01 design-package handoff, implementation tests and public-doc validators had not run, public content was unchanged, and both work orders were pending. Task 02 later added the public guidance and completed the implementation and validation recorded below.

Implementation and completion checks passed:

- Task 01's focused, race, and full lifecycle/provider test commands are recorded in its work order.
- Task 02's backend recovery/quarantine tests, backend recovery race test, and full lifecycle/provider suite passed.
- `make -C apps/backend build` passed.
- Specification catalog, specification linter tests (36), full specification lint, public-doc tests (62), public-doc validation (47 pages), and `git diff --check` passed.
- Actual changed-package coverage preflight returned `ok: true`, `status: covered`.

The managed-cache fallback is implemented. Storage maintenance keeps its persisted setting and strict safety checks. No host cache was repaired or changed during implementation.

Implementation is complete. Requirement 006 is Active, its lifecycle design and decision record describe the implemented behavior, and both work orders are done with results recorded. Public operations guidance now documents the fallback and retained maintenance safeguards.

### PR review follow-up verification (2026-10-02)

Review corrections add task/session correlation to bounded fallback warnings, assert provider-call counts for disabled/absent/remote cases, move requirement 006 to its own current lifecycle design, and stop all test agents during recovery-fixture cleanup.

Passed after these corrections:

- `go test ./internal/agent/runtime/lifecycle -run '^Test(ManagedGoCacheFallbackWarningIsBounded|ManagedGoCacheDisabledAbsentAndRemote)$' -count=1`
- `go test ./internal/agent/runtime/lifecycle ./internal/system/storage/gocache -run 'ManagedGoCache|ExecutionEnvironment|CleanupRejects|ValidateAdoption' -count=1`
- `go test -race ./internal/agent/runtime/lifecycle -run 'ManagedGoCache' -count=1`
- `go test -race ./internal/backendapp -count=1`, including the recovery fixture and backend goroutine-leak regressions
- The exact backend recovery/quarantine test selection and the focused race selection containing the recovery fixture and listener cleanup tests
- `make -C apps/backend build`
- `golangci-lint run ./... --new-from-rev=517249b5e609aef76e9ad06496b8f1a13cd00516 --timeout=5m` (0 issues)
- `python3 scripts/list-docs.py validate` (340 decisions and 1284 specifications), 36 specification-linter tests, full specification lint, 62 public-doc tests, and validation of 47 published pages
- Actual changed-package documentation coverage: `ok: true`, `status: covered`; `git diff --check`

One additional local run of `go test ./internal/agent/runtime/lifecycle ./internal/system/storage/gocache -count=1` reached the 10-minute package timeout in `TestOpenSSHRuntimeAPITunnel_ResumeRebindsPersistedRemotePort`; the Go-cache provider package passed in that run. The exact SSH test then passed 20 repeated race-enabled runs with `-timeout=90s`. This separate suite-order timeout did not reproduce in the focused run.

## Compatibility and risks

- Cache-only launch failures become warnings. Callers no longer receive the old fatal cache error.
- Valid managed-cache precedence and remote executor behavior remain unchanged.
- Ordinary inherited or independent `GOCACHE` can still name the same unusable path. Later tool failures remain possible.
- A fallback execution receives no new managed override if the host is repaired during its lifetime.
- Maintenance can still fail on unsafe paths, and skipped cache management can reduce performance or increase cache size.
- The provider's existing path checks do not create a new transactional filesystem guarantee. Existing replacement races remain outside this repair.
- The backend recovery fixture must use the real dispatch chain. Mock-only success is insufficient evidence.
- Cancellation races require barriers and positive controls. Sleeps cannot prove the launch boundary.

## Handoff

Implementation and validation are complete. The requirements, current design, decision, plan, and both completed work orders are committed on the feature branch and under review in [PR #4153](https://github.com/kdlbs/kandev/pull/4153). No host cache repair or saved-setting change occurred.
