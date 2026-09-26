---
status: draft
system: agents
created: 2026-09-25
owners:
  - kandev
---

# Dynamic Routing Graduation Requirements

## Overview

The Agents system owns dynamic profiles and candidate selection. This
requirement graduates the existing [dynamic routing](dynamic-agent-routing.md)
contract after its [base implementation plan](../../../plans/dynamic-agent-routing/plan.md)
is complete. It does not change the provider error policy.

## Requirements

### REQ-AGENTS-DYNAMIC-ROUTING-GRADUATION-001: Dynamic profiles without a release toggle

**Intent:** Users can configure and select dynamic profiles on a normal
installation, and an existing logical dynamic session retains its recovery
behavior after an upgrade.

#### Acceptance criteria

- **AC-AGENTS-DYNAMIC-ROUTING-GRADUATION-001.1:** In the first default-on stable
  release, dynamic profile creation, selection, execution, and recovery shall be
  available by default in all shipped profiles. An explicit environment value
  or installation override may still disable the feature during that release.
- **AC-AGENTS-DYNAMIC-ROUTING-GRADUATION-001.2:** In a later stable release,
  dynamic profile behavior shall not depend on `features.dynamicAgentRouting`,
  `KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING`, or a retained false override. The
  former key shall be absent from the Feature Toggles page and
  `/api/v1/features` and reserved against reuse.
- **AC-AGENTS-DYNAMIC-ROUTING-GRADUATION-001.3:** Enabling dynamic routing shall
  not convert or select a dynamic profile for an existing concrete-profile
  binding. Only an explicitly selected dynamic profile may route among
  candidates; its durable route and recovery state shall survive restart.
- **AC-AGENTS-DYNAMIC-ROUTING-GRADUATION-001.4:** An ambiguous or post-result
  provider failure shall not start another provider. A persisted waiting route
  shall expose its existing recovery action, and an invalid candidate shall
  fail closed without changing the logical profile binding.
- **AC-AGENTS-DYNAMIC-ROUTING-GRADUATION-001.5:** Desktop and phone users shall
  be able to create or edit a dynamic profile, select it for supported task and
  utility flows, and inspect an actionable routed failure through their
  existing respective surfaces.

## Out of scope

- Automatically migrating legacy Office routing rows or concrete profiles.
- Adding provider cost or subscription telemetry as a candidate selector.
