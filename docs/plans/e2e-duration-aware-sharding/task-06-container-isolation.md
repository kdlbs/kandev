---
id: "06-container-isolation"
title: "Isolate container E2E state"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/e2e-duration-aware-sharding.md"
---

# Task 06: Isolate container E2E state

## Acceptance

- Storage-maintenance tests create and count only resources owned by the
  current test or worker, using unique labels/identifiers rather than a global
  `kandev.managed=true` count.
- Setup and teardown remove only resources created by that test and remain
  safe when another container E2E shard is running on the same daemon.
- The original first-attempt timeout path has a regression assertion or
  diagnostic that distinguishes resource leakage from genuine slow work.

## Verification

```sh
cd apps/web
KANDEV_E2E_CONTAINERS=1 pnpm exec playwright test --config e2e/playwright.config.ts --project=containers e2e/tests/docker/storage-maintenance.spec.ts
```

Run the targeted test with at least two container processes when the local
Docker daemon permits it, and verify that cleanup does not touch unrelated
Kandev-labelled resources.

## Files likely touched

- `apps/web/e2e/tests/docker/storage-maintenance.spec.ts`
- `apps/web/e2e/fixtures/docker-test-base.ts`
- `apps/web/e2e/fixtures/docker-probe.ts`
- related Docker/SSH fixture helpers only if ownership labels cross the shared
  boundary

## Dependencies

None for implementation. Task 08's performance experiment depends on this
task's scoped cleanup guarantee.

## Parallelism

Parallel candidate with Tasks 01, 04, and 05. Coordinate only if a shared
Docker fixture helper is also being changed by another repair.

## Inputs

- The global cleanup and count behavior in the Docker fixture.
- The storage-maintenance test's `removeKandevContainers()` call and comments.
- The existing container project timeout and worker lifecycle.

## Output contract

Report the ownership label/identifier scheme, cleanup scope, concurrent-shard
test evidence, targeted result, and any daemon prerequisites. Do not solve the
flake by raising the 180-second test timeout.

## Implementation result

Docker ownership now propagates from the E2E process into backend-created
containers and storage accounting. The storage-maintenance test passed once
and in two concurrent container processes; no managed containers remained
afterward. Backend lifecycle/storage tests passed in the focused Go packages.
Review remediation now runs process-scoped container removal after every API
reset. A serial regression proves the following test cannot observe a scoped
container left by its predecessor; all three storage-maintenance tests passed.
