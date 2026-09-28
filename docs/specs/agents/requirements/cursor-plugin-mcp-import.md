---
status: draft
system: agents
created: 2026-09-28
owners:
  - kandev
---

# Cursor plugin MCP import requirements

## Overview

This capability makes supported, installed Cursor plugin MCP definitions available to local task launches. The reported ACP discovery gap must be reproduced against a recorded Cursor version before implementation; it is not a universal claim about every Cursor release.
The agent system owns this capability because it manages profile preferences, runtime project MCP materialization, and provider launch behavior.

## Requirements

### REQ-AGENTS-CURSOR-PLUGIN-MCP-001: Plugin MCP server discovery

**Intent:** Local Cursor sessions discover and import MCP server configurations from installed Cursor plugins and user-level settings.

#### Acceptance criteria

- **AC-AGENTS-CURSOR-PLUGIN-MCP-001.1:** When enabled for a local Cursor launch, Kandev shall discover only installed, enabled plugins applicable to the launch scope, using manifests under `~/.cursor/plugins/cache/`, `~/.cursor/plugins/local/`, and `~/.cursor/plugins/marketplaces/` for declared `mcpServers`. A cached version or marketplace catalog entry alone shall not establish eligibility.
- **AC-AGENTS-CURSOR-PLUGIN-MCP-001.2:** Discovery shall inspect `mcp.json`, `.mcp.json`, `.cursor-plugin/plugin.json`, and `plugin.json` in each eligible plugin directory, including plugin-relative MCP file references from descriptors.
- **AC-AGENTS-CURSOR-PLUGIN-MCP-001.3:** Discovery shall also read user-level `~/.cursor/mcp.json` when present.
- **AC-AGENTS-CURSOR-PLUGIN-MCP-001.4:** Supported discovered server entries shall preserve server transport (`stdio`, `http`, `sse`, `streamable-http`), `command`, `args`, `env`, `url`, and `headers`.
- **AC-AGENTS-CURSOR-PLUGIN-MCP-001.5:** Server name `kandev` is strictly reserved for Kandev internal tools and shall never be overwritten by a plugin definition.
- **AC-AGENTS-CURSOR-PLUGIN-MCP-001.6:** Precedence shall be explicit profile servers > existing project servers > global user servers > eligible plugin servers. Within eligible plugins, newest valid manifest modification time wins, with canonical source path as a deterministic tie-breaker. Only the selected installed version is eligible. Invalid entries shall not suppress valid lower-priority entries.
- **AC-AGENTS-CURSOR-PLUGIN-MCP-001.7:** Absent Cursor plugins or unreadable manifests shall produce a silent no-op and allow normal startup without failing the task launch.

- **AC-AGENTS-CURSOR-PLUGIN-MCP-001.8:** Plugin-relative references and supported plugin-root variables shall resolve against the eligible plugin root. Unsupported required fields or unresolved plugin variables shall cause that server to be skipped rather than launch with altered semantics.
- **AC-AGENTS-CURSOR-PLUGIN-MCP-001.9:** Imported definitions shall respect the applicable MCP transport and executor policies. Discovery shall not execute commands, resolve secrets into logs, or modify source manifests or global configuration.

- **AC-AGENTS-CURSOR-PLUGIN-MCP-001.10:** Materialized plugin server names shall preserve Cursor's native `plugin-<plugin-name>-<server-name>` identity so that shared workspace OAuth entries remain addressable. Logical manifest names retain their policy and precedence semantics. Credential objects and Cursor approval/disable settings shall not be rewritten to compensate for a different identity.

- **AC-AGENTS-CURSOR-PLUGIN-MCP-001.11:** Native marketplace imports shall use the current account's effective enabled-plugin inventory and its selected immutable version. A missing login, unavailable inventory, unsupported version or absent selected cache root shall omit those imports without activating another cached version. Explicit project/profile/global definitions remain available through their existing paths.
- **AC-AGENTS-CURSOR-PLUGIN-MCP-001.12:** A server disabled by exact native identifier in the task's source repository shall be excluded from automatic imports into that task workspace. The task's own disables shall also be honored. Unrelated workspaces shall not contribute disables. This inheritance applies to imports only; it shall not rewrite source or destination disabled/approval files or override explicit project/profile definitions. A malformed or unreadable disable store shall not be treated as an empty list.
- **AC-AGENTS-CURSOR-PLUGIN-MCP-001.13:** Inventory discovery shall run only for eligible local launches with the import preference enabled and the same runtime user home. It shall use existing Cursor account authentication only with Cursor's verified HTTPS service, keep credentials and raw service payloads out of logs and application state, and never initiate login, token refresh or plugin installation.

### REQ-AGENTS-CURSOR-PLUGIN-MCP-002: Profile preference and settings control

**Intent:** Users can control whether each Cursor profile automatically imports plugin MCP servers.

#### Acceptance criteria

- **AC-AGENTS-CURSOR-PLUGIN-MCP-002.1:** Existing and new Cursor profiles shall default to enabled (`cursor_plugins_mcp_enabled: true`). An explicit disabled value shall survive save, reload, restart, and duplication.
- **AC-AGENTS-CURSOR-PLUGIN-MCP-002.2:** Cursor profile settings shall expose a labeled checkbox and description explaining automatic plugin MCP import.
- **AC-AGENTS-CURSOR-PLUGIN-MCP-002.3:** Disabling the preference shall omit plugin-discovered MCP servers from the generated `.cursor/mcp.json`, retaining `kandev`, explicit profile servers, and user-owned project entries. On the next preparation, unchanged entries previously imported by Kandev shall be removed; user edits shall survive. This controls Kandev copying, not Cursor's native global-config discovery or authentication sharing.
- **AC-AGENTS-CURSOR-PLUGIN-MCP-002.4:** Desktop and mobile settings interfaces shall render, save, and discard the preference consistently with at least a 44px touch target on mobile and localized strings across supported locales.
- **AC-AGENTS-CURSOR-PLUGIN-MCP-002.5:** Settings discovery and API DTO contracts shall expose the boolean field and its default.

### REQ-AGENTS-CURSOR-PLUGIN-MCP-003: Worktree MCP configuration merging

**Intent:** Discovered plugin MCP servers are safely merged into the task's workspace MCP configuration.

#### Acceptance criteria

- **AC-AGENTS-CURSOR-PLUGIN-MCP-003.1:** Kandev shall compose discovered plugin MCP servers into the generated `.cursor/mcp.json` alongside Kandev internal tools during task preparation.
- **AC-AGENTS-CURSOR-PLUGIN-MCP-003.2:** If a project already has a `.cursor/mcp.json`, Kandev shall merge plugin servers under `mcpServers` without overwriting repository-committed custom fields.
- **AC-AGENTS-CURSOR-PLUGIN-MCP-003.3:** Both Cursor ACP (`cursor-acp`) and Cursor-strategy terminal profiles on local executors, including fresh, resume, and restart preparation, shall receive the merged configuration.
- **AC-AGENTS-CURSOR-PLUGIN-MCP-003.4:** Plugin discovery shall only run for local executions that share the host filesystem; remote and containerized executors, unknown executor types, and launches with a different runtime home shall remain isolated.

- **AC-AGENTS-CURSOR-PLUGIN-MCP-003.5:** Repeated preparation, plugin removal, disable, and cleanup shall reconcile only Kandev-owned imported entries. Malformed or symlinked destination files shall remain untouched; a failure to remove known stale owned imports shall prevent launching with those imports silently active.
- **AC-AGENTS-CURSOR-PLUGIN-MCP-003.6:** Concurrent preparation and delayed cleanup for the same workspace shall not delete a newer execution's configuration or overwrite user changes. Imported secrets shall not appear in logs, profile responses, or browser-visible execution metadata.

## Scope and limits

- Discovery reads plugin metadata from disk on launch; it does not install plugins or manage plugin market lifecycles.
- Credential management remains handled by the Cursor OAuth bridge (REQ-AGENTS-CURSOR-AUTH-001); this capability handles server definitions.

## Implementation plans

- [Cursor plugin MCP import plan](../../../plans/cursor-plugin-mcp-import/plan.md)

## Discovery/auth repair

[Discovery and authenticated-source repair](../../../plans/cursor-mcp-discovery-auth-repair/plan.md)
replaces the unverified local-registry assumption. The user selected source-repository-only disable inheritance; credential discovery
continues to scan eligible existing workspaces independently.
