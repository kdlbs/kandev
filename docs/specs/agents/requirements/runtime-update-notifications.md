---
status: active
system: agents
created: 2026-10-01
updated: 2026-10-02
owners:
  - Kandev
---
# Agent Runtime Update Notifications and Automation

## Overview

Operators need to discover updates while working and optionally authorize Kandev to activate validated runtime versions. This contract extends [managed runtime versions](runtime-updates.md) to background awareness across every registered agent. A runtime is the executable or package used for the agent's structured execution; separately installed login and passthrough CLIs retain their own ownership.

## Requirements

### REQ-AGENTS-RUNTIME-NOTIFY-001: Runtime coverage and awareness

- **AC-AGENTS-RUNTIME-NOTIFY-001.1:** Every registered agent shall expose runtime ownership, release source when known, known current/effective and latest versions, and managed, manual, or unsupported actions. Disabled, unavailable, custom, and virtual agents shall remain explicit. Unknown versions or failed checks shall never be presented as current.
- **AC-AGENTS-RUNTIME-NOTIFY-001.2:** Available enabled agents shall be checked without opening Settings. Checks shall be bounded, shared by release source, cached for six hours on success and fifteen minutes on failure, and cancelled during shutdown. Offline or failed sources shall not create user-facing failure notifications.
- **AC-AGENTS-RUNTIME-NOTIFY-001.3:** A newer known runtime version shall generate an app notification naming the agent, runtime, and new version, linking to that runtime's settings or verified vendor guidance. Delivery shall respect the existing update-notification preference and persist deduplication by recipient, agent, runtime source, and version across reloads and backend restarts.
- **AC-AGENTS-RUNTIME-NOTIFY-001.4:** Retired by AC-AGENTS-RUNTIME-NOTIFY-001.7. The former persistent app-wide update indicator is removed from the intended contract.
- **AC-AGENTS-RUNTIME-NOTIFY-001.5:** On an ordinary visit to Settings > Agents, runtime update settings shall appear after the installed-agent content as a collapsed section. Expanding it shall show compact runtime rows with named ownership/source, current and latest versions or explicit unknown values, supported actions, and automatic update controls. Shared explanatory copy shall appear once rather than under every runtime; retained outcomes and unavailable-runtime limitations shall remain accessible.
- **AC-AGENTS-RUNTIME-NOTIFY-001.6:** Opening a runtime notification shall expand the runtime section and reveal its destination, including a disabled or unavailable runtime inside the additional registrations disclosure. Desktop and phone users shall be able to collapse and reopen the section without losing policy drafts. Phone controls shall have at least 44px touch targets and content shall not cause horizontal page overflow.

- **AC-AGENTS-RUNTIME-NOTIFY-001.7:** Desktop and phone application views shall show no persistent floating agent runtime update button or count, including while newer runtimes are available. Runtime version management, ownership information, and manual guidance shall remain reachable through Settings > Agents and runtime notification links without hover.

### REQ-AGENTS-RUNTIME-NOTIFY-002: Opt-in verified automatic updates

- **AC-AGENTS-RUNTIME-NOTIFY-002.1:** Automatic updates shall be disabled initially. Only an operator with agent configuration permission shall enable an install-wide per-runtime policy, and that policy shall persist across restart with its trusted runtime identity.
- **AC-AGENTS-RUNTIME-NOTIFY-002.2:** Automation shall use a supported staging, validation, and activation path. External installations without verified ownership, candidate isolation, and recovery shall expose manual guidance and shall never run guessed install/update commands.
- **AC-AGENTS-RUNTIME-NOTIFY-002.3:** Automatic activation shall occur only after the exact stable published candidate passes the same runtime validation as a manual update. Failed preparation, validation, or persistence shall preserve the prior selection and capabilities. Current and candidate versions shall not be confused in outcomes.
- **AC-AGENTS-RUNTIME-NOTIFY-002.4:** Running sessions and executor-owned installations shall remain unchanged. Managed selection affects subsequent launches according to the existing managed-runtime contract; backend preparation shall not claim to modify remote installations, native dependency CLIs, login data, or passthrough runtimes.
- **AC-AGENTS-RUNTIME-NOTIFY-002.5:** Manual version selection, rollback, and return-to-default shall remain available for managed runtimes. A manual selection shall stop automatic updates. Opt-out or an intervening selection shall prevent a staged automatic candidate from activating.
- **AC-AGENTS-RUNTIME-NOTIFY-002.6:** Success and failure outcomes shall persist, name the agent and runtime, include old and target versions, and provide recovery through manual version selection or vendor guidance. A failed target shall not be attempted repeatedly; explicit re-enablement can authorize another attempt.
- **AC-AGENTS-RUNTIME-NOTIFY-002.7:** Settings controls and outcomes shall be localized in every shipped locale, usable on desktop and phone, and retain authoritative saved state on failure.

## Exclusions

Model discovery, vendor credential/configuration migrations, hot-swapping sessions, arbitrary package or command input, changing vendor auto-update settings, and inventing rollback for external installations are excluded. Native/vendor mechanisms without safe verified activation remain manual capabilities, with explicit limitations rather than successful-update claims.

## Design and implementation

See [runtime update notifications design](../system-design/runtime-update-notifications.md), [original delivery plan](../../../plans/agent-runtime-notifications/plan.md), and [compact settings follow-up plan](../../../plans/agent-runtime-settings-compact/plan.md), and [floating indicator removal plan](../../../plans/remove-agent-runtime-update-indicator/plan.md).
