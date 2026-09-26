---
status: draft
system: costs
created: 2026-09-24
owners:
  - kandev
---

# Built-in conversation usage requirements

## Overview

Users can inspect recorded tokens and cost in chat without a session-cost plugin or Office.
This capability extends the existing [task usage ledger](../../task-cost-ledger/spec.md).
The costs system owns accounting, provenance, aggregation, and its conversation display.
The [native Codex integration](../../agents/requirements/codex-app-server.md) supplies observations.

## Terms

- **Last response:** The latest attributable upstream model completion in a turn.
- **Turn usage:** Recorded work for that user turn, with direct and child work separated.
- **Session usage:** New work recorded for the Kandev session, excluding inherited fork history.
- **Calculated cost:** Tokens priced by a versioned rate catalogue, not an invoice amount.
- **Provider estimate:** A provider's explicitly estimated amount, with its own scope and currency.

## Requirements

### REQ-COSTS-CONVERSATION-USAGE-001: Attributed token measurements

#### Acceptance criteria

- **AC-COSTS-CONVERSATION-USAGE-001.1:** The system shall distinguish response, turn, session, and context-window usage. It shall never label last-response usage as whole-turn usage.
- **AC-COSTS-CONVERSATION-USAGE-001.2:** Repeated observations, resume replay, and inherited fork history shall not increase recorded usage twice.
- **AC-COSTS-CONVERSATION-USAGE-001.3:** Cached input and reasoning output shall appear as breakdowns when the provider includes them in input/output totals. Totals shall not count those tokens twice.
- **AC-COSTS-CONVERSATION-USAGE-001.4:** Missing, partial, estimated, pending, and measured-zero usage shall remain distinguishable after reload.
- **AC-COSTS-CONVERSATION-USAGE-001.5:** Child usage shall retain its own thread and turn identity. Late child usage shall update the originating work, never the currently active unrelated turn.

### REQ-COSTS-CONVERSATION-USAGE-002: Honest cost provenance

#### Acceptance criteria

- **AC-COSTS-CONVERSATION-USAGE-002.1:** Cost displays shall distinguish provider-reported monetary values, provider estimates, calculated list-price estimates, and unavailable prices.
- **AC-COSTS-CONVERSATION-USAGE-002.2:** A provider amount without established monetary units shall not appear as USD. Missing prices shall not appear as free usage.
- **AC-COSTS-CONVERSATION-USAGE-002.3:** A thread-level provider estimate shall not be divided into invented response or turn costs, or added to calculated usage totals.
- **AC-COSTS-CONVERSATION-USAGE-002.4:** Calculated costs shall retain the model and pricing snapshot used. Subscription usage estimates shall not claim to be extra invoice charges.

### REQ-COSTS-CONVERSATION-USAGE-003: Built-in chat display

#### Acceptance criteria

- **AC-COSTS-CONVERSATION-USAGE-003.1:** A turn's usage control shall show its tokens and cost, with separate last-response detail when available.
- **AC-COSTS-CONVERSATION-USAGE-003.2:** A session usage control shall show recorded totals without installing a plugin or enabling Office.
- **AC-COSTS-CONVERSATION-USAGE-003.3:** Desktop and phone users shall reach the same detail and provenance. Phone disclosure shall use a touch-accessible drawer with one scroll owner.
- **AC-COSTS-CONVERSATION-USAGE-003.4:** Delayed accounting and fetch errors shall appear as pending or unavailable states. They shall not block the agent or erase known values.

### REQ-COSTS-CONVERSATION-USAGE-004: Compatible durable accounting

#### Acceptance criteria

- **AC-COSTS-CONVERSATION-USAGE-004.1:** Existing ledger rows, ACP totals, task totals, Office budgets, and plugin readers shall retain their established meaning.
- **AC-COSTS-CONVERSATION-USAGE-004.2:** Turn and response reads shall enforce task/session ownership. Unknown ownership shall never attach usage to another session.
- **AC-COSTS-CONVERSATION-USAGE-004.3:** Accounting shall preserve recorded work through restart, including interrupted turns. Incomplete recovery shall remain visible rather than invent missing measurements.

## Out of scope

- Invoices, billing reconciliation, subscription purchase, and pricing claims for undocumented amounts.
- Retroactive reconstruction of missing historical response measurements.
- Replacement of third-party cost plugins or their independent features.

## Implementation plans

- [Native Codex support](../../../plans/codex-app-server/plan.md)
