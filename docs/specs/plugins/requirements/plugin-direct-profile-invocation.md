---
status: deprecated
system: plugins
created: 2026-09-14
owners:
  - kandev
---
# Plugin Direct Agent Profile Invocation Requirements

> Superseded by [explicit invocation requirements](plugin-explicit-utility-invocation.md).
> This document records the previous contract; do not implement its implicit routing.

## Overview

Plugins performing a one-shot LLM step previously chose execution identity by
selecting a utility agent, which forced them to route through a secondary
configurable object and hid the actual agent profile that would run. Users
need to select the platform agent profile directly from a plugin's settings so
the profile that executes is the profile they configured, while existing
plugins that still declare the legacy `utility_agent` selector keep working.
The plugin system owns the Host invocation contract and its configuration
state; the agents system continues to own profile eligibility and utility
execution resolution.

## Terminology

- **Direct profile selection:** a plugin `config_schema` field with
  `type: string` and `format: agent-profile` whose stored value is a stable
  agent-profile ID executed directly by the host.
- **Legacy utility-agent selection:** an existing `config_schema` field with
  `format: utility-agent` whose stored value resolves through the utility
  agent catalog, unchanged for plugins that still declare it.
- **Eligibility:** enabled, non-deleted, global, non-CLI-passthrough profiles
  whose agent supports sessionless inference; the backend remains
  authoritative.

## Requirements

### REQ-PLUGINS-DIRECT-PROFILE-INVOCATION-001: Direct agent profile invocation for plugins

**Intent:** Let a plugin invoke one executable agent profile of the
operator's choosing without inventing a custom utility prompt, while keeping
the public Host RPC, SDK, and capability gate source-compatible and
preserving installed legacy configurations.

**User story:** As a Kandev operator configuring a plugin such as Notes, I
want to pick the agent profile that performs the plugin's LLM step directly
in the plugin's settings, so that the profile I configured is the profile
that executes and I do not have to maintain a separate custom utility
prompt.

#### Acceptance criteria

- **AC-PLUGINS-DIRECT-PROFILE-INVOCATION-001.1:** When a plugin manifest
  declares a `config_schema` field with `format: agent-profile`, Settings >
  Plugins shall list only eligible platform agent profiles and shall not list
  utility or custom-prompt rows, so built-in and custom utility prompts with
  the same or different IDs cannot be confused with executable profiles.
- **AC-PLUGINS-DIRECT-PROFILE-INVOCATION-001.2:** When the user selects a
  profile, the plugin configuration shall persist that profile's stable ID
  through the shared settings save path, the contributor shall become dirty
  and save successfully, and a reload shall again show the selected profile's
  display label.
- **AC-PLUGINS-DIRECT-PROFILE-INVOCATION-001.3:** When a profile is disabled,
  deleted, workspace-scoped, or CLI-passthrough, the picker shall exclude it
  from new selections and the settings surface shall represent a previously
  saved unavailable selection safely and replaceably rather than failing
  render.
- **AC-PLUGINS-DIRECT-PROFILE-INVOCATION-001.4:** When a plugin with
  `capabilities.agent_invoke: true` invokes `Host.InvokeUtilityAgent` with a
  non-empty direct `agent_profile` configuration, the host shall route the
  prompt to `ExecuteProfilePrompt` using that exact profile ID and shall not
  resolve or execute a utility or custom-prompt definition.
- **AC-PLUGINS-DIRECT-PROFILE-INVOCATION-001.5:** When the direct profile
  selection is missing, or the selected profile is missing or ineligible at
  execution time, the host shall return a typed gRPC `FailedPrecondition`
  that plugins can translate into an actionable configuration message rather
  than a generic execution failure.
- **AC-PLUGINS-DIRECT-PROFILE-INVOCATION-001.6:** When a plugin's
  configuration holds both a direct profile and a legacy `utility_agent`
  value, the direct profile shall take precedence; when only the legacy value
  exists and the active manifest declares that selector, the existing
  utility-agent resolution and execution behavior shall remain unchanged.
- **AC-PLUGINS-DIRECT-PROFILE-INVOCATION-001.7:** A plugin that does not
  declare `capabilities.agent_invoke` shall be denied before any profile or
  configuration lookup, and operational runner failures that occur after
  successful configuration validation shall remain execution errors instead
  of being misreported as configuration failures.
- **AC-PLUGINS-DIRECT-PROFILE-INVOCATION-001.8:** The plugin manifest and its
  contract tests shall declare the agent-profile-backed setting with a
  pinned field name and format, and the plugin's package metadata shall stay
  internally consistent.

## Out of scope

- Changing the public `Host.InvokeUtilityAgent(ctx, prompt)` RPC or SDK
  signature, or the `agent_invoke` capability-gate semantics.
- Adding new config migration; a database or schema migration is not
  required for the dual-format selection.
- Changing utility-agent resolution for plugins that continue to declare
  only the legacy `utility_agent` selector.
- Changing agent-profile eligibility rules owned by the agents system.
