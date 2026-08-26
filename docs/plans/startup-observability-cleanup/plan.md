---
created: 2026-08-26
status: in_progress
requirements:
  - REQ-PLATFORM-GO-DEV-LAUNCHER-001
  - REQ-PLATFORM-AGENT-PROCESS-EXIT-DRAIN-001
  - REQ-PLATFORM-AGENTCTL-INSTANCE-STOP-001
system_design:
  - ../../specs/platform/system-design/go-dev-launcher.md
  - ../../specs/platform/system-design/agent-process-exit-drain.md
  - ../../specs/platform/system-design/agentctl-instance-stop.md
legacy_specs: []
---

# Implementation Plan: Startup observability and lifecycle cleanup

## Overview

The 2026-08-26 log audit found two confirmed implementation defects and one
requested observability gap. First, `waitForExit` emits a false stderr-drain
warning because it waits for EOF before `cmd.Wait`. Second, duplicate instance
stops return HTTP 500 after another stop has already completed. Third, the
native launcher does not print the build version in its startup banner. The
work orders are sequential because each changes a lifecycle boundary and needs
focused regression evidence before the next change is reviewed.

## Scope

### In scope

- Print the native build version in `dev`, `start`, and `run` startup banners.
- Make stderr-drain timeout warnings post-exit diagnostics only.
- Make overlapping stops for the same agentctl instance converge on the stopped
  state without masking genuine teardown failures.
- Update the public CLI reference for the new startup line.

### Out of scope

- Cursor ACP authentication failure. This requires the operator to log in or
  remove the unready Cursor agent configuration.
- SSE MCP filtering. This is expected capability negotiation for ACP agents
  that do not support SSE.
- Shell stop grace periods, process-group force-kill policy, Git polling policy,
  and log rotation.
- Broad reclassification of unrelated agentctl stderr or startup warnings.

## Technical approach

### Startup version identity

Pass `launcher.BuildInfo.Version` from `launcher.Run` through the mode-specific
launch configuration into the shared `logStartup` function. Render one
`[kandev] version:` line before the existing URL lines, using the native `dev`
default when metadata is empty. Keep `KANDEV_VERSION` as optional installed
release header metadata. Update `docs/public/cli.md` in the same implementation
change.

### Agent stderr exit sequencing

Keep `readStderr` running concurrently with the child process. Replace the
manager's `Cmd.StderrPipe` with an explicitly owned `os.Pipe`, then change
`internal/agentctl/server/process.Manager.waitForExit` to call `cmd.Wait()`
before the bounded wait on the generation-specific stderr completion channel.
Close the owned reader when the bounded drain expires. Preserve recent-stderr
sanitization, exit events, intentional-stop behavior, and process-group
reaping.

### Idempotent instance stop

Keep the initial unknown-ID 404 and the existing per-instance `stopMu`. After a
stop caller acquires the lock, distinguish removal of the same instance pointer
after successful teardown from an ID remapped to a different instance. Treat
the former as an already-satisfied stop, release the port only once, and retain
HTTP 500 plus the instance for genuine cleanup failures.

## Tests

- `AC-PLATFORM-GO-DEV-LAUNCHER-001.9`: add launcher banner coverage in
  `apps/backend/internal/launcher/network_test.go` or a focused startup test,
  including stamped, empty, and `--version`-equivalent values.
- `AC-PLATFORM-AGENT-PROCESS-EXIT-DRAIN-001.1` through `.4`: extend
  `apps/backend/internal/agentctl/server/process/manager_stderr_exit_test.go`
  to prove no pre-exit timeout warning, post-exit bounded drain, preserved
  recent stderr, and unchanged intentional/error exit paths.
- `AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.1` and `.4`: add deterministic
  concurrent-stop coverage in
  `apps/backend/internal/agentctl/server/instance/manager_shutdown_test.go`.
- `AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.2` and `.3`: retain or extend
  control-server boundary coverage for unknown IDs and cleanup failures.

## E2E tests

The new version contract is native launcher stdout, not browser behavior, so a
Playwright project cannot observe it. The package uses an end-to-end launcher
boundary test in `apps/backend/internal/launcher` and the existing backend
process tests for the two internal lifecycle contracts. No browser E2E test is
needed for these acceptance criteria.

## Work orders

- [x] [Task 01: Startup banner version identity](task-01-startup-version.md) (done)
- [x] [Task 02: Agent stderr exit sequencing](task-02-agent-stderr-exit.md) (done)
- [x] [Task 03: Idempotent agent instance stop](task-03-instance-stop.md) (done)

## Verification results

- Task 01 focused launcher tests passed.
- Public documentation validation passed (`node --test scripts/validate-public-docs.test.mjs`; `node scripts/validate-public-docs.mjs`).
- The full launcher package ran 259 tests successfully, with two pre-existing
  service configuration tests selecting the host's `/root/.kandev/config.yaml`
  instead of their temporary fixture.
- The full process-manager package passed 707 tests after the stderr sequencing
  fix and owned-pipe correction.
- The combined instance and control-server packages passed 463 tests after the
  idempotent-stop fix.

## Risks

- The owned stderr writer must close in the parent after startup, while the
  reader remains concurrent so a child that writes a full pipe cannot deadlock.
- Treating a missing map entry as already stopped must be tied to the original
  instance pointer so an ID reuse cannot be mistaken for completed cleanup.
- The startup banner is consumed by people and scripts, so existing line order
  and values must remain stable apart from the additive version line.
