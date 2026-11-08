---
id: "03-npm-and-executor-proof"
title: "Prove real npm and executor reuse"
status: done
wave: 3
depends_on:
  - "02-launch-and-probe-reuse"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-006
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-007
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.1
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.5
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.6
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.8
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.9
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.10
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.11
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.13
system_design:
  - ../../specs/agents/system-design/managed-npm-repair-reuse.md
---

# Task 03: Prove real npm and executor reuse

## Summary

Prove the repair with real npm and protocol processes across repeated cold starts.
Prove execution-host storage on Docker and SSH with the existing containers fixtures.

## In scope

- Loopback registry and local archives using the existing real-npm fixture.
- Missing-root failure, successful replacement, owner teardown, fresh owner, and first-attempt reuse.
- ACP, managed native Codex, selected-package families, and actual physical runtime observations.
- Independent agentctl process restart, context mismatch, invalid records, and old-consumer reads.
- Docker/SSH repeated starts with persistent execution-host storage and shared sentinels.

## Out of scope

- Public registry downloads, paid model requests, new executors, and browser presentation assertions.

## Acceptance

1. `TestManagedRuntimeRepairReuseNPM` first reproduces npm's missing-root failure.
   After recovery and complete owner teardown, a compatible launch initializes once.
   Installer count and loopback registry requests do not increase on that reuse.
2. `TestManagedRuntimeRepairReuseRestart` and lifetime cases prove a fresh agentctl
   owner reuses the selected generation. Context mismatch prevents reuse, and an
   old process can still read its files after successor publication.
3. Docker/SSH scenarios prove reuse occurs in the execution host's store.
   A new container/storage lifetime is a miss. Backend and shared npm sentinels remain unchanged.

## Verification

Run sequentially from the repository root. Missing npm with the real-npm selector is a failed prerequisite.
Missing Docker/SSH infrastructure remains missing acceptance evidence.

```bash
(cd apps/backend && KANDEV_TEST_REAL_NPM=1 go test -trimpath -tags fts5 ./internal/agent/managedruntime ./internal/agentctl/server/process ./internal/agentctl/server/utility -run 'TestManagedRuntimeRepairReuseNPM|TestManagedRuntimeRepairReuseRestart|TestManagedRuntimeRepairReuseLifetime' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/managedruntime ./internal/agentctl/server/process ./internal/agentctl/server/utility -run 'RepairStoreConcurrency|ManagedRuntimeRepairReuse' -count=1)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --shards 1 --project containers tests/docker/managed-runtime-npm-recovery.spec.ts tests/ssh/managed-runtime-npm-recovery.spec.ts --grep 'reusable verified cache repair')
```

For a fresh worktree, install workspace dependencies from `apps/` before the pnpm command.
The managed E2E runner builds current artifacts and owns instance teardown.

## Files likely touched

- New `repair_reuse_npm_test.go` under `apps/backend/internal/agent/managedruntime/` and matching process/utility integration tests.
- Existing real-npm registry/archive fixture helpers from `prepared_runtime_test.go`.
- `apps/web/e2e/helpers/managed-runtime-recovery.ts` and `managed-runtime-test-registry.ts`.
- `apps/web/e2e/tests/docker/managed-runtime-npm-recovery.spec.ts`.
- `apps/web/e2e/tests/ssh/managed-runtime-npm-recovery.spec.ts`.
- Existing owned npx fixture scripts only where generation selection needs observation.

## Dependencies

Task 02. Keep this task's container assertions independent of the visible UI assertions in Task 04.

## Risks

- Restart proof can accidentally adopt a live process instead of starting a new one.
- Backend and executor roots can appear identical in a fixture without proving physical isolation.
- A mocked ETARGET does not reproduce the incomplete-root defect.

## Parallelism

`sequential`

## Inputs

- Design: Verification; File lifetime; Compatibility.
- Existing cache-recovery real-npm and Docker/SSH scenarios.
- Repository E2E runner and single-worker rules.

## Results

Verified on 2026-11-08.

- `KANDEV_TEST_REAL_NPM=1 go test -trimpath -tags fts5
  ./internal/agent/managedruntime -run TestManagedRuntimeRepairReuseNPM
  -count=1` passed locally with npm 11 and Node 24.
- `KANDEV_TEST_REAL_NPM=1 go test -trimpath -tags fts5
  ./internal/agent/managedruntime ./internal/agentctl/server/process
  ./internal/agentctl/server/utility -run
  'TestManagedRuntimeRepairReuseNPM|TestManagedRuntimeRepairReuseRestart|TestManagedRuntimeRepairReuseLifetime'
  -count=1` passed.
- The test reproduces the real missing shared root-manifest failure against a
  loopback registry, prepares one generation, completes ACP initialization,
  publishes it, stops every owner, reopens the store, and then initializes the
  same generation once more without another installer or registry request.
- Shared-cache sentinels and the intentionally incomplete shared tree remain
  unchanged.
- Process-manager reuse, restart selection, context mismatch, publication
  after initialization, and retirement continuity are covered by focused Go
  tests in `internal/agentctl/server/process`.
- Utility selection and changed-configuration isolation are covered in
  `internal/agentctl/server/utility`.
- `go test -trimpath -tags fts5 -race ./internal/agent/managedruntime` passed
  for the full package.
- Same-storage executor reuse passed in the containers project. The Docker and
  SSH specs kill only the owned mock-agent process, keep the container,
  agentctl storage, and shared cache intact, resume the same session, and
  assert:
  - the published generation remains in the execution host's store,
  - the registry request count does not increase,
  - the resumed agent process starts in the same container or remote host,
  - direct input remains usable.
- The Docker spec also starts a second task in a new container and asserts the
  registry request count increases, proving a new storage lifetime is a miss.
- `KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --shards 1 --project containers
  tests/docker/managed-runtime-npm-recovery.spec.ts
  tests/ssh/managed-runtime-npm-recovery.spec.ts
  --grep 'reusable verified cache repair'` passed both reuse cases.
- The full container spec run passed 6 tests, including the existing
  private-preparation and online-retry controls.
- Utility/store reuse is separately proven on the execution host by the
  desktop and mobile profile discovery scenarios in Task 04.
- On Windows, reusable repair remains disabled until a native ACL and
  ownership verification is implemented; recovery continues through the
  existing private path. A native Windows lock/replace run remains in the
  Windows verification lane, and this package does not claim cross-platform
  completion without it.
