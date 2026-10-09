---
status: active
system: agents
created: 2026-10-09
owners:
  - kandev
---

# Agent profile mutation catalogue requirements

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

The concrete editor save contract also preserves the current catalogue when a
save completes after a live deletion. It does not change server deletion or
the shared settings editor's draft and revision contract.

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
- [Concrete editor save catalogue preservation](../../../plans/agent-profile-save-catalogue/plan.md)

## Concrete editor save

### REQ-AGENTS-PROFILE-SAVE-CATALOGUE-001: Current catalogue after concrete editor save

**Intent:** Saving one concrete profile must not restore a deleted profile or
replace another current profile's membership, metadata or selectable choice.

#### Acceptance criteria

- **AC-AGENTS-PROFILE-SAVE-CATALOGUE-001.1:** When a sibling profile is removed
  by an accepted live deletion while a concrete editor save is pending, a
  successful save of the other profile shall leave the sibling absent from
  the settings catalogue and selectable choices. An already selected deleted
  sibling shall retain the existing unavailable state after the save completes.
- **AC-AGENTS-PROFILE-SAVE-CATALOGUE-001.2:** Every unrelated current agent,
  profile and option shall retain its membership and latest received metadata
  after that save, including same-owner updates, new profiles and independently
  held choices. Enabled state, eligibility and the global/Office boundary shall
  retain their existing meanings. The save shall not insert a missing owner or
  restore its target if that target was removed while the request was pending.
- **AC-AGENTS-PROFILE-SAVE-CATALOGUE-001.3:** With its owner and target still
  present and its response accepted by existing editor revision rules, an
  ordinary successful save shall publish the normalized returned profile and
  actual selectable label. Its baseline, dirty clearing and edits made after
  submission shall retain the shared editor's existing behavior. A response
  rejected by those revision rules shall not replace the newer profile.
- **AC-AGENTS-PROFILE-SAVE-CATALOGUE-001.4:** A rejected PATCH shall not publish
  a catalogue replacement. Deletions and updates received while it was pending
  shall remain effective. Existing validation, permission checks, dependency
  conflicts, explicit force confirmation, error reporting and draft recovery
  shall remain effective.
- **AC-AGENTS-PROFILE-SAVE-CATALOGUE-001.5:** Desktop and phone consumers,
  including task and subtask choices, shall receive the same preserved
  catalogue outcome through their existing selectors and shared save controls.
  No page composition, copy, touch behavior or navigation change is required.

### Save exclusions

- Other save writers, dynamic editors, creation, duplication, deletion actions,
  list-fetch reconciliation, availability policy and global event handlers.
- Server ordering or deletion semantics, backend/API changes, persistence,
  global journals, tombstones, timestamp frameworks and new conflict policies.

The [shared editor contract](../../platform/requirements/agent-settings-parity.md)
owns acknowledgement correlation, draft preservation and revision recovery.
The [permission contract](permission-control-integrity.md) remains unchanged.

## Related contracts

- [Agent system ownership](../README.md)
- [Creation entry points](settings-profile-layout.md)
- [Settings interface parity](../../platform/requirements/agent-settings-parity.md)
- [Creation catalogue design](../system-design/creation-catalogue.md)
