---
status: active
system: executors
created: 2026-10-10
owners:
  - kandev
---

# Executor creation catalogue publication requirements

## Overview

Creating an executor must retain the independently changing executor catalogue
used by task and subtask choices. Executors owns this contract because it owns
executor membership and nested profiles. This capability covers acceptance of
an executor owner, distinct from the [profile editor's](profile-editor.md)
creation of a profile within an existing owner.

## Terminology

- **Accepted executor:** The executor descriptor returned by a successful
  creation request.
- **Current catalogue:** Executor owners and their nested profiles visible
  immediately before the accepted descriptor is applied.
- **Unrelated entry:** Any owner other than the accepted executor, and every
  profile belonging to those other owners.

## Requirements

### REQ-EXECUTORS-CREATE-CATALOGUE-001: Preserve current choices on executor creation

**Intent:** Successful executor creation shall add the accepted executor without
undoing independent catalogue changes received during creation.

#### Acceptance criteria

- **AC-EXECUTORS-CREATE-CATALOGUE-001.1:** When executor creation succeeds,
  every unrelated current owner and profile shall retain its membership,
  relative order and values. Additions and updates received while creating
  shall remain present with their current values; owners and profiles removed
  meanwhile shall remain absent, including a catalogue containing both
  surviving and removed entries.
- **AC-EXECUTORS-CREATE-CATALOGUE-001.2:** Applying successful creation shall
  leave exactly one descriptor for the accepted executor, using its accepted
  values at the existing creation position after the unrelated current owners.
  This shall also hold when an owner-creation notification has already made
  that descriptor visible before acceptance. Creation shall not imply that a
  profile was also created.
- **AC-EXECUTORS-CREATE-CATALOGUE-001.3:** Desktop and phone task and subtask
  executor choices shall continue to expose retained eligible profiles with
  their current names and owner metadata. Newly received eligible choices
  shall remain available after creation; removed choices shall remain absent.
  Existing eligibility and selection behavior shall retain their meaning.
- **AC-EXECUTORS-CREATE-CATALOGUE-001.4:** Creation shall retain its existing
  supported types, submitted name, status and configuration, transport choice,
  pending control, successful return to the executor settings listing and
  final pending-state cleanup. Rejected creation shall publish no accepted
  descriptor or successful return and shall retain its existing error policy.

## Related contracts

The [task selection default](../../tasks/requirements/task-create-executor-default.md)
owns selection. The [profile editor](profile-editor.md) owns profile creation,
editing, policy saves, connection saves and profile-card refresh. Their
lifecycles and publication rules are not extended by this requirement.

## Out of scope

- Backend APIs, persistence, registry, executor providers and runtime launch.
- Profile creation for the accepted executor or preservation/arbitration of
  that executor's profiles received before its acknowledgement. That adjacent
  scenario has no qualified causal evidence in this package.
- Same-executor later-update ordering, timestamps, global catalogue revisions,
  event-writer changes, cancellation and request-lifetime redesign.
- New validation, selection policy, error UI, catch, toast or retry behavior;
  form, layout, copy, touch or settings navigation redesign.
- Other executor/profile creation pages or other catalogue writers.

## Implementation plans

- [Preserve live choices during executor creation](../../../plans/executor-create-catalogue-preservation/plan.md)
