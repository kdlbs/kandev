---
status: draft
system: agents
created: 2026-09-25
owners:
  - kandev
---

# Managed agent tool policy requirements

## Overview

These requirements define the proposed coordinator extension for the agents
system. They describe the requested outcome, not currently shipped behavior.
The [implementation plan](../../../plans/plugin-coordinator-platform/plan.md)
stages delivery through public contracts and independent plugin consumers.

## Requirements

### REQ-AGENTS-MANAGED-TOOLS-001: Restricted managed execution

**Intent:** Constrain managed agent turns to their approved tool surface.

#### Acceptance criteria

- **AC-AGENTS-MANAGED-TOOLS-001.1:** A restricted managed turn shall expose only its declared plugin-owned tools and required protocol controls; it shall deny native shell, file, network, ambient MCP, and unrelated plugin tools.
- **AC-AGENTS-MANAGED-TOOLS-001.2:** The host shall derive workspace, installation, conversation, session, and execution provenance from the authenticated tool transport; forged arguments and stale execution generations shall fail before a domain command.
- **AC-AGENTS-MANAGED-TOOLS-001.3:** Capability revocation, plugin disable, or policy replacement shall prevent further authorized tool effects from a live restricted turn and shall cancel it through the normal runtime lifecycle.

### REQ-AGENTS-MANAGED-TOOLS-002: Provider and lifecycle enforcement

**Intent:** Preserve restrictions through launch, resume, and recovery.

#### Acceptance criteria

- **AC-AGENTS-MANAGED-TOOLS-002.1:** The system shall advertise restricted execution support only for adapters that enforce the complete tool policy; an unsupported provider shall fail before launch without falling back to ordinary task tools.
- **AC-AGENTS-MANAGED-TOOLS-002.2:** Resumed, recovered, and retried managed turns shall receive the same validated policy and current execution generation; persisted metadata alone shall not grant authority.
- **AC-AGENTS-MANAGED-TOOLS-002.3:** The system shall expose typed policy failures and auditable denied operations without logging prompts, credentials, or token values.

## Related documents

- [System design](../system-design/managed-tool-policy.md)
- [Delivery plan](../../../plans/plugin-coordinator-platform/plan.md)
