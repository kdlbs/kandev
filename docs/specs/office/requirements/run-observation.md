---
status: draft
system: office
created: 2026-09-17
owners:
  - kandev
---

# Office Run Observation

## Overview

Office owns the run and activity projections operators use to understand agent
work. Names identify entities; UUIDs remain diagnostic and navigation identifiers.
This extends the existing dashboard name/identifier contract to run detail and
the workspace activity feed.

## Requirements

### REQ-OFFICE-RUN-OBSERVATION-001: Understandable run and activity identity

**Intent:** Operators can identify the agent, skills, runtime and affected work
without manually resolving UUIDs.

#### Acceptance criteria

- **AC-OFFICE-RUN-OBSERVATION-001.1:** Run detail shall show the Office agent name and human-readable skill names or slugs. Version/hash information shall remain available. New runs shall retain skill labels captured for that run even after a skill rename or deletion.
- **AC-OFFICE-RUN-OBSERVATION-001.2:** Adapter and model shall describe the actual invoked runtime, including provider fallback. Before invocation, the UI shall distinguish configured runtime from actual invocation or show unavailable; an Office agent UUID shall never serve as the adapter label.
- **AC-OFFICE-RUN-OBSERVATION-001.3:** Workspace activity and dashboard activity shall resolve agent names and task identifiers/titles consistently. System and scheduler actors shall have readable labels. Navigation shall still use stable IDs.
- **AC-OFFICE-RUN-OBSERVATION-001.4:** Missing, deleted or inaccessible entities shall have a localized unavailable/deleted label with a short diagnostic ID where useful. Missing entities shall not fail the entire feed, and labels shall never be resolved across workspace boundaries. Legacy runs may use explicitly current labels when historical labels do not exist.
- **AC-OFFICE-RUN-OBSERVATION-001.5:** Desktop and phone shall expose the same information without document-level horizontal overflow. Long names shall wrap or have a touch-accessible disclosure. Application-authored labels shall use localization; user-authored names shall remain unchanged.

## Out of scope

Renaming entities, changing runtime behavior, reconstructing unknown historical
adapter choices, and a general redesign of activity event wording.

## Related requirements and design

[Dashboard identity](dashboard.md), especially AC-OFFICE-DASHBOARD-001.7 and .8.
[Observation projections](../system-design/run-observation.md).
