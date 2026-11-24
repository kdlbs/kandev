---
status: active
system: agents
created: 2026-11-23
owners:
  - kandev
---

# Copilot context tier requirements

### REQ-AGENTS-COPILOT-CONTEXT-TIER-001: Long context profile preset

Users can choose a Copilot profile for new sessions that need a larger context window.
The agent profile owns the launch preference. Copilot determines the effective
window from its model and account capabilities.

#### Acceptance criteria

- **AC-AGENTS-COPILOT-CONTEXT-TIER-001.1:** On desktop and phone, the Copilot
  profile shows a Long context switch in CLI Flags. It is off by default and
  the profile saves the user's choice.
- **AC-AGENTS-COPILOT-CONTEXT-TIER-001.2:** The switch controls whether the
  profile contributes the `--context long_context` argument pair to the
  command preview and Copilot launch. It uses the existing CLI flag path.
- **AC-AGENTS-COPILOT-CONTEXT-TIER-001.3:** The preset is launch configuration.
  Chat and workflow session overrides cannot use it to replace a running
  Copilot process.
