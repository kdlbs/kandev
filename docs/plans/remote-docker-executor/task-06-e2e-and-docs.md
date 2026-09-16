---
id: "06-e2e-and-docs"
title: "E2E scenario and public documentation"
status: pending
wave: 4
depends_on:
  - "05-profile-create-and-test"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-REMOTE-DOCKER-001
acceptance_criteria:
  - AC-EXECUTORS-REMOTE-DOCKER-001.7
  - AC-EXECUTORS-REMOTE-DOCKER-001.11
  - AC-EXECUTORS-REMOTE-DOCKER-001.12
system_design:
  - ../../specs/executors/system-design/remote-docker-executor.md
---

# Task 06: E2E Scenario and Public Documentation

## Summary

Prove the executor end to end in the `containers` Playwright project and replace
the "Not implemented" documentation with the supported behavior.

## In scope

- A fixture exposing a Docker daemon reachable over the existing
  `kandev-sshd:e2e` container.
- An E2E scenario: create a profile with fingerprint trust, launch a task,
  confirm the agent responds, stop, resume into the same container, then delete
  and confirm remote teardown.
- Replacing the Remote Docker row in `docs/public/executors.md` with supported
  behavior, trust boundary, and cleanup rules, including when to choose this
  over single-node Kubernetes.
- Updating the "Workspace sources" note that currently says Remote Docker is
  unavailable.

## Out of scope

- Screenshot regeneration beyond what the executor hub change requires.
- Performance benchmarking of remote image builds.

## Acceptance

- The scenario passes in the `containers` project, gated on
  `KANDEV_E2E_CONTAINERS=1`.
- Resume reattaches to the same container ID rather than creating a second one.
- No documentation page still describes Remote Docker as not implemented.

## Verification

```bash
# From apps/web:
KANDEV_E2E_CONTAINERS=1 rtk pnpm run e2e --project=containers --grep "remote docker"
# From the repo root:
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `apps/web/e2e/tests/containers/remote-docker.spec.ts`
- `apps/web/e2e/fixtures/docker-test-base.ts`
- `docs/public/executors.md`
- `docs/public/feature-status.md`

## Dependencies

Task 05.

## Risks

- Running a Docker daemon reachable through the sshd fixture may require a
  privileged nested daemon or a second compose service. This is the least
  certain estimate in the plan; if nesting proves unstable, fall back to a
  dedicated fixture rather than weakening the scenario.
- The `containers` project is memory-budgeted with one worker per shard; adding
  a daemon raises footprint and may need shard rebalancing.

## Parallelism

`sequential`

## Inputs

- `REQ-EXECUTORS-REMOTE-DOCKER-001`.
- `apps/web/e2e/README.md` and the existing sshd fixture.
- `docs/public/executors.md`.

## Results

_Not started._
