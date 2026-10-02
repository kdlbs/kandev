---
id: "03-background-cleanup-recovery"
title: "Recover cleanup in the owned worker"
status: done
wave: 3
depends_on:
  - "02-cancel-source-capture"
plan: "plan.md"
requirements:
  - REQ-TASKS-ARCHIVE-SOURCE-MANIFEST-001
  - REQ-TASKS-RUNTIME-CLEANUP-001
acceptance_criteria:
  - AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.1
  - AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.5
  - AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.7
  - AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.8
  - AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.9
  - AC-TASKS-RUNTIME-CLEANUP-001.32
system_design:
  - ../../specs/tasks/system-design/archive-source-manifest.md
---

# Task 03: Recover cleanup in the owned worker

## Summary

Register the cleanup worker without waiting for due recovery jobs.
Run recovery immediately in that worker, then cancel and join it on shutdown.

## In scope

- Use TDD. `TestStartTaskResourceCleanupWorkerReturnsBeforeRecovery` uses the
  existing blocked-repository fixture to prove Start returns while resume is blocked.
- Move initial resume into the accounted worker goroutine before its first wait.
  Preserve the original prepared cutoff and full resume retries after failures.
- Add a lifecycle mutex in `Service` distinct from `cleanupWorkerMu`.
  Serialize start/stop registration and drainage. Never hold the wake-state
  mutex while joining a worker that can produce a wake.
- Add channel-driven tests for stop during recovery, repeated start/stop,
  cancellation during capture, immediate recovery, and start during stop drainage.
- Update `TestStopTaskResourceCleanupWorkerJoinsStartupResume` so Start succeeds
  before Stop, while Stop still joins the owned recovery operation.
- Update startup failure tests in `resource_cleanup_activation_test.go` to assert
  asynchronous warning/retry instead of a synchronous recovery error.
  Preserve `TestWorkerResumeRetryPreservesPreparedCleanupCreatedAfterStartup`.
- Add `TestCleanupIgnoredDependenciesLifecycle` with archive and delete cases.
  Use real Git registration and installed ignored dependency/build fixtures.
  Wait for cleanup completion and assert persisted omission evidence before
  physical worktree removal. Retained evidence remains retrievable after deletion.
- Add `TestStopTaskResourceCleanupWorkerCancelsManifestCapture` using controlled
  reader/collector cancellation, without machine-speed performance assertions.
- Preserve cascade retry after attempt eight, generation fencing, persisted
  manifest reuse, and the explicit synchronous resume method.
- Update package Results and public-doc impact after all targeted checks pass.

## Out of scope

Mandatory archive-cascade gate redesign, health UI changes, retry terminalization,
new settings, migrations, or desktop/frontend layout changes.

## Acceptance

- Start returns before due cleanup drains. Recovery begins immediately and
  retries failures with the original cutoff and observable warnings.
- Stop cancels and joins the owned worker. No successor worker starts during
  drainage, and repeated start/stop remains safe under the race detector.
- Real archive/delete cleanup with ignored dependencies succeeds, persists
  evidence before removal, and preserves readable, workspace-authorized evidence.

## Verification

```bash
(cd apps/backend && go test -race -count=1 -run 'Test.*(TaskResourceCleanup|Cleanup|ArchiveManifest|ArchiveTaskCleanup|TaskLifecycleCleanup|StartupActivation|WorkerResume|WorkerRetries|PeriodicReconciliation)' ./internal/task/service ./internal/worktree)
(cd apps/backend && go test -count=1 ./internal/system/storage)
(cd apps/backend && go test -count=1 -run '^TestHTTPGetArchiveSourceManifestDeniesForeignWorkspace$' ./internal/task/handlers)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/archive-manifest-bounded-cleanup
node <<'JS'
const fs = require('fs');
const cp = require('child_process');
const validator = require('./.github/scripts/pr-docs.cjs');
const packageDir = 'docs/plans/archive-manifest-bounded-cleanup';
const docs = fs.readdirSync(packageDir).filter(name => name.endsWith('.md'))
  .map(name => `${packageDir}/${name}`);
docs.push('docs/specs/tasks/requirements/archive-source-manifest.md',
  'docs/specs/tasks/requirements/runtime-cleanup.md',
  'docs/specs/tasks/system-design/archive-source-manifest.md');
const paths = [...new Set([
  ...cp.execFileSync('git', ['diff', 'HEAD', '--name-only', '-z']).toString().split('\0'),
  ...cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z']).toString().split('\0'),
].filter(Boolean))];
const result = validator.validateCoverage({
  changedFiles: paths.map(filename => ({filename, status: 'modified'})),
  fileContents: Object.fromEntries(docs.map(filename =>
    [filename, fs.readFileSync(filename, 'utf8')])),
});
console.log(JSON.stringify({ok: result.ok, status: result.status, errors: result.errors}));
if (!result.ok) process.exitCode = 1;
JS
```

No broad verification suite or browser instance is required.

## Files likely touched

- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- `apps/backend/internal/task/service/service.go`
- `apps/backend/internal/task/service/resource_cleanup_jobs_test.go`
- `apps/backend/internal/task/service/resource_cleanup_activation_test.go`
- `apps/backend/internal/task/service/resource_cleanup_source_manifest_boundary_test.go`
- `apps/backend/internal/task/service/resource_cleanup_archive_identity_test.go`
- `docs/plans/archive-manifest-bounded-cleanup/plan.md`
- `docs/plans/archive-manifest-bounded-cleanup/task-03-background-cleanup-recovery.md`

## Dependencies

Task 02 makes manifest capture responsive to worker cancellation.

## Risks

Existing Start error assertions encode synchronous recovery. Replace their
assumptions while preserving durable recovery and prepared-job cutoff coverage.
WaitGroup registration must not race with Stop or successor worker start.

## Parallelism

`sequential`

## Inputs

- [Manifest design: Background recovery](../../specs/tasks/system-design/archive-source-manifest.md#background-recovery).
- [Runtime startup registry](../../specs/tasks/system-design/runtime-startup-registry.md).
- Existing blocking resume, cancellable cleanup, activation retry, cutoff,
  cascade retry, and source-boundary fixtures.
- The plan's startup-blocking RED reproduction.

## Results

Moved due-job recovery into the registered cleanup worker so startup returns
while recovery runs. The worker retries complete resume failures with the
original prepared-job cutoff. A lifecycle mutex serializes registration and
drainage, while the wake-state mutex is released before Stop joins the worker.
Updated recovery tests for asynchronous errors, added channel-driven stop,
start, and repeated lifecycle coverage, and added archive/delete integration
cases that verify ignored dependency and build-directory omission evidence is
persisted before worktree removal and remains retrievable afterward.

Passed:

- `(cd apps/backend && go test -race -count=1 -run 'Test.*(TaskResourceCleanup|Cleanup|ArchiveManifest|ArchiveTaskCleanup|TaskLifecycleCleanup|StartupActivation|WorkerResume|WorkerRetries|PeriodicReconciliation)' ./internal/task/service ./internal/worktree)`
- `(cd apps/backend && go test -count=1 ./internal/system/storage)`
- `(cd apps/backend && go test -count=1 -run '^TestHTTPGetArchiveSourceManifestDeniesForeignWorkspace$' ./internal/task/handlers)`
- `(cd apps/backend && go build -tags fts5 -o /tmp/kandev-issue4130-check ./cmd/kandev)`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`
- `.github/scripts/pr-docs.cjs` coverage validation: `ok: true`, `status: covered`.

Public docs need no change because the existing cleanup guidance already covers
the user-visible asynchronous behavior; the new evidence field is internal.
