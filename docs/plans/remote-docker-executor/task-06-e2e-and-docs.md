---
id: "06-e2e-and-docs"
title: "E2E scenario and public documentation"
status: done
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

- Added an executor-scoped build path: `RemoteDockerBuilder` plus
  `POST /api/v1/remote-docker/executors/:id/build`, admin-gated because a build
  runs arbitrary Dockerfile instructions with the remote daemon's authority.
  The build refuses an executor with no trusted fingerprint.
- Threaded `remoteExecutorId` through the profile page's Docker sections so a
  remote profile builds on its own daemon. Building on the install-wide daemon
  would place the image where the task container never runs, which closes the
  gap recorded in task 05.
- The build stream owns its connection: closing the stream releases SSH, and a
  failed build releases it too rather than leaking.
- Replaced the Playwright-only plan with a Go integration test,
  `TestRemoteDockerTransportReachesARealDaemon`. It starts an sshd host with
  the Docker CLI and the machine's daemon socket mounted, then drives the real
  transport: dial, `docker system dial-stdio`, Engine API. Verified live
  against Docker 29.7.2, API 1.55. It also asserts connection reuse and a
  response-body request, which are what a wrong option order and a broken
  half-close would each break.
- Added `scripts/build-remote-docker-test-image.sh` to reproduce that host.
  Mounting the machine's own socket avoids nesting a second daemon, which the
  plan flagged as the least certain estimate.
- Two fixture facts the first attempt got wrong, both found by running it:
  Alpine needs `shadow` for `usermod` or sshd refuses public-key auth on the
  locked account, and the published port accepts TCP before sshd is ready, so
  the handshake itself has to be retried.
- Updated `docs/public/executors.md` and `docs/public/feature-status.md`:
  Remote Docker is supported, `tcp://` is excluded by construction, and the
  guidance states when to choose this over a single-node Kubernetes cluster.
- Verified with:
  - `KANDEV_TEST_REMOTE_DOCKER=1 go test ./internal/agent/runtime/lifecycle/ -run TestRemoteDockerTransportReachesARealDaemon`
  - `go test ./internal/agent/... ./internal/dockerremote/ -count=1` (clean)
  - `golangci-lint run ./internal/agent/... ./internal/dockerremote/... ./internal/backendapp/...` (0 issues)
  - `pnpm run typecheck`, `pnpm --filter @kandev/web lint` (clean)
  - `pnpm vitest run components/settings/profile-edit lib/api/domains` (71 files, 479 tests)
  - `scripts/list-docs.py validate`, `scripts/lint-spec-files.py --all`

### Screenshot

- `docs/screenshots/settings-executors.png` was stale: it predates the Remote
  Docker card, and nothing regenerates it. The PR skill's screenshot step
  captures assets for the PR body on an orphan branch, not this committed doc
  asset, so it would have stayed stale indefinitely.
- Added `e2e/tests/settings/executors-docs-screenshots.spec.ts`, following the
  existing `plugins-docs-screenshots.spec.ts` convention: skipped unless
  `CAPTURE_DOCS_MEDIA=1`, writes straight into `docs/screenshots/`. The
  recapture is now a command rather than a manual chore.
- Recaptured at 2x density to match the published asset, compressed with
  `pngquant` per the PR skill's recipe (239 KB to 75 KB), and corrected the alt
  text, which claimed an existing Sprites profile the capture does not show.

## Known gaps

- No Playwright spec in the `containers` project. The Go integration test
  covers the transport against a real daemon, which is the part that could not
  be proven with stubs; a browser-level scenario driving the settings form and
  a full task launch is still missing.
