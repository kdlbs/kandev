---
id: "02-remote-launch-and-terminal"
title: "Launch and attach the SSH terminal"
status: done
wave: 2
depends_on:
  - "01-remote-controller"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-PASSTHROUGH-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.1
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.2
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.3
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.4
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.5
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.6
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.12
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.14
system_design:
  - ../../specs/executors/system-design/ssh-passthrough.md
---

# Task 02: Launch and attach the SSH terminal

## Summary

Route fresh SSH passthrough through the remote owner from Task 01. Prepare MCP
configuration beside the CLI, then connect the existing browser terminal to
that remote process. Preserve local runner behavior.

## In scope

- A lifecycle transport seam selected by executor identity. SSH never chooses
  the standalone runner, including on a capability or transport failure.
- Remote snapshot paths and MCP URL. Preserve the agent-owned emission strategies.
- Extract shared materialization into `internal/common/passthroughconfig`.
  Keep locking, merging, fingerprints, user-file preservation, and symlink rules.
  Add remote cross-controller claims under the private task root.
- Remote `prepare` handler with path and ownership validation. Local callers use
  the extracted implementation with their existing semantics.
- Fresh launch, initial idle/prompt injection, remote environment, callbacks,
  and a working MCP stream. Programmatic writes use submission identities.
- Gateway authorization before runtime lookup, upstream authenticated streams,
  and cancellation that closes both sockets and joins both pumps.
- Safe capability and startup errors through the existing recovery projection.
  Add localized detail keys only if existing typed causes cannot explain the failure.

## Out of scope

Abnormal recovery, reset/model replacement, and persistence reconciliation are
completed in Task 03. No terminal layout or style changes are planned.

## Acceptance

1. RED: `TestSSHPassthroughBootstrapUsesRemoteMCP` enters real command preparation
   with SSH-shaped metadata and no standalone port. RED:
   `TestSSHPassthroughNeverStartsHostCLI` enters the real startup path with a
   remote-only workspace and with a matching host directory. GREEN proves
   remote launch in both cases and no host CLI side effects.
2. MCP strategy fixtures for Claude, Codex, OpenCode, and project-file emission
   use the remote instance port and target paths. `TestRemotePassthroughConfigOwnership`
   and `TestRemotePassthroughConfigSymlinks` prove user edits, concurrent session
   claims, and failed reads preserve files. Remote startup preserves the selected
   environment and completes one initial prompt through the existing boundary.
3. `TestRemotePassthroughTerminalBridge` proves remote bytes, resize, accepted
   submission, and disconnect/reconnect through the unchanged browser protocol.
   `TestRemotePassthroughOldHelper` and `TestRemotePassthroughSafeStartupFailure`
   prove safe details and absence of any local or ACP fallback.

## Verification

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/common/passthroughconfig ./internal/agent/mcpconfig ./internal/agentctl/server/api ./internal/agent/runtime/lifecycle ./internal/gateway/websocket -run 'Test.*Passthrough|TestMaterialize.*|Test.*MCP.*Ownership|Test.*MCP.*Symlink' -count=1)
git diff --check
```

Run changed-file Go lint as in Task 01. If product copy changes, install web
dependencies once, then run these additional gates:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
```

## Files likely touched

- `apps/backend/internal/common/passthroughconfig/` (new shared implementation
  and ownership/path tests).
- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough.go`,
  `passthrough_mcp_ownership.go`, `passthrough_mcp_file_lock*.go`, and existing
  MCP file/ownership tests.
- `apps/backend/internal/agent/runtime/lifecycle/passthrough_transport.go` and
  `manager_ssh_passthrough_test.go` (new).
- `apps/backend/internal/agent/runtime/lifecycle/manager_startup.go`,
  `manager_execution.go`, `streams.go`, and related event helpers.
- `apps/backend/internal/agentctl/server/api/passthrough_prepare.go` and tests
  (new), plus Task 01's DTO/client files for preparation.
- `apps/backend/internal/gateway/websocket/terminal_handler.go`,
  `terminal_pumps.go`, `terminal_sessions.go`, and
  `terminal_remote_passthrough_test.go` (new).
- `apps/backend/internal/agent/runtime/lifecycle/bootstrap_failure.go` and the
  existing browser error-code mapping only if needed for safe typed details.

## Dependencies

Task 01's authenticated controller/client contract and ownership tests.

## Risks

A remote MCP URL must use the instance port, not its backend forward. Concurrent
controllers share project files, so in-memory claims alone cannot protect cleanup.
Keep lifecycle callback identity immutable even when a workspace-only execution
is promoted or a remote process exits before registration.

## Parallelism

`sequential`

## Inputs

- [Remote preparation design](../../specs/executors/system-design/ssh-passthrough.md#remote-command-and-mcp-preparation).
- [Transport selection](../../specs/executors/system-design/ssh-passthrough.md#lifecycle-transport-selection).
- Existing `manager_passthrough_mcpfiles_test.go`, `manager_passthrough_scope_test.go`,
  `manager_passthrough_launch_test.go`, and gateway PTY tests.
- Existing task initial-prompt criteria and ADR linked by the requirement.

## Results

The remote launch, MCP preparation, environment, and terminal routing are implemented.
A real PTY in a distinct workspace proves that the host does not run the remote
CLI, even when a matching host directory exists. Remote MCP strategy files use
the target path and loopback port. Ownership tests preserve sibling claims,
user files, and symlink boundaries. The authenticated gateway bridge and remote
session stream preserve the PTY across browser disconnect and replacement.

The planned race commands passed for the controller API, process, instance,
client, lifecycle, gateway, and shared config packages. The full `mcpconfig`
package also passed with `-race`. Changed Go lint passed with zero issues:
`GOCACHE=/tmp/kandev-ssh-go-cache.7r9WV7 xargs bash scripts/lint-go-changed < /tmp/ssh-go-changed`.
The list contains every existing changed or untracked Go source path.

Real SSH browser tests pass for remote input, same-PID reconnect, remote MCP
scoping, and backend interruption. Task 04 records final browser checks.
