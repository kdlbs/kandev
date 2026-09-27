---
status: draft
system: tasks
created: 2026-09-25
owners:
  - kandev
---

# Task coordination controls requirements

## Overview

These requirements define the proposed coordinator extension for the tasks
system. They describe the requested outcome, not currently shipped behavior.
The [implementation plan](../../../plans/plugin-coordinator-platform/plan.md)
stages delivery through public contracts and independent plugin consumers.

## Requirements

### REQ-TASKS-COORDINATION-001: Explicit management claims

**Intent:** Prevent competing managers from changing the same delegated work.

#### Acceptance criteria

- **AC-TASKS-COORDINATION-001.1:** The system shall support one optional management claim per task, separate from its worker assignee, with an installation, opaque instance key, and fencing generation.
- **AC-TASKS-COORDINATION-001.2:** Claim acquisition, release, and transfer shall compare the observed task and claim versions; a conflicting plugin or an obsolete generation shall not perform a management mutation.
- **AC-TASKS-COORDINATION-001.3:** A human shall be able to inspect and transfer or release a claim, including when its plugin is disabled or uninstalled; the system shall audit the action and shall not silently steal a claim after a timeout.

### REQ-TASKS-COORDINATION-002: Optional completion requirements

**Intent:** Make task completion evidence enforceable across every entry point.

#### Acceptance criteria

- **AC-TASKS-COORDINATION-002.1:** A task shall support versioned optional completion criteria with evidence and verifier identity; changing a criterion or its declared evidence subject shall invalidate the affected verification. Removing or weakening an unmet criterion shall require explicit human confirmation.
- **AC-TASKS-COORDINATION-002.2:** An enabled completion gate shall block every transition into a completing workflow step until the current criteria are verified, including manual, bulk, queued, agent, and automation paths.
- **AC-TASKS-COORDINATION-002.3:** A human shall be able to inspect blockers and explicitly override a gate with a recorded reason; plugin absence shall leave a visible blocker and shall never require a synchronous plugin callback to evaluate completion.

## Related documents

- [System design](../system-design/coordination-controls.md)
- [Delivery plan](../../../plans/plugin-coordinator-platform/plan.md)
