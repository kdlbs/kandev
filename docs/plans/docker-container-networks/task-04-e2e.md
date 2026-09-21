---
id: "04-e2e"
title: "Container end-to-end coverage"
status: pending
wave: 4
depends_on: ["03-profile-editor"]
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-DOCKER-NETWORKS-001
  - REQ-EXECUTORS-DOCKER-NETWORKS-002
  - REQ-EXECUTORS-DOCKER-NETWORKS-003
acceptance_criteria:
  - AC-EXECUTORS-DOCKER-NETWORKS-001.1
  - AC-EXECUTORS-DOCKER-NETWORKS-001.7
  - AC-EXECUTORS-DOCKER-NETWORKS-002.2
  - AC-EXECUTORS-DOCKER-NETWORKS-003.4
system_design:
  - ../../specs/executors/system-design/docker-container-networks.md
---

# Task 04: Container End-to-End Coverage

## Summary

Prove against a real Docker daemon that a task container configured with a
named primary network and an additional attachment lands on both, starts, and
stays reachable through its published `agentctl` port.

## In scope

- A spec in the Playwright `containers` project, beside
  `apps/web/e2e/tests/docker/userns-sandbox.spec.ts`, that creates a
  user-defined bridge network for the run, configures a `local_docker` profile
  with it as the primary network plus a second bridge as an additional
  attachment, launches a task, and asserts the container is attached to both
  and that the agent session reaches ready.
- Persistence coverage in
  `apps/web/e2e/tests/settings/docker-profile-persistence.spec.ts` for the new
  keys: a configured profile round-trips them, and an untouched profile sends
  neither, mirroring the existing `allow_user_namespaces` assertions.
- Teardown that removes every network the spec created, including after a
  failed assertion, so a local run does not accumulate networks.

## Out of scope

- A `macvlan` attachment. It requires a parent interface and privileges no CI
  host is guaranteed to have; its behavior is covered by unit tests over the
  attachment call and by the driver-rejection table in task 01.
- Remote Docker end-to-end coverage, which needs a second host.
- Re-proving the validation table end to end; one rejection case is enough for
  the surfaced-error path.

## Acceptance

- The containers spec passes against a real daemon, asserting both attachments
  on the launched container and a ready agent session.
- The persistence spec asserts the configured and the untouched profile cases
  for all three network keys.
- Running the spec twice in a row leaves no network behind.

## Verification

```sh
cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project=containers --grep "network"
cd apps/web && pnpm e2e:run --grep "docker profile persistence"
cd apps/web && pnpm run lint:e2e-sleeps
```

## Likely files

- `apps/web/e2e/tests/docker/container-networks.spec.ts` (new)
- `apps/web/e2e/tests/settings/docker-profile-persistence.spec.ts`
- `apps/web/e2e/README.md` when the containers project's prerequisites change

## Dependencies and risks

Depends on task 03 for the editor surface the persistence spec drives.

The containers project needs a real Docker daemon and is gated on
`KANDEV_E2E_CONTAINERS=1`; follow `apps/web/e2e/README.md` and the repository's
one-worker-per-shard rule rather than passing worker overrides. Creating a
network is host state outside the test's own workspace, so name it per run and
remove it in teardown unconditionally.
