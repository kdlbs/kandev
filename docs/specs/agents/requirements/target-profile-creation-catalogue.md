---
status: active
system: agents
created: 2026-10-10
owners:
  - kandev
---

# Existing-owner profile creation catalogue requirements

## Overview

Accepting an additional profile under an existing configured agent shall preserve
the other profiles currently available under that same owner. Agents owns the
configured identities, profile membership and selectable projection. Platform
owns the shared settings save interface.

The [earlier creation contract](creation-catalogue.md) preserves different-owner
profiles and explicitly excludes concurrent target changes. This bounded extension
owns same-owner publication during additional-profile creation. It does not change
the earlier creation or deletion contracts.

## Terminology

- **Existing owner:** The configured agent receiving the additional profile,
  still represented in the browser catalogue when the accepted result publishes.
- **Sibling:** Any current profile under that owner other than the newly accepted
  profiles from this creation submission.
- **Accepted profile:** A new profile with a persisted identity returned by this
  submission, including a profile whose subsequent MCP configuration save fails.

## Requirements

### REQ-AGENTS-TARGET-CREATION-CATALOGUE-001: Same-owner choices after creation

**Intent:** Complete additional-profile creation without losing the owner's
current sibling profiles or their selectable choices.

#### Acceptance criteria

- **AC-AGENTS-TARGET-CREATION-CATALOGUE-001.1:** When a sibling arrives and is
  selectable while additional-profile creation is pending, accepting creation
  shall retain that sibling in the owner catalogue and selectable choices. An
  already selected sibling shall keep its actual label and remain selectable
  instead of becoming unavailable.
- **AC-AGENTS-TARGET-CREATION-CATALOGUE-001.2:** Every current sibling shall retain
  its received identity, label, model, configuration, enabled state and capability
  metadata when creation publishes. Creation shall not restore captured older
  sibling membership or metadata over the current catalogue.
- **AC-AGENTS-TARGET-CREATION-CATALOGUE-001.3:** Without an intervening event,
  accepted creation shall publish each accepted persisted identity exactly once
  alongside existing profiles, reflect its normalized saved configuration in
  choices, and retain successful-save navigation and draft identity remapping.
- **AC-AGENTS-TARGET-CREATION-CATALOGUE-001.4:** When the creation request is
  rejected after a sibling arrives, that sibling shall remain selectable, newer
  unsaved draft edits shall remain, the shared save shall report failure, and the
  creation route shall remain open without publishing an accepted profile.
- **AC-AGENTS-TARGET-CREATION-CATALOGUE-001.5:** When profile creation is accepted
  but a subsequent MCP save fails, accepted profiles shall remain represented
  with persisted identities and their pending MCP drafts. All current siblings
  shall remain represented and selectable as before; shared save shall report
  failure without successful-save navigation, and draft identities shall remap
  so retry does not recreate accepted profiles.
- **AC-AGENTS-TARGET-CREATION-CATALOGUE-001.6:** Desktop and phone consumers shall
  receive the same preserved profile data. Existing validation, administrator
  permission, eligibility, copy, layout and touch interactions shall remain.

## Out of scope

- Ordinary profile saves, standalone/CLI editors, duplication, deletion,
  enablement writers and dynamic-agent writers.
- Absent-owner or orphan-option inventory guarantees, server deletion reversal,
  server ordering, backend/API/event changes, global WS/store reconciliation,
  tombstones, a general conflict policy or an eligibility/lifecycle framework.
- New-agent first-profile behavior and completed implementation packages; their
  existing contracts remain authoritative.

## References

- [Design](../system-design/target-profile-creation-catalogue.md)
- [Implementation plan](../../../plans/target-profile-creation-catalogue/plan.md)
- [Existing creation and deletion contracts](creation-catalogue.md)
- [Shared settings interface](../../platform/requirements/agent-settings-parity.md)
