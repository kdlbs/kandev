---
status: active
system: plugins
created: 2026-09-14
owners:
  - kandev
---

# Explicit Plugin Utility Invocation Requirements

## Overview

The plugin system owns the public Host invocation contract. Agent settings own profiles and the platform default utility profile.
A plugin can save a preferred profile in its configuration. It must pass that preference explicitly when it requests a completion.
These requirements replace REQ-PLUGINS-DIRECT-PROFILE-INVOCATION-001. The user accepted this direction after review of PR #2870.
The implementation and verification are recorded in the [implementation plan](../../../plans/plugin-explicit-utility-invocation/plan.md).

## Terminology

- **Default profile:** the profile selected in Settings > Utility Agents under Default utility agent model.
- **Override:** an agent-profile ID supplied in a single invocation. It is not a utility-agent record ID.
- **Plugin preference:** a profile ID saved by the plugin through the existing configuration service.
- **Utility completion:** one prompt and one text response, without a task session or caller-selected working directory.

## Requirements

### REQ-PLUGINS-EXPLICIT-UTILITY-001: Explicit utility execution selection

**Intent:** Make execution selection visible in the invocation contract and keep plugin configuration ownership with the caller.

#### Acceptance criteria

- **AC-PLUGINS-EXPLICIT-UTILITY-001.1:** When the caller omits an override or supplies an empty profile ID, the host shall use the current default profile.
- **AC-PLUGINS-EXPLICIT-UTILITY-001.2:** When the caller supplies an eligible profile ID, the host shall use that exact profile, independent of the default.
- **AC-PLUGINS-EXPLICIT-UTILITY-001.3:** Plugin configuration, field names, manifest selectors, and legacy transition metadata shall not determine invocation selection.
- **AC-PLUGINS-EXPLICIT-UTILITY-001.4:** A missing or ineligible explicit override shall return FailedPrecondition without executing a different profile.
- **AC-PLUGINS-EXPLICIT-UTILITY-001.5:** An unset, missing, or ineligible default shall return FailedPrecondition when the caller omits an override.
- **AC-PLUGINS-EXPLICIT-UTILITY-001.6:** Without agent_invoke permission, the call shall fail before settings access, profile lookup, or execution.
- **AC-PLUGINS-EXPLICIT-UTILITY-001.7:** Cancellation and operational failures shall remain distinct from configuration failures. No failure shall trigger implicit profile fallback.
- **AC-PLUGINS-EXPLICIT-UTILITY-001.8:** Changes to the default shall affect subsequent default invocations. A default change shall not redirect an explicit override or an invocation already dispatched.
- **AC-PLUGINS-EXPLICIT-UTILITY-001.9:** The plugin shall read its saved preference and pass it explicitly. Saving, clearing, and reloading a preference shall preserve that behavior.
- **AC-PLUGINS-EXPLICIT-UTILITY-001.10:** A host that cannot honor the explicit selection contract shall reject the new client call before execution. It shall not silently ignore an override.
- **AC-PLUGINS-EXPLICIT-UTILITY-001.11:** Existing prompt-only clients on the revised host shall use the platform default. Migration documentation shall state this behavior change.
- **AC-PLUGINS-EXPLICIT-UTILITY-001.12:** Invocation shall remain a single text completion. It shall not create a task, session, or worktree, or accept a caller-selected working directory.

## Out of scope

- General agent execution, cwd, streaming, tools, session lifecycle, and arbitrary execution options.
- Changes to eligibility: profiles remain enabled, global, non-CLI-passthrough, and explicitly inference-capable.
- UI redesign, new configuration storage, automatic conversion of utility-agent record IDs into profile IDs, or new capability grants.
