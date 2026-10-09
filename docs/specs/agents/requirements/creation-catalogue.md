---
status: active
system: agents
created: 2026-10-09
owners:
  - kandev
---

# Agent creation and deletion catalogue requirements

## Overview

Creating an agent profile must leave independently received profiles available
to other consumers in the same browser. Agents owns this capability because it
owns configured agent identities, profile membership and their selectable
projection. Platform's settings discovery and save interface remain dependencies.

This contract covers the normal agent setup flow: an additional profile under a
configured agent, or the first profiles under a discovered agent. It concerns
local availability after creation, not a claim of server deletion.

The list-row deletion contract below covers local availability after one
accepted deletion. Selectable profiles can be received independently of the
loaded agent catalogue. Removing a different profile must preserve those choices.

## Terminology

- **Creation target:** The configured agent whose new profiles are being saved.
- **Independent owner:** A different configured agent already present in the
  browser's loaded catalogue.
- **Independent profile:** A profile published for that owner during a pending
  creation, through the existing live event path.

## Requirements

### REQ-AGENTS-CREATION-CATALOGUE-001: Available profiles after creation

**Intent:** Finish creation without losing another configured agent's available
profiles or their selectable choices.

#### Acceptance criteria

- **AC-AGENTS-CREATION-CATALOGUE-001.1:** When an independent profile becomes
  selectable while an additional-profile creation is pending, accepting that
  creation shall retain the independent owner's received profile and its actual
  selectable choice. A selected independent profile shall retain its label
  instead of becoming an unavailable entry.
- **AC-AGENTS-CREATION-CATALOGUE-001.2:** When additional-profile creation
  succeeds without an intervening event, the accepted profile shall appear
  exactly once alongside the target's existing profiles. Its returned identity
  and saved configuration shall be reflected in selectable choices and the
  existing post-save navigation shall proceed.
- **AC-AGENTS-CREATION-CATALOGUE-001.3:** When the creation POST is rejected
  after an independent profile arrives, the independent profile shall remain
  selectable, the unsaved creation draft shall retain edits made while waiting,
  the shared save shall report failure and the creation route shall stay open.
- **AC-AGENTS-CREATION-CATALOGUE-001.4:** When additional-profile creation is
  accepted but its following MCP save fails, the accepted profile shall remain
  represented with its persisted identity and pending MCP draft, independent
  profiles shall remain selectable, and the shared save shall report failure
  without successful-save navigation.
- **AC-AGENTS-CREATION-CATALOGUE-001.5:** When creating a discovered agent's
  first profiles completes after an independent profile arrives, successful
  creation shall retain that independent profile and choice, publish the
  accepted agent and profile identities, and follow existing navigation. When
  creation succeeds but its MCP save fails, the accepted agent shall still be
  represented with the pending MCP draft, the independent profile shall remain
  selectable, and the shared save shall report failure while retaining this
  branch's existing post-creation navigation.
- **AC-AGENTS-CREATION-CATALOGUE-001.6:** Desktop and phone consumers shall
  receive the same preserved profile choices. Creation validation, shared save
  contribution and draft identity remapping shall retain their existing
  behavior, with no change to page composition, copy or touch interactions.

## Out of scope

The following exclusions apply to `REQ-AGENTS-CREATION-CATALOGUE-001`:

- Concurrent changes to profiles under the creation target itself; no new
  same-owner conflict-resolution or revision-ordering contract.
- Options whose owning agent is absent from the loaded catalogue; this repair
  does not establish a complete snapshot or orphan-option retention guarantee.
- Ordinary saved-agent editing, standalone profile editors, CLI profile editor
  callbacks, duplication, deletion, list-fetch reconciliation or other writers.
- Server persistence, API/event schemas, transport, caches, timestamp arbitration,
  feature flags, migrations and global state refactoring.

## List-row deletion

### REQ-AGENTS-PROFILE-DELETION-CATALOGUE-001: Available profiles after accepted deletion

**Intent:** Remove the accepted deletion target without losing any other current
profile or selectable choice.

#### Acceptance criteria

- **AC-AGENTS-PROFILE-DELETION-CATALOGUE-001.1:** When list-row deletion succeeds,
  the deleted profile shall disappear from the settings catalogue and new-work
  choices. Every other current profile and option shall remain represented,
  including a global choice received while deletion was pending whose owner is
  temporarily absent from the loaded settings catalogue.
- **AC-AGENTS-PROFILE-DELETION-CATALOGUE-001.2:** Unrelated profiles and options
  shall retain their current identity, label, model, configuration, enabled state,
  capability status and eligibility after deletion. Newer received metadata shall
  not revert to older values. Disabled choices shall remain disabled, and the
  global/Office selection boundary shall retain its existing behavior.
- **AC-AGENTS-PROFILE-DELETION-CATALOGUE-001.3:** When two list-row deletions
  overlap, each accepted deletion shall remove only its own target from the
  current catalogue and choices. Either completion order shall preserve all
  unrelated profiles and shall not restore an already removed target.
- **AC-AGENTS-PROFILE-DELETION-CATALOGUE-001.4:** A rejected deletion or conflict
  shall perform no local removal. Choices received while it was pending shall
  remain available. Existing error feedback, handled-error behavior, focus return
  and navigation to the target's guided conflict-resolution page shall remain.
- **AC-AGENTS-PROFILE-DELETION-CATALOGUE-001.5:** Task and subtask consumers shall
  continue to offer the retained eligible choices with their actual labels after
  deleting another profile. An already selected retained choice shall keep its
  label. An ineligible retained option shall not become a selectable choice.
- **AC-AGENTS-PROFILE-DELETION-CATALOGUE-001.6:** Desktop and phone list-row
  deletion shall apply the same preservation behavior. Existing management
  permission, confirmation, cancellation, touch controls and localized copy shall
  remain effective without changes to page composition.

### Deletion exclusions

- Changing server deletion semantics, transport or event contracts, list-fetch
  reconciliation, global handlers, profile selection or Office inventory.
- Materializing absent agent rows, replacing the full catalogue, repairing
  adjacent creation, duplication, save or enablement writers, or redesigning UI.
- Adding revision arbitration, tombstones, flags, persistence or observability.

## Implementation plans

- [List-row deletion catalogue preservation](../../../plans/agent-profile-delete-inventory/plan.md)

## Related contracts

- [Agent system ownership](../README.md)
- [Creation entry points](settings-profile-layout.md)
- [Settings interface parity](../../platform/requirements/agent-settings-parity.md)
- [Creation catalogue design](../system-design/creation-catalogue.md)
