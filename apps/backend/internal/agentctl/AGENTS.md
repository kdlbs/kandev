# agentctl — HTTP server, adapters, agent protocols

Scoped guidance for `apps/backend/internal/agentctl/`. Higher-level backend architecture is in `apps/backend/AGENTS.md`.

## API Groups

For agentctl API, Git operations, or pull request creation, read [API guidance](API-GUIDANCE.md).

## Adapter Model

For agent adapters and dialects, read [adapter guidance](ADAPTERS.md).

## ACP Protocol

When changing or debugging ACP handling, read [ACP protocol guidance](ACP-PROTOCOL.md).

## Process-group cleanup and `RequiresProcessKill`

For subprocess termination and cleanup, read [process cleanup guidance](PROCESS-CLEANUP.md).

## Permission auto-approval has one carrier

`auto_approve` on an agent profile reaches agentctl through
`CreateInstanceRequest.AutoApprovePermissions` (and its explicit
`AutoApprovePermissionsOverride`), which `applyApprovalOverrides` resolves onto
`config.InstanceConfig.AutoApprovePermissions`. `process.Manager` reads that one
field. `AGENTCTL_AUTO_APPROVE_PERMISSIONS` exists for tests and container
bootstrap only; an explicit request override wins over it.

The `/agent/configure` request still accepts an `approval_policy` string so an
older backend can configure a newer agentctl, but nothing reads it. It was
transmitted, stored and logged for years while no code path consulted it, which
made the wire contract actively misleading during a permission investigation. Do
not reintroduce a second permission field here.

## Standalone instance port leases

`instance.PortAllocator` reservations are `PortLease` values containing the
owner, port, and allocator-local generation. Release and mark-unavailable must
compare the full lease. A pre-registration instance remains in the manager's
provisional map until its listener and process resources stop successfully;
failed cleanup keeps the bundle and lease for a serialized retry, and the same
instance ID cannot bind another listener while that bundle remains. Shutdown
drains in-flight cleanup and retries retained bundles before returning.

## Env stripping for credential-mode agents (`StripEnv`)

For credential-mode environments, Tracker Git environment snapshots, or SSH command rewriting, read [environment stripping guidance](ENV-STRIPPING.md).

## Idle-instance reaper (`KANDEV_ACP_IDLE_TIMEOUT`)

`instance.Manager` runs a background goroutine that reaps instances whose owning kandev backend appears to be gone. An instance is "idle" when it has zero in-flight HTTP requests *and* its most recent observed activity is older than the configured timeout. Activity is tracked by a middleware on the per-instance handler that bumps a counter on every request entry/exit, so a long-lived `/agent/stream` WebSocket keeps the instance "active" for as long as the stream is open.

Env knobs:
- `KANDEV_ACP_IDLE_TIMEOUT` (default `1h`, `0` to disable)
- `KANDEV_ACP_IDLE_REAPER_INTERVAL` (default `1m`)

Both accept any value `time.ParseDuration` accepts (`30m`, `2h`, `500ms`, …). Disabled mode is intended for tests and edge cases — production should keep the reaper on.

## MCP stdio command validation

`instance.Manager.buildMcpServerConfigs` calls `exec.LookPath` on every stdio MCP `Command` before passing the list to the agent. Entries whose command can't be resolved are dropped with a warn log (URL-transport MCPs always pass through). This prevents the `/snap/bin/brave` repro in GH issue #1247 — an MCP whose binary was uninstalled after config save no longer causes a permanently broken child process to be spawned every session.

## Built-in MCP package delivery

See [MCP delivery](MCP_DELIVERY.md) for the configure record, helper capability,
remote project-file endpoints, and per-process evidence rules.

Native MCP route mode is immutable for one helper. A create response supplies
the accepted `scoped_mcp_routes` value; retained helpers supply it through
authenticated `InstanceInfo`. Missing fields mean legacy mode, while a failed
mode read is an adoption error. Scoped Core and package routes use the helper's
instance ID under `/i/<id>`; package routes keep their existing package suffixes.
The helper ID scopes accidental routing only and does not replace authenticated
task-owner checks.
