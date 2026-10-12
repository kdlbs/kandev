---
status: active
system: executors
created: 2026-10-10
owners:
  - kandev
---

# Initial executor catalogue loading requirements

## Overview

Users opening task creation or a new subtask need eligible executor profiles
to remain usable when the initial catalogue response arrives after live
updates. The executor system owns executor/profile membership and values;
task selectors consume that catalogue. This capability owns initial loading,
an independent read lifetime from the existing
[profile editor and profile-card refresh contract](profile-editor.md).

## Terminology

- **Initial read:** A catalogue read admitted while the catalogue is empty and
  initial loading has not settled.
- **Live catalogue transition:** Publication of at least one executor after
  an initial read begins. Subsequent owner/profile changes, removals, or
  restoration to an empty catalogue do not erase that observation.
- **Current choices:** Eligible profiles derived from current owner/profile
  membership, order and values, using existing availability rules.

## Requirements

### REQ-EXECUTORS-INITIAL-CATALOGUE-001: Usable choices during initial loading

**Intent:** Initial loading shall populate an untouched catalogue without
undoing live executor choices received while reading.

#### Acceptance criteria

- **AC-EXECUTORS-INITIAL-CATALOGUE-001.1:** When an initial read succeeds
  without a live catalogue transition, the returned executors and profiles,
  including an empty result, shall populate the catalogue and supply usable
  eligible task and subtask choices.
- **AC-EXECUTORS-INITIAL-CATALOGUE-001.2:** When a live catalogue transition
  occurs during an initial read, settlement shall retain the entire current
  catalogue. Every current executor and profile shall retain its membership,
  order and values. Removed entries shall remain absent, including in a mixed
  catalogue and when an added executor is later removed before settlement.
  A transition later restored to its original state shall still take precedence.
- **AC-EXECUTORS-INITIAL-CATALOGUE-001.3:** A live eligible profile selected in
  the task-creation or subtask draft while the initial read is pending shall
  remain selected and usable after settlement while its owner and profile
  remain available. The existing selector shall show the current profile and
  owner labels instead of reverting to its placeholder. Desktop and phone
  shall receive the same catalogue outcome and existing eligibility behavior.
- **AC-EXECUTORS-INITIAL-CATALOGUE-001.4:** Disabled consumers and consumers
  whose initial loading has settled shall not initiate initial reads. An
  enabled consumer with a populated catalogue shall preserve those choices,
  skip the initial read and settle initial loading through the existing fast
  path. A disabled consumer later enabled against an empty unsettled catalogue
  shall retain ordinary initial loading.
- **AC-EXECUTORS-INITIAL-CATALOGUE-001.5:** A failed initial read shall retain
  current live choices, or leave an untouched empty catalogue empty, and shall
  settle initial loading with the existing silent failure behavior. No retry,
  notification, or newly reported error shall be introduced by this correction.
- **AC-EXECUTORS-INITIAL-CATALOGUE-001.6:** Changes to unrelated settings and
  replacement of an empty catalogue with another empty catalogue shall not
  prevent an uncontested initial response from populating choices. Initial
  read settlement shall not change agent discovery, profile availability
  policy, selection defaults, or task submission behavior.

## Compatibility and limits

Initial loading retains its existing completion semantics; live arrival can
settle the populated fast path before an admitted read completes. Preserving a
contested catalogue can defer response-only entries until an existing later
load or live publication. This contract adds no automatic reconciliation and
does not promise that an initial read discovers all unobserved server changes.
Separate admitted reads receive the same local publication rule; no shared
request deduplication or newest-read policy is introduced.

## Out of scope

- Profile-card refresh, creation/save/delete writers and server arbitration.
- Global timestamps, versions, resource frameworks or catalogue freshness.
- Authentication scope, request lifetime, backend boot payloads, events and
  agent-list ownership redesign.
- Layout, copy, interaction, navigation, scrolling, touch or breakpoint changes.

## Implementation plans

- [Preserve choices during initial loading](../../../plans/executor-initial-catalogue-preservation/plan.md)
