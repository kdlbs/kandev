---
status: active
system: agents
created: 2026-08-08
owners:
  - kandev
---
# Profile-backed Utility Agents Requirements

## Overview

Utility agents run unattended one-shot jobs, but choosing only an agent family and model omits the permissions and launch configuration that make that agent safe and reliable. Users need utility jobs to run with an agent profile they have already configured, so a job does not stop midway for a permission choice that its caller cannot answer.

## Requirements

### REQ-AGENTS-UTILITY-AGENT-PROFILES-001: Profile-backed Utility Agents

**Intent:** Utility agents run unattended one-shot jobs, but choosing only an agent family and model omits the permissions and launch configuration that make that agent safe and reliable. Users need utility jobs to run with an agent profile they have already configured, so a job does not stop midway for a permission choice that its caller cannot answer.

#### Acceptance criteria

- **AC-AGENTS-UTILITY-AGENT-PROFILES-001.1:** Settings > Utility Agents has one **Default utility agent profile** selection. The choice is an eligible concrete or dynamic global agent profile, not an agent family/model pair.
- **AC-AGENTS-UTILITY-AGENT-PROFILES-001.2:** Each built-in utility action either inherits the default utility profile or selects one eligible profile as an override.
- **AC-AGENTS-UTILITY-AGENT-PROFILES-001.3:** Every custom utility agent selects one eligible concrete or dynamic profile. A custom utility agent cannot be created or saved without a profile.
- **AC-AGENTS-UTILITY-AGENT-PROFILES-001.4:** A built-in utility action without a profile override inherits the default utility profile. A stale override remains **unconfigured** after its profile is deleted or disabled and cannot run until the user repairs it.
- **AC-AGENTS-UTILITY-AGENT-PROFILES-001.5:** An empty `unconfigured` built-in binding is normalized to `inherit`. Selecting Default in an action picker persists the same inherited state and never submits an empty explicit binding.
- **AC-AGENTS-UTILITY-AGENT-PROFILES-001.6:** Eligible choices are enabled, non-deleted, global concrete profiles for ACP inference-capable agents and global dynamic profiles with at least one valid candidate. CLI-passthrough profiles and workspace-scoped Office profiles are not eligible.
- **AC-AGENTS-UTILITY-AGENT-PROFILES-001.7:** A utility invocation resolves its effective profile at the start of the call. It uses that profile's agent, model, mode, dynamic config options, enabled CLI flags, command prefix, environment/secret references, and permission policy. Editing the profile affects the next call; an in-flight call keeps the configuration resolved when it started.
- **AC-AGENTS-UTILITY-AGENT-PROFILES-001.8:** A dynamic selection resolves through the shared dynamic conductor. The caller submits the same profile ID as for a concrete selection. The call record retains that logical profile ID and the concrete execution profile that produced the final result.

## System design

The migrated technical source is split into [part 1](../system-design/utility-agent-profiles.md).
