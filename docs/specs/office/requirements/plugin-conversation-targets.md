---
status: draft
system: office
created: 2026-09-25
owners:
  - kandev
---

# Plugin conversation automation targets requirements

## Overview

These requirements define the proposed coordinator extension for the office
system. They describe the requested outcome, not currently shipped behavior.
The [implementation plan](../../../plans/plugin-coordinator-platform/plan.md)
stages delivery through public contracts and independent plugin consumers.

## Requirements

### REQ-OFFICE-PLUGIN-TARGETS-001: Generic managed conversation destination

**Intent:** Deliver scheduled prompts to existing plugin conversations.

#### Acceptance criteria

- **AC-OFFICE-PLUGIN-TARGETS-001.1:** An automation shall support an explicitly selected managed conversation in its workspace; firing shall snapshot that destination for the occurrence, enqueue one input per occurrence, and distinguish accepted delivery from completed agent work.
- **AC-OFFICE-PLUGIN-TARGETS-001.2:** A disabled, missing, revoked, or paused destination shall produce a visible delivery state; automation cleanup shall never delete the destination conversation or its shared transcript.
- **AC-OFFICE-PLUGIN-TARGETS-001.3:** The automation editor and portable import/export shall preserve destination intent through explicit instance rebinding, with desktop and phone parity and unchanged defaults for existing task modes.

- **AC-OFFICE-PLUGIN-TARGETS-001.4:** Authorized plugins shall read, create, update, pause, and delete their own managed-conversation schedules through exact commands; changing another installation's schedule or selecting a foreign destination shall be denied.

## Related documents

- [System design](../system-design/plugin-conversation-targets.md)
- [Delivery plan](../../../plans/plugin-coordinator-platform/plan.md)
