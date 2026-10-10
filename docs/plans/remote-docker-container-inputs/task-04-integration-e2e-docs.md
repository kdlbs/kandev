---
id: "04-integration-e2e-docs"
title: "Integration, E2E, and documentation"
status: done
wave: 4
depends_on:
  - "03-teardown-session-dir"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-REMOTE-DOCKER-002
acceptance_criteria:
  - AC-EXECUTORS-REMOTE-DOCKER-002.1
  - AC-EXECUTORS-REMOTE-DOCKER-002.2
  - AC-EXECUTORS-REMOTE-DOCKER-002.9
system_design:
  - ../../specs/executors/system-design/remote-docker-container-inputs.md
---

# Task 04: Integration, E2E, and Documentation

## Summary

Prove the host footprint is actually gone against a real daemon and a real SSH
host, and tell users that Remote Docker no longer needs SFTP or a writable
remote home.

## In scope

- Extending `remote_docker_integration_test.go` so a daemon-backed launch
  asserts the helper is executable inside the container, the session directory
  holds the seeded credentials at the expected mode, and the remote home has no
  `.kandev` tree afterward.
- A `containers` Playwright assertion over the existing `kandev-sshd:e2e`
  fixture: across launch, stop, resume, and delete, no `~/.kandev` path is
  created on the fixture host, and no per-instance session directory survives.
- `docs/public/executors.md`: state that Remote Docker writes nothing to the
  remote host filesystem and needs only an SSH exec channel and TCP forwarding,
  no SFTP subsystem and no writable home, and that a container's removal removes
  its seeded credentials. The SSH executor's row and section, which do require
  SFTP, stay as they are.
- `docs/public/feature-status.md` if it carries a matching claim.

## Out of scope

- New E2E infrastructure. The scenario and the sshd fixture already exist.
- Backend behavior changes. Anything this task discovers belongs in a fix to
  task 02 or task 03.
- Screenshots. Nothing rendered changes.

## Acceptance

- The daemon-backed integration test fails if a `.kandev` tree appears on the
  remote account at any point in the launch/stop/delete cycle.
- The `containers` scenario passes with the same assertion over a real sshd.
- Public documentation no longer implies a remote-host footprint for this
  executor, and the SSH executor's documented SFTP requirement is unchanged.

## Verification

From `apps/backend`:

```bash
go test ./internal/agent/runtime/lifecycle/... -run 'RemoteDockerIntegration' -count=1
```

with the integration build tag and a reachable daemon; record the exact
invocation in Results. From `apps/web`:

```bash
KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project=containers --grep 'remote docker'
```

Documentation review runs through `/docs-maintainer`.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/remote_docker_integration_test.go`
- `apps/web/e2e/` — the existing remote Docker scenario in the `containers`
  project
- `docs/public/executors.md`
- `docs/public/feature-status.md`

## Dependencies

Tasks 01 through 03. This task verifies them; it does not implement behavior.

## Risks

- The integration and `containers` suites need a real Docker daemon and the
  sshd fixture. If either is unavailable in the working environment, say so in
  Results rather than reporting an unrun check as passing.
- Asserting the absence of a path is only as good as the account it inspects.
  Check the same remote account the executor connects as, not the fixture's
  root.

## Inputs

- `REQ-EXECUTORS-REMOTE-DOCKER-002`, acceptance criteria `.1`, `.2`, `.9`.
- `apps/web/e2e/README.md` for the `containers` project's gating and shard
  budget.
- `docs/public/executors.md` lines on the Remote Docker transport and on the
  SSH executor's SFTP requirement.

## Results

Every check in this work order ran against real infrastructure. Nothing here is
reported from a stub.

### Go integration coverage

`remote_docker_footprint_integration_test.go` adds two tests over the same sshd
fixture the transport test uses, a real SSH host with a real Docker daemon:

- `TestRemoteDockerLeavesNoHostFootprint` makes the remote account's home
  read-only, launches through the Engine API, and then inspects the account. It
  first proves the guard is live by asserting a write to that home fails, so the
  test cannot pass against a home that is still writable. The delivered helper
  runs, the seeded session directory is present in the container, and no
  `.kandev` tree exists on the host before or after teardown.
- `TestRemoteDockerTeardownRemovesAPreExistingSessionDir` seeds a pre-upgrade
  `~/.kandev/agent-sessions/<id>/creds.json` over SSH and proves a delete
  removes it.

`TestArchiveDeliveryIntoACreatedContainer` (work order 01) remains the proof
that a created-but-unstarted container accepts the archive.

### E2E coverage

`remote-docker-task.spec.ts` gains "writes nothing to the remote host
filesystem". The fixture mounts the SSH host's `$HOME` at a path the runner can
read, so the assertion inspects the real remote account after a launch, a
resume, and a delete. It asserts the home directory itself exists first, so a
wrong path cannot make the test pass by accident.

### Documentation

- `docs/public/executors.md`: Remote Docker writes nothing to the remote host;
  it needs only an SSH exec channel, TCP forwarding, and Docker socket access,
  with no SFTP subsystem and no writable home. Also states what an existing
  install may still hold, and that the shared `~/.kandev/bin/agentctl` cache is
  deliberately left alone because the SSH executor uses the same path. The SSH
  executor's own SFTP requirement is unchanged.
- `docs/public/feature-status.md`: the Remote Docker row no longer says mounts
  and the helper resolve on the remote host.
- The Local Docker credential section gained one sentence distinguishing the
  remote path, so a reader does not generalize the bind-mount description.

### Verified with

- `KANDEV_TEST_REMOTE_DOCKER=1 go test ./internal/agent/runtime/lifecycle/ -run 'TestRemoteDockerLeavesNoHostFootprint|TestRemoteDockerTeardownRemovesAPreExistingSessionDir' -count=1` — 2 PASS against Docker 29.7.2
- `KANDEV_TEST_REMOTE_DOCKER=1 go test ./internal/agent/runtime/lifecycle/ -run TestRemoteDockerTransportReachesARealDaemon -count=1` — PASS (API 1.55), unchanged
- `KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project=containers --grep 'remote docker executor'` — 3 passed (58.4s)
- `cd apps/web && pnpm run typecheck` — clean
- `pnpm --filter @kandev/web lint` — clean
- `python3 scripts/lint-spec-files.py --all` and `scripts/list-docs.py validate` — clean

### Pre-existing failures in the full backend suite

`go test ./...` reports 15 failures on this machine. None is caused by this
package; each was reproduced at the branch's merge base.

Fourteen are environment-dependent and fail identically at the base commit in a
plain run: the two `BASH_ENV` shim tests, `TestRunGit_DisablesCommitSigning`,
the git-operator push and preflight tests, `TestGetGitStatus_*`,
`TestProbeRealTree_*`, and `TestInstallSystemd/LaunchdWrites*`.

The fifteenth, `TestStartTaskWithEnv_OfficeCreateThenReusePublishesOneCreatedEvent`,
is a flaky race: `session *** state changed from STARTING to RUNNING before
runtime persistence`. It passes in isolation and passes the full
`internal/orchestrator` package standalone at both revisions, and it did not
appear in the base full-suite run, which made it look like a regression at
first. Running it with `-count=30` reproduces it at both revisions, so it is
pre-existing and load-sensitive, not caused by this work.
