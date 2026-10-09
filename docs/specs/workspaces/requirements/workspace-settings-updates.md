---
status: active
system: workspaces
created: 2026-10-07
owners:
  - kandev
---

# Workspace settings update requirements

## Overview

Overlapping workspace renames, default selections, and idle-policy saves must
retain each successful disjoint edit. The workspace system owns this contract
because it owns persisted workspace values and the current workspace catalogue
consumed by settings and navigation. Settings presentation, executor
suspension, and organization reach consume those values and keep their own
contracts. The existing workspace catalog has no capability that owns partial
settings persistence; this pair supplies that narrow contract. The client
publication boundaries below preserve catalogue choices during a settings save
and successful Add Workspace creation without changing that persistence contract.

## Terms and boundary

An **ordinary update** supplies optional workspace settings through an existing
request-driven update surface. A supplied field expresses intent; omission
preserves its current value. JSON null currently means omission. A blank default
selection means clear, whereas an empty name or description remains a supplied
empty string. An **exact update** also requires a previously observed workspace
timestamp to match at persistence.

The guarantee covers updates through that settings owner. Deliberate complete
workspace writes and independent ownership, placement-backfill, bootstrap, and
counter operations keep their own contracts. A subsequent deliberate complete
write is not required to preserve an earlier partial edit.

## Requirements

### REQ-WORKSPACES-SETTINGS-UPDATES-001: Preserve partial settings intent

**Intent:** Saving a workspace setting must not restore omitted settings from an
earlier observation of the workspace.

#### Acceptance criteria

- **AC-WORKSPACES-SETTINGS-UPDATES-001.1:** When independent ordinary updates
  supply disjoint settings and both succeed, the workspace shall retain both
  edits in either write order. This includes a rename versus idle enabled/timeout
  edit and default selections versus other settings.
- **AC-WORKSPACES-SETTINGS-UPDATES-001.2:** An ordinary update shall preserve
  omitted name, description, placement, four default selections, and both idle
  settings. Supplied empty name or description shall remain empty; supplied
  blank or whitespace-only default selections shall clear that default;
  nonblank default selections shall retain existing whitespace normalization.
  Explicit false shall disable idle suspension without resetting the timeout.
  JSON null and an empty request shall preserve all setting values.
- **AC-WORKSPACES-SETTINGS-UPDATES-001.3:** A mixed update shall apply all its
  supported supplied fields together. Two successful updates to the same scalar
  field shall leave the value of the later write. Untouched absent defaults
  shall retain their stored absence, and newly created workspaces shall retain
  disabled idle suspension and the 120-minute default timeout.
- **AC-WORKSPACES-SETTINGS-UPDATES-001.4:** Existing manage authorization and
  unit-move admission shall continue to apply. A valid supplied destination
  shall use the existing namespace and permission checks. Omission shall never
  restore an old placement, including after an admitted move. Empty and
  unchanged destinations shall retain their existing no-move meaning. The
  existing transport support for placement shall remain unchanged.
- **AC-WORKSPACES-SETTINGS-UPDATES-001.5:** An exact update shall succeed only
  when its expected timestamp still matches at the write boundary. A changed
  timestamp, including an intervening disjoint settings save, shall cause the
  existing conflict outcome without changes by the rejected request.
- **AC-WORKSPACES-SETTINGS-UPDATES-001.6:** Invalid nonpositive timeouts,
  unauthorized updates, invalid unit moves, missing workspaces, failed exact
  matches, and failed or cancelled-before-write persistence shall produce no
  successful update event or partial setting changes from that request.
- **AC-WORKSPACES-SETTINGS-UPDATES-001.7:** REST and WebSocket settings updates
  shall preserve optional presence and the existing response shape. A successful
  response and its workspace update event shall project the persisted row
  observed for that write, including omitted fields already changed before its
  mutation. An empty ordinary request shall retain its existing successful
  timestamp-refresh and update-event behavior. Existing event timestamp
  formatting shall remain unchanged.
- **AC-WORKSPACES-SETTINGS-UPDATES-001.8:** Desktop and phone users shall receive
  the same settings persistence outcome through their existing save flows.
  Intentional complete-write callers and exact administration consumers shall
  retain their established behavior and error contracts.

### REQ-WORKSPACES-SETTINGS-UPDATES-002: Preserve current catalogue during settings save

**Intent:** A successful settings save shall preserve independent workspace
choices already visible when the save is acknowledged.

The linked catalogue-preservation work order implements this client boundary.
Requirement 001 and all its backend presence, admission, exact-update, response,
and event contracts remain active and unchanged.

#### Acceptance criteria

- **AC-WORKSPACES-SETTINGS-UPDATES-002.1:** When another workspace is added,
  updated, or removed while a workspace settings save is pending, successful
  acknowledgement shall preserve that current membership, order, and other
  workspace's current values. It shall neither remove an added choice, restore
  an earlier value, nor resurrect a removed choice.
- **AC-WORKSPACES-SETTINGS-UPDATES-002.2:** The real workspace switcher shall
  expose those current choices and names after acknowledgement while showing
  the accepted target settings. Selecting a retained choice after an ordinary
  successful save shall keep the existing workspace selection and navigation
  behavior. This outcome shall apply to desktop and phone users through their
  existing shared state and controls.
- **AC-WORKSPACES-SETTINGS-UPDATES-002.3:** A successful settings acknowledgement
  shall preserve the target's current metadata outside the existing accepted
  settings projection, including description, ownership, caller scopes,
  placement, configuration-agent default, and creation/update timestamps. It
  shall preserve the current active workspace identity and its selection
  revision. It shall retain the accepted name, executor/environment/agent
  defaults, and idle-policy projection with existing absence/default behavior.
- **AC-WORKSPACES-SETTINGS-UPDATES-002.4:** Ordinary successful saves without an
  intervening catalogue change shall keep existing accepted values, saved
  baseline, contributor completion, and dirty-state behavior. A pristine form
  shall issue no save. Newer unsaved edits made during an accepted save shall
  remain unsaved and shall retain the existing protection against leaving.
- **AC-WORKSPACES-SETTINGS-UPDATES-002.5:** A rejected save shall publish no
  accepted settings or baseline from that request. Current catalogue changes
  and a newer unsaved draft shall remain available; the failed contributor,
  existing error feedback, and dirty-route protection shall remain effective.
- **AC-WORKSPACES-SETTINGS-UPDATES-002.6:** Settings saves shall retain existing
  changed-field request presence, trimmed name, default clearing, explicit
  false, numeric idle timeout, validation, and manage-scope behavior. Omitted
  settings shall not become supplied values because the catalogue changed.
  Backend partial-field intent guarantees in requirement 001 remain intact.

### REQ-WORKSPACES-SETTINGS-UPDATES-003: Preserve current catalogue during creation

**Intent:** A successful Add Workspace acknowledgement shall make the accepted
workspace available without discarding independent workspace choices or changes
already visible while creation was pending.

This extends only creation publication in Settings. Requirements 001 and 002
retain their backend field-presence and merged settings-Save contracts.

#### Acceptance criteria

- **AC-WORKSPACES-SETTINGS-UPDATES-003.1:** When another workspace is added,
  updated, or removed while Add Workspace is pending, successful acknowledgement
  shall preserve every other workspace's current membership, relative order,
  and descriptor values. It shall neither remove an added choice, restore an
  earlier value, nor resurrect a removed choice. Independent notifications
  received after acknowledgement shall retain their existing effect.
- **AC-WORKSPACES-SETTINGS-UPDATES-003.2:** Acknowledgement shall expose the
  accepted workspace exactly once, including when its creation notification
  arrived first. It shall retain the accepted descriptor's name, description,
  ownership, placement, caller role/scopes, member count, defaults, idle policy,
  workspace kind, and timestamps with existing absent-value behavior. The
  accepted workspace shall occupy the existing creation position, preserving
  the relative order of other workspaces.
- **AC-WORKSPACES-SETTINGS-UPDATES-003.3:** The real workspace picker and Settings
  management list shall retain current choices and names and show one accepted
  workspace after acknowledgement. Current active identity and selection
  revision shall survive, including a selection made while creation was pending.
  If there is no active identity, ordinary creation shall retain the existing
  first-workspace selection behavior. Choosing a retained workspace shall use
  the existing selection and navigation behavior. Desktop and phone users shall
  receive this outcome through the shared catalogue.
- **AC-WORKSPACES-SETTINGS-UPDATES-003.4:** Ordinary and initially empty catalogue
  creation shall retain the trimmed-name request and successful form clearing
  and closing. A blank name shall issue no creation request. Acknowledgement
  shall publish into the initiating view's catalogue without changing another
  independent view's store.
- **AC-WORKSPACES-SETTINGS-UPDATES-003.5:** Rejected creation shall publish no
  accepted workspace from that request. Independently notified catalogue
  changes, current selection, entered name, open form, and existing request
  error feedback shall remain available.

## Out of scope

- New fields, transport capabilities, revision tokens, schema changes, settings
  controls, or hierarchy/runtime behavior. Client-store changes beyond the
  settings and Add Workspace acknowledgement publication in requirements 002
  and 003 are excluded.
- Changes to reach policy, authentication, legacy visibility, or task numbering.
- Universal serialization with unrelated workspace writers.
- Global event order, target-settings revision arbitration, guaranteed response
  freshness, or a new guarantee that clients reject stale target projections.
- Changes to delete/placement routes, other creation or save callers, or
  creation persistence; catalogue loss is not backend workspace deletion.
- Suspension/recovery behavior owned by the executor system.

## References

- [System design](../system-design/workspace-settings-updates.md).
- [Organization units](org-units.md).
- [Idle runtime parking](../../executors/requirements/idle-runtime-parking.md).
- [Settings manual save](../../ui/requirements/settings-manual-save.md).
- [Backend implementation plan](../../../plans/preserve-workspace-settings-updates/plan.md).
- [Catalogue preservation plan](../../../plans/workspace-save-catalogue-preservation/plan.md).
- [Creation catalogue preservation plan](../../../plans/workspace-create-catalogue-preservation/plan.md).
