---
status: draft
system: agents
created: 2026-08-13
updated: 2026-08-17
owners:
  - cfl
---
# Dynamic Agent Routing Requirements

## Overview

Users often name profiles by the capability they want rather than a provider brand. A task can use a profile named Frontier for planning and one named Balanced for execution. The dynamic profile selects Claude, Codex, OpenCode, or another provider from an ordered list of complete agent profiles.

## Requirements

### REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001: Dynamic Agent Routing

**Intent:** Users often name profiles by the capability they want rather than a provider brand. A task can use a profile named Frontier for planning and one named Balanced for execution. The dynamic profile selects Claude, Codex, OpenCode, or another provider from an ordered list of complete agent profiles.

#### Acceptance criteria

- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.1:** Kandev always registers one built-in virtual agent family with canonical ID `dynamic` and display name Dynamic. It cannot be disabled or uninstalled, does not expose a CLI command, and is not probed as a concrete inference agent. Profiles created under it have kind `dynamic`.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.2:** Agent settings can create a dynamic profile with a user-defined name, description, icon, and availability scope.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.3:** A dynamic profile references existing concrete agent profiles. It never copies or merges their credentials, environment, model, ACP options, flags, permissions, passthrough behavior, or MCP configuration.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.4:** A dynamic profile's candidate list can reference only concrete, launchable profiles. It cannot reference itself, another dynamic profile, or a rich Office identity in the first version. An Office identity's separate `execution_agent_profile_id` binding can reference a concrete or dynamic profile.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.5:** The profile name is the only capability label. Users can create profiles such as Frontier, Balanced, Economy, Review, or Security Review. Kandev stores no class or tier field and assigns no semantics to those names.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.6:** Each profile has an ordered candidate list. A candidate identifies one concrete profile and stores separate transient-error and hard-error policies. Each class can wait for a trusted near reset, retry the same candidate with bounded exponential backoff, then either skip the candidate or stop.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.7:** A concrete profile with `AutoFallback=true` is not an eligible dynamic candidate. The conductor is the only owner of cross-candidate fallback. An explicit `FallbackModel` remains part of the concrete profile's start-model policy. It does not advance the dynamic candidate list, and turn attribution records the model that ran.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.8:** Dynamic profiles and their concrete candidates participate in the existing profile-in-use dependency dialog. A dependency lookup failure blocks the change. Otherwise, the user can cancel or explicitly confirm deletion or disabling. Confirmed changes keep durable bindings unchanged: stale selected profiles fail closed, while stale or disabled candidates become ineligible and another configured candidate can be selected.

## System design

The migrated technical source is split into [part 1](../system-design/dynamic-agent-routing-01.md), [part 2](../system-design/dynamic-agent-routing-02.md).
