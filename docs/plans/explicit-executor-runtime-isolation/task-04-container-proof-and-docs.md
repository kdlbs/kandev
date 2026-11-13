---
id: "04-container-proof-and-docs"
title: "Prove container boundaries and document behavior"
status: done
wave: 4
depends_on:
  - "03-runtime-identity-projection"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-RUNTIME-SELECTION-001
  - REQ-EXECUTORS-RUNTIME-SELECTION-002
  - REQ-EXECUTORS-RUNTIME-SELECTION-003
acceptance_criteria:
  - AC-EXECUTORS-RUNTIME-SELECTION-001.1
  - AC-EXECUTORS-RUNTIME-SELECTION-001.2
  - AC-EXECUTORS-RUNTIME-SELECTION-001.3
  - AC-EXECUTORS-RUNTIME-SELECTION-001.5
  - AC-EXECUTORS-RUNTIME-SELECTION-002.1
  - AC-EXECUTORS-RUNTIME-SELECTION-002.2
  - AC-EXECUTORS-RUNTIME-SELECTION-002.3
  - AC-EXECUTORS-RUNTIME-SELECTION-002.4
  - AC-EXECUTORS-RUNTIME-SELECTION-003.1
  - AC-EXECUTORS-RUNTIME-SELECTION-003.2
  - AC-EXECUTORS-RUNTIME-SELECTION-003.3
  - AC-EXECUTORS-RUNTIME-SELECTION-003.4
system_design:
  - ../../specs/executors/system-design/runtime-selection.md
---

# Task 04: Prove container boundaries and document behavior

## Summary

Prove user-visible refusal and real separate-container execution in isolated fixtures.
Update the existing public executor and Docker pages with the implemented contract.

## In scope

- Add desktop and phone browser coverage through existing task/session controls.
- Add real Docker scenarios in the guarded `containers` project, including a disposable containerized service fixture.
- Cover negative registration, daemon unavailability, failed create, healthy identity, and retained mismatch without host fallback.
- Record selected executor/profile, actual runtime, execution/container IDs, daemon resource evidence, and exact teardown.
- Update public reference/how-to text after behavior exists. Keep deployment limitations and operator permission boundaries visible.

## Out of scope

Changing a developer deployment, mounting its Docker socket, forwarding real credentials, fixture UI bypasses, new UI controls, or unrelated provider validation.
No container scenario may claim success from `/.dockerenv`, a hostname, or a mock-only runtime.

## Acceptance

1. Desktop and phone launches with disabled Docker show the existing failure flow, preserve selection and history, and create no host worker.
   Repository-free and repository-backed cases have the same boundary. Retry never changes the executor.
2. Real healthy Docker exposes a distinct daemon-verifiable runner. Disabled/unreachable/create-failure and retained-mismatch cases never start standalone or consume a prompt there.
   The mismatched exact worker remains observable and stoppable by an authorized actor.
3. Public docs describe required selection, unknown/observed runtime, retry limits, image defaults, and separate-container verification accurately.
   Recorded results distinguish passed, skipped, and unavailable infrastructure cases.

## Disposable containerized-backend recipe

This recipe is planned, not executed. Implement it as a focused Playwright fixture and test, not a permanent production deployment manifest.

1. Use a throwaway Linux/amd64 VM or host with an exclusively test-owned Docker daemon.
   Keep the daemon unreachable outside that disposable machine. Never reuse a developer's deployment daemon or data directory.
2. Let the fixture own a temporary daemon state root, socket, loopback API port, service container, volumes, and network resources.
   Run the backend container with host networking only for this test topology, keeping its PID/mount namespaces separate.
   The selected Docker backend can then reach the fixture's loopback daemon without mounting a daemon socket into the backend.
   Do not publish an unauthenticated daemon endpoint on a non-loopback interface.
3. Build the service image and Linux/amd64 helper from the implementation head.
   Export fixture helper and task/session directories under one temporary root.
   Mirror that root into the backend container at identical absolute daemon-host paths.
   Use the existing bridge-reachable HTTP Git fixture and synthetic agent profile data.
   Assert the daemon can read every intended bind source before the positive case.
4. Start the negative service with the stock image's explicit `KANDEV_DOCKER_ENABLED=false`.
   Create a Local Docker profile and bounded synthetic task, first without a repository, then with one.
   Assert typed refusal, no new `executors_running` worker, no daemon runner, and no new authenticated mock-agent process inside the service.
   Capture pre/post instance and process inventory by exact task/session IDs, without dumping environment values.
5. Start a separate service using the same disposable topology with Docker explicitly enabled and its test-only daemon address configured.
   Use a bounded mock-agent canary and no real provider credentials.
   Run both repository-free and repository-backed success cases.
   Assert requested Local Docker, session `execution_runtime=docker`, matching WS runtime, and a nonempty full container ID.
6. Inspect backend and worker by their full IDs in the daemon inventory.
   Require different IDs and correlate worker task/session labels, running process membership, and PID/mount namespace identities.
   Resolve process membership from daemon inspection and host `/proc`, rather than in-container hostname or filesystem markers.
   Confirm the bounded mock-agent child belongs to the worker and not the backend service container.
7. Exercise an unreachable endpoint and a deliberately rejected container create in separate fixture service instances.
   Use fixture-owned invalid inputs and no deployment changes. Require a safe error and zero standalone worker effects.
8. Seed a synthetic retained mismatch using disposable database/fixture APIs, then restart and attempt ensure, promotion, and resume.
   Do not create the fixture by exploiting the old fallback or by editing a real journal.
   Assert unchanged source/history/inventory, blocked prompts, exact authorized Stop, and an unaffected healthy sibling.
9. In `finally`, stop bounded canaries, remove exact test-labelled worker/backend resources, stop the owned daemon, and join all processes.
   Remove only fixture-owned roots after inventory confirms resource absence.
   Retain a sanitized test report containing identities and assertions, not tokens or raw environment maps.

The outer managed runner uses `--host`; the spec itself creates the containerized service and isolated daemon.
Do not assume `e2e:run --docker` gives a working daemon or correct bind-source mirroring.
If the host cannot provide the owned daemon topology, report the missing capability and leave this acceptance pending.
Skipped cases are not isolation evidence.

## TDD and verification

Read `/e2e`, `e2e/README.md`, the resource-safety reference, and existing Docker fixtures before implementation.
Use the managed runner. It rebuilds production assets and enforces the shard budget.
Arm HTTP/WS waits before actions. Use causal waits rather than arbitrary sleeps.
Keep one worker per shard and run the commands sequentially.

```bash
(cd apps && pnpm install --frozen-lockfile)
# On a non-Linux host, provide a linux/amd64 backend binary first:
# make -C apps/backend build-kandev-linux
# export KANDEV_E2E_BACKEND_BIN=$PWD/apps/backend/bin/kandev-linux-amd64
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/session/executor-runtime-failure.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-executor-runtime-failure.spec.ts)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --shards 1 --project containers tests/docker/runtime-boundary.spec.ts tests/docker/docker-launch.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/decisions docs/plans/explicit-executor-runtime-isolation docs/public
```

All commands start at the repository root. Confirm the runner discovers the intended specs before claiming coverage.
Run the repository PR-documentation coverage evaluator against the actual changed work orders and their referenced artifacts.
The plan's planning-validation entry explains the local evaluator invocation.
If browser-authored copy becomes necessary, apply all seven locale catalogs and add `pnpm run i18n:check` and `pnpm run i18n:ratchet`.
No authored UI copy or layout change is currently planned.

## Files likely touched

- New `apps/web/e2e/tests/docker/runtime-boundary.spec.ts`.
- New `apps/web/e2e/fixtures/containerized-backend.ts` with exact owned teardown.
- `apps/web/e2e/fixtures/docker-test-base.ts`, `docker-probe.ts`, and `helpers/docker.ts`, only for reusable fixture support.
- New `apps/web/e2e/tests/session/executor-runtime-failure.spec.ts` and `mobile-executor-runtime-failure.spec.ts`.
- `docs/public/executors.md`, `docs/public/docker.md`.
- This plan and work-order Results after the implementation checks run.

## Dependencies

Tasks 01 through 03. The new APIs and runtime guard must exist before these tests can pass.

## Risks

Containerized Local Docker requires identical daemon-host bind paths and a compatible helper architecture.
The existing host-backend fixture alone cannot prove service/worker separation.
Do not introduce an isolation bypass or modify operator configuration to make the test pass.
Mobile uses the existing failure flow. If rendered behavior must change, update the design and previews before broadening this work order.

## Parallelism

`sequential`

## Inputs

- Runtime-selection design and plan E2E matrix.
- `apps/web/e2e/tests/docker/docker-launch.spec.ts` and Docker fixtures.
- Public Docker image behavior and containerized-control-plane limitation sections.
- Mobile parity: data-only type changes have no new composition. Phone browser coverage proves existing failure/retry parity.

## Results

Implemented and verified.

- `docs/public/executors.md`: required-runtime section covering refusal, retry, retained mismatch,
  and `execution_runtime`.
- `docs/public/docker.md`: the service image's disabled-Docker default, refusal instead of host
  fallback, separate worker container, and observed runtime/container ID.
- `apps/web/e2e/tests/session/executor-runtime-failure.spec.ts` and
  `mobile-executor-runtime-failure.spec.ts`: disabled Local Docker refuses through the normal task
  launch, creates no session or environment, preserves the saved profile, and renders the existing
  failure surface without phone overflow.
- `apps/web/e2e/tests/docker/runtime-boundary.spec.ts`: a healthy Local Docker launch records
  `execution_runtime=docker`, the session/environment/container identity agree, the worker
  container exists on the daemon with task/session labels, the bounded mock agent runs inside it,
  and the worker PID namespace differs from the backend process namespace. A registered Docker
  backend pointed at a closed daemon port fails the launch with no container, no standalone
  session, and no observed runtime.
- `apps/web/e2e/fixtures/containerized-backend.ts` and
  `apps/web/e2e/tests/docker/containerized-runtime-boundary.spec.ts`: a disposable privileged
  Docker-in-Docker container owns its own daemon and network namespace. Inside it, a Kandev
  backend container and its worker containers are distinct resources. The test asserts distinct
  full daemon IDs, task/session labels, the mock agent inside the worker, and separate PID
  namespaces, then seeds a retained standalone worker, selects Local Docker on the same binding,
  proves the refusal leaves inventory and history unchanged, and verifies exact authorized Stop
  retires that execution. The host daemon only starts the disposable dind container; the host's
  `docker0` bridge and existing deployment are untouched.
- `apps/web/e2e/tests/docker/docker-launch.spec.ts`: the existing healthy launch asserts the
  additive session runtime projection and matching container ID.
- `apps/web/e2e/helpers/docker.ts`: label and process-top helpers for the boundary assertions.

Verification (all passed):

```bash
(cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project chromium tests/session/executor-runtime-failure.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/session/mobile-executor-runtime-failure.spec.ts)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --no-build --shards 1 --project containers tests/docker/runtime-boundary.spec.ts)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --no-build --shards 1 --project containers tests/docker/containerized-runtime-boundary.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

No live deployment is used: the fixture starts a disposable dind container, runs the backend and
workers on the daemon inside it, and removes that container and its temporary root. It never
creates, reuses, or removes the host's `docker0` bridge.
Regression coverage also includes prepared-launch retained-runtime ordering, cold
session/environment/launch retained checks, inventory read failures and cancellation, unknown
remote runtime evidence, and profile-parent resolution failures in work orders 01 and 02.
