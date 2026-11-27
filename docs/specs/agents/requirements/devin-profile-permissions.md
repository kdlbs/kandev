---
status: draft
system: agents
created: 2026-11-03
owners:
  - kandev
---

# Devin profile permissions requirements

## Overview

Users can configure Devin's permission behavior through predefined profile controls.
Agents owns this capability because it owns provider modes, profile settings, and launch intent.
The controls reuse provider mode discovery and the existing profile permission authority.

Smart is the requested default for new profiles.
Existing profiles retain their saved choices.

## Terms

- **Smart:** Devin approves workspace edits and judges other actions before it requests approval.
- **Bypass:** Devin approves tool calls subject to its organization policy.
- **Autonomous:** Devin's permission mode for its OS sandbox. Direct edit/write tools can still request approval.
- **Sandbox override:** A profile choice to enable, disable, or inherit Devin's own sandbox setting.
- **Kandev auto-approval:** The independent setting that answers provider permission requests without user interaction.

## Requirements

### REQ-AGENTS-DEVIN-PERMISSIONS-001: Predefined Devin permission settings

**Intent:** Users can select Devin's documented permission behavior without constructing conflicting CLI flags.

#### Acceptance criteria

- **AC-AGENTS-DEVIN-PERMISSIONS-001.1:** The Devin profile editor shall expose discovered modes, including Smart and Bypass when available.
  It shall expose a predefined sandbox override alongside its extra settings.
- **AC-AGENTS-DEVIN-PERMISSIONS-001.2:** New Devin profiles shall prefer Smart, disable the profile sandbox override, and leave Kandev auto-approval off.
  An explicit creation choice shall take precedence over those defaults.
- **AC-AGENTS-DEVIN-PERMISSIONS-001.3:** When Smart is unavailable, the editor shall explain that limitation and offer supported alternatives.
  It shall require an explicit alternative choice rather than silently substitute another mode.
  An explicit unsupported mode shall prevent inference with a visible failure.
- **AC-AGENTS-DEVIN-PERMISSIONS-001.4:** When the user enables the sandbox override, the profile shall request Autonomous behavior.
  An enabled sandbox override shall not retain Smart or Bypass as the active mode.
  Missing platform support, prerequisites, or provider support shall produce a visible failure without an unsandboxed retry.
- **AC-AGENTS-DEVIN-PERMISSIONS-001.5:** Profile discovery, dependent option resolution, and subsequent host ACP task launches shall use the same saved or draft settings.
  The editor shall distinguish host discovery from support on a remote executor.
- **AC-AGENTS-DEVIN-PERMISSIONS-001.6:** Existing profiles, their duplicates, explicit update clears, and unrelated edits shall retain their saved behavior.
  The editor shall distinguish an inherited sandbox setting from an explicit disabled setting.
  Enabling and disabling the override shall preserve unrelated environment entries and CLI flags.
- **AC-AGENTS-DEVIN-PERMISSIONS-001.7:** Kandev auto-approval shall remain an independent explicit choice.
  The editor shall explain that it can answer remaining Smart or Autonomous permission requests.
  Provider organization policy shall retain its authority.
- **AC-AGENTS-DEVIN-PERMISSIONS-001.8:** Desktop and phone editors shall offer equivalent mode, sandbox, error, save, and reload outcomes.
  New copy shall be localized and controls shall support keyboard and touch input.
  Phone controls shall have at least 44px hit targets without horizontal page overflow.
- **AC-AGENTS-DEVIN-PERMISSIONS-001.9:** Saving and reopening the profile shall retain its permission and sandbox choices.
  A failed discovery shall preserve unfinished edits and the existing retry path.

## Out of scope

- Applying new defaults to existing profiles or running sessions.
- Changing Kandev's organization permissions, agentctl approval protocol, or Devin team policy.
- Editing Devin's user or project configuration files.
- Automatic installation of sandbox prerequisites or sandbox network configuration.
- Independent Smart and Bypass switches, hardcoded provider capability lists, or a new permission transport.
- First-profile persistence ordering, owned by [profile capability discovery](profile-capability-discovery.md).

## Sources and delivery

- [Devin permissions](https://docs.devin.ai/cli/reference/permissions)
- [Devin commands and ACP options](https://docs.devin.ai/cli/reference/commands)
- [Devin sandbox](https://docs.devin.ai/cli/sandbox)
- [System design](../system-design/devin-profile-permissions.md)
- [Implementation package](../../../plans/devin-profile-permissions/plan.md)
