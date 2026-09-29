---
id: coordinator-spend
title: Cost against a declared ceiling
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
---

# Cost against a declared ceiling Requirements

## Overview

A coordinator's turns cost money whether or not anything needs doing. Phase 3
measures what each coordinator's own sessions have cost, compares it with a
ceiling a manager declares, and stops unattended turns at that ceiling.
Attended turns are a manager's own decision and are never stopped by it.

## Terminology

- **Ceiling:** a per-coordinator amount in USD per rolling 24 hours, declared
  by a manager. It is optional until autonomy is turned on
  ([wake](wake.md#req-coordinator-wake-004-autonomy-setting-and-admission)).
- **Coordinator spend:** the sum of the recorded cost of every usage event of
  every conversation task the coordinator has ever had (current or archived),
  attended and unattended alike, in the window `[now - 24h, now)`.
- **Unpriced:** a usage event whose cost source is `unpriced`, so its cost is
  unknown and recorded as zero.
- **Measurable:** the spend read succeeded and the window holds no unpriced
  event.
- Other terms are defined in [wake](wake.md#terminology).

## Mockup

The source mockup (`mockup-v2.1`, outside this repository) shows a SPEND line
with the coordinator's cost per day against a ceiling "declared, not
enforced". Phase 3 enforces it for unattended turns; the plan's ASCII previews
UI-01 and UI-04 are the reference.

## Requirements

### REQ-COORDINATOR-SPEND-001: Declaring a ceiling

**Intent:** A manager states what one coordinator may cost.

#### Acceptance criteria

- **AC-COORDINATOR-SPEND-001.1:** A manager shall be able to set a
  coordinator's ceiling to an amount from 0.01 to 10000.00 USD with at most two
  decimals, or clear it. Any other value shall be refused with 400 naming
  `cost_ceiling`, changing nothing.
- **AC-COORDINATOR-SPEND-001.2:** Clearing the ceiling while autonomy is on
  shall be refused with 400 naming `cost_ceiling`.
- **AC-COORDINATOR-SPEND-001.3:** The ceiling shall be changed through the
  coordinator's PATCH route; changing it shall neither change
  `config_revision` nor replace the conversation, and a reader or a
  coordinator principal shall be refused.

### REQ-COORDINATOR-SPEND-002: Measuring spend

**Intent:** Spend is measured from the coordinator's own sessions, from the
same usage ledger the task cost display uses.

#### Acceptance criteria

- **AC-COORDINATOR-SPEND-002.1:** The system shall compute coordinator spend
  over every conversation task the coordinator has had, including tasks
  archived by a conversation reset, and over no other task.
- **AC-COORDINATOR-SPEND-002.2:** When the window holds an unpriced event, or
  the read fails, the spend shall not be measurable, and admission shall hold
  unattended turns with reason `spend_unmeasured`. A failed read of the
  7-day mean shall not make spend unmeasurable.
- **AC-COORDINATOR-SPEND-002.3:** Each unattended turn shall record its own
  cost, the sum of the priced usage events the conversation session recorded
  for that turn, including events recorded after the turn ended, and no event
  of another turn on the same session.

### REQ-COORDINATOR-SPEND-003: Stopping at the ceiling

**Intent:** Unattended turns stop at the ceiling, with overshoot bounded by
one usage report, or by one backstop period when a notification is missed.
The bound depends on the agent honouring a cancel; when it does not, the
failure is shown to the manager rather than hidden.

#### Acceptance criteria

- **AC-COORDINATOR-SPEND-003.1:** When coordinator spend is at or above the
  ceiling, admission shall start no unattended turn, with hold reason
  `ceiling_reached`, until spend falls below the ceiling as the window moves
  or a manager raises the ceiling.
- **AC-COORDINATOR-SPEND-003.2:** When a usage event recorded during an open
  unattended turn brings spend to or above the ceiling, or leaves spend not
  measurable, the system shall
  cancel that turn as a manager's Stop would, and record its outcome as
  `stopped_at_ceiling`. The backstop pass shall apply the same check, so a
  missed usage notification delays the stop by at most one backstop period.
  When the cancel fails, the turn shall stay open and the cancel shall be
  retried at the next usage notification or backstop pass whatever the spend
  then reads; when the unattended turn has already ended, or the session is
  running another turn, nothing shall be cancelled.
- **AC-COORDINATOR-SPEND-003.3:** An attended turn shall never be cancelled or
  refused because of the ceiling, including a manager message that starts on
  the session between the ceiling check and the cancel.
- **AC-COORDINATOR-SPEND-003.4:** While a ceiling stop has been requested for
  more than five minutes and the unattended turn is still open, the autonomy
  strip shall show that the stop is not confirmed and offer Stop, and each
  failed cancel shall be counted in a metric.

### REQ-COORDINATOR-SPEND-004: Showing spend

**Intent:** The manager sees cost where they decide about autonomy.

Mockup:

- Plan UI-01: the spend part of the autonomy strip.
- Plan UI-04: the ceiling field and the spend lines in settings.

#### Acceptance criteria

- **AC-COORDINATOR-SPEND-004.1:** The Autonomy section of the settings and
  the autonomy strip shall show spend in the last 24 hours, the ceiling, and a
  pill reading Inside or Over; with no ceiling the pill reads "No ceiling".
- **AC-COORDINATOR-SPEND-004.2:** The Autonomy section shall also show the
  mean daily spend over the last 7 days and the cost of the last unattended
  turn.
- **AC-COORDINATOR-SPEND-004.3:** When spend is not measurable, both places
  shall say "Spend unknown: some usage is unpriced" or "Spend unavailable" for
  a failed read, in place of the amount.

## Out of scope

- Workspace-wide or instance-wide coordinator budgets, and Office budgets.
  Office's budget policies are unchanged and do not apply to coordinators.
- Stopping inside a single model request. The ceiling is checked when usage is
  recorded.
- Currency other than USD, and calendar-day windows.
- Forecasting spend.
