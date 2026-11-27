---
status: current
system: plugins
requirements:
  - REQ-PLUGINS-AGENT-TOOLS-001
created: 2026-10-12
---

# Agent tool config readiness

## Boundary

This refines catalog membership for [plugin-contributed agent tools](../requirements/agent-tools.md).
The plugin service owns the authoritative catalog (`Service.AgentToolCatalog`) and the
operator config store. The MCP handlers, the live catalog refresher, and agentctl
consume the catalog unchanged. Ordinary invocation joins the existing per-plugin
dispatch read lease. Manifest validation, config validation
(`validateConfigSchema`), secret vault storage, tool payloads, and the settings UI are
unchanged.

| Requirement | Design sections |
| --- | --- |
| REQ-PLUGINS-AGENT-TOOLS-001 (AC 001.9) | Catalog membership; readiness rule |
| REQ-PLUGINS-AGENT-TOOLS-001 (AC 001.10) | Recompute on config save; failure isolation |
| REQ-PLUGINS-AGENT-TOOLS-001 (AC 001.11) | Invocation ordering |
| REQ-PLUGINS-AGENT-TOOLS-001 (AC 001.12) | Catalog recompute ordering |

## Problem

`AgentToolCatalog` lists every declared tool of every active plugin. A connector plugin
that declares `config_schema.required` fields (for example a base URL and a token) is
active as soon as it is installed and enabled, before the operator has configured it.
Its tools are advertised to every task agent, and each call can only fail because the
plugin has nothing to connect to. The settings list already marks such a plugin
"Setup required", so the agent-facing catalog disagrees with what the operator sees.

## Catalog membership

`AgentToolCatalog` builds definitions only from records that are active **and**
config-ready. Collision detection, sorting, normalization, and revision handling stay in
`buildAgentToolDefinitions` and the existing snapshot comparison, so an unchanged
effective tool set still does not advance the revision.

A record is config-ready when its `config_schema.required` list is empty (or absent), or
when the stored config sets every listed field. Malformed `required` entries are ignored
the same way `checkRequiredKeys` ignores them.

## Readiness rule

A required field is unset when its key is absent from the stored config, or its value is
null, or its value is a string that is empty after trimming whitespace. Any other value,
including `false` and `0`, is set.

This intentionally mirrors `isConfigured` in
`apps/web/hooks/domains/plugins/use-plugin-setup-status.ts`, which drives the
"Setup required" badge. It does not reuse `isZeroConfigValue`: that helper decides
which secret values are masked and treats `false` and `0` as zero, which would hide the
tools of a plugin whose required boolean or number is legitimately stored as `false` or
`0`. A secret field's stored value is its non-empty vault reference, so a saved secret
counts as set without reading the vault.

## Recompute on config save

`UpdateConfig` calls `notifyAgentToolCatalogChanged` after the config commit and secret
cleanup, before `restartForConfigChange`. The production refresher coalesces
requests on a buffered channel. Listener lookup can wait for an in-flight catalog
calculation under the service snapshot mutex before queueing that refresh. Placing
it before the restart means the catalog is recomputed even when the plugin is not
running or its restart fails. A rejected or failed save does not
signal. `UpdateConfig` is the only writer of plugin config; uninstall and upgrade already
signal.

Catalog readiness describes stored config presence, not a subprocess liveness
guarantee. Lifecycle status and persistence behavior remain unchanged. A
failed restart requests another catalog and delivery refresh even if the status
transition or its cleanup fails. Consumers read the authoritative registry state;
a failed record write retains the existing registry rollback behavior. Catalog
refresh does not restart the agent or task session, while a running plugin
subprocess still restarts to read the saved config.

## Catalog recompute ordering

The existing service snapshot mutex covers config-readiness reads, definition
calculation, snapshot comparison, and revision assignment as one operation.
Concurrent catalog requests therefore cannot publish an older config calculation
with a newer revision than a post-commit refresh. Delayed responses retain their
older revision, which downstream consumers can reject after seeing the newer
snapshot. Unchanged definitions still retain their revision and service generation.

Config persistence releases its store lock before notification takes the service
mutex. Catalog readiness takes no dispatch lease; it can finish while a config
writer holds the dispatch write lease and waits to queue its refresh. Listener
callbacks run after notification releases the service mutex. The permanent
regression pauses a pre-commit config read, commits a configure or clear update,
and lets concurrent recomputes finish before comparing their snapshot revisions.

## Invocation ordering

`UpdateConfig` holds the existing per-plugin dispatch write lock across the config
commit, catalog invalidation, and subprocess restart. Ordinary `InvokeAgentTool`
now takes the read side before reading the installation and retains it through
the RPC, matching managed invocation. An invocation begun after the newly
configured tool is advertised therefore cannot reach the old unconfigured
process or the gap before its replacement is ready. The same lease also protects
ordinary calls against the existing disable, uninstall, and upgrade write paths.

The initial notification stays after the commit and before restart so clearing
required config invalidates the catalog immediately, and inactive or failed
restarts still recompute it. No subprocess-liveness membership gate is added.
A deterministic regression blocks the replacement handshake, reads the newly
advertised catalog, then starts invocation and observes its dispatch wait. After
a failed restart, invocation reads the resulting inactive status instead of
returning the premature missing-runtime error.

## Failure isolation

Readiness reads the stored config per record through the existing store. If that read
fails (for example an unparsable config file), the record is treated as not config-ready
and the service logs a warning with the plugin ID only; config values are never logged.
The catalog call itself still succeeds, so one plugin's broken config cannot remove
every other plugin's tools or fail the MCP tool list. A missing config file is an empty
config, not an error.

## Cost

Only records that declare required fields read their config, once per catalog build.
Catalog builds happen on MCP tool-list requests and lifecycle refreshes, and plugin
counts are small, so the extra local file reads are bounded.

## Non-goals

- Config readiness adds no new invocation membership gate. A client holding a stale list may still call a hidden
  tool; the plugin answers as it does today, and the next `tools/list_changed` refresh
  removes the tool.
- No new plugin status, API field, or UI change. The settings badge already reports the
  same condition.
