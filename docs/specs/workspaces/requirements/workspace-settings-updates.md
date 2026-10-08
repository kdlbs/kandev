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
because it owns persisted workspace values. Settings presentation, executor
suspension, and organization reach consume those values and keep their own
contracts. The existing workspace catalog has no capability that owns partial
settings persistence; this pair supplies that narrow missing contract.

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

## Out of scope

- New fields, transport capabilities, revision tokens, schema changes, settings
  controls, client stores, or hierarchy/runtime behavior.
- Changes to reach policy, authentication, legacy visibility, or task numbering.
- Universal serialization with unrelated workspace writers.
- Global event order, guaranteed freshness when a response is received, or a
  new guarantee that clients reject stale projections.
- Suspension/recovery behavior owned by the executor system.

## References

- [System design](../system-design/workspace-settings-updates.md).
- [Organization units](org-units.md).
- [Idle runtime parking](../../executors/requirements/idle-runtime-parking.md).
- [Settings manual save](../../ui/requirements/settings-manual-save.md).
- [Implementation plan](../../../plans/preserve-workspace-settings-updates/plan.md).
