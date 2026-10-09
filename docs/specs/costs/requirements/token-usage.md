---
status: active
system: costs
created: 2026-09-13
owners:
  - kandev
---

# Token Usage Requirements

## Overview

Kandev shows saved token usage and cost across tasks, sessions, models, and dates.
The costs system owns this contract because it owns accounting and cost projections.
Collectors supply measurements. The native statistics view remains available without a collector plugin.

The user selected core persistence and native statistics on 2026-09-12.
The existing [task ledger](../../task-cost-ledger/spec.md) retains its event-recording contract.
This capability adds source-aware measurements and a statistics projection over available data.

## Terminology

- **Measurement:** Token counts and cost for an identified scope of work.
- **Source:** The collector or native integration that supplies a measurement.
- **Coverage:** The work and time interval that a measurement represents.
- **Saved cost:** The latest accepted cost for the selected session.
- **Estimated cost:** A usage-based equivalent that does not establish an invoice or subscription charge.

## Requirements

### REQ-COSTS-TOKEN-USAGE-001: Durable source-aware accounting

**Intent:** Consumers can use measurements from different sources without duplicate charges.

#### Acceptance criteria

- **AC-COSTS-TOKEN-USAGE-001.1:** Kandev shall accept authorized session measurements independently of the Session Cost plugin and the Office feature.
- **AC-COSTS-TOKEN-USAGE-001.2:** Repeated measurements shall not increase totals twice. Older revisions shall not replace newer measurements. Explicit newer corrections shall replace prior values.
- **AC-COSTS-TOKEN-USAGE-001.3:** Overlapping sources shall not contribute twice. Ambiguous coverage shall remain visibly incomplete rather than produce an invented combined total.
- **AC-COSTS-TOKEN-USAGE-001.4:** Kandev shall preserve unknown values separately from measured zero, token categories, source provenance, and estimated versus reported cost.
- **AC-COSTS-TOKEN-USAGE-001.5:** Plugin disablement, uninstall, or restart shall not remove saved usage. Task deletion shall remove its usage. Session pruning shall retain task-level history.
- **AC-COSTS-TOKEN-USAGE-001.6:** Unauthorized reads and writes shall fail without exposing measurements or changing totals. A collector shall not impersonate another source.
- **AC-COSTS-TOKEN-USAGE-001.7:** Existing native ledger records shall remain intact. The new statistics view shall use available native records without adding overlapping external measurements.

### REQ-COSTS-TOKEN-USAGE-002: Optional collection

**Intent:** Operators can collect usage without a process for each task or browser tab.

#### Acceptance criteria

- **AC-COSTS-TOKEN-USAGE-002.1:** Scheduled collection shall default to disabled. Operators shall control its interval in minutes, with a five-minute default and one-minute minimum.
- **AC-COSTS-TOKEN-USAGE-002.2:** When collection is disabled, the plugin shall preserve manual calculations without scheduled scans or new persisted measurements.
- **AC-COSTS-TOKEN-USAGE-002.3:** Concurrent scheduled and manual refreshes shall share at most one active tokscale invocation per plugin instance.
- **AC-COSTS-TOKEN-USAGE-002.4:** Without active sessions, pending final collection, or explicit historical import, the plugin shall perform no scheduled tokscale invocation.
- **AC-COSTS-TOKEN-USAGE-002.5:** Collection shall cover eligible sessions beyond the focused tab, including sessions that finish between polls and delayed final transcript writes.
- **AC-COSTS-TOKEN-USAGE-002.6:** Restart recovery and historical import shall resume bounded work without duplicate totals. Failures shall retain prior measurements and expose stale status.
- **AC-COSTS-TOKEN-USAGE-002.7:** Sessions with inaccessible or ambiguous transcripts shall show missing coverage. Collection shall not attribute unrelated local transcript usage to Kandev tasks.

### REQ-COSTS-TOKEN-USAGE-003: Saved session cost during refresh

**Intent:** A refresh does not hide information that Kandev already knows.

#### Acceptance criteria

- **AC-COSTS-TOKEN-USAGE-003.1:** With saved session usage, the plugin shall show saved values and their timestamp during refresh.
- **AC-COSTS-TOKEN-USAGE-003.2:** Without saved usage, the plugin shall show a calculation placeholder. A successful refresh shall replace it with current values.
- **AC-COSTS-TOKEN-USAGE-003.3:** A failed refresh shall retain saved values and expose retry status. A session switch shall reject responses for the previous session.
- **AC-COSTS-TOKEN-USAGE-003.4:** Keyboard and touch users shall access the same values, freshness information, and refresh action without hover.

### REQ-COSTS-TOKEN-USAGE-004: Native usage statistics

**Intent:** Users can inspect saved usage independently of the source that supplied it.

#### Acceptance criteria

- **AC-COSTS-TOKEN-USAGE-004.1:** The desktop statistics topbar shall place Overview and Token Usage navigation to the left of the date controls.
- **AC-COSTS-TOKEN-USAGE-004.2:** Token Usage shall support direct navigation and preserve the selected workspace and date range across view changes and browser history.
- **AC-COSTS-TOKEN-USAGE-004.3:** The view shall show available cost, token categories, trends, model breakdown, and task/session details. Copy Stats shall describe the selected view.
- **AC-COSTS-TOKEN-USAGE-004.4:** Date filters shall use consumption coverage rather than collection time. Undated totals shall remain separate from dated charts and range totals.
- **AC-COSTS-TOKEN-USAGE-004.5:** The view shall distinguish measured zero, missing values, partial coverage, estimated cost, and stale data. Plugin removal shall not hide existing history.
- **AC-COSTS-TOKEN-USAGE-004.6:** Phones shall provide the same navigation, ranges, details, and copy action without document horizontal scrolling or hover-only actions.
- **AC-COSTS-TOKEN-USAGE-004.7:** Phone controls shall have touch targets of at least 44px. All controls shall have accessible names and localized labels.

- **AC-COSTS-TOKEN-USAGE-004.8:** The model table shall group by model and provider. It shall show token categories, total tokens, total cost, and effective cost per million tokens.
- **AC-COSTS-TOKEN-USAGE-004.9:** Effective cost per million shall equal total cost divided by matching total tokens, multiplied by one million. It shall not average individual rates.
- **AC-COSTS-TOKEN-USAGE-004.10:** Daily and monthly tables shall show token categories, total tokens, total cost, and effective cost per million for each period.
- **AC-COSTS-TOKEN-USAGE-004.11:** Table sorting and totals shall apply to the complete filtered result. A partial month shall include only dates within the selected range.
- **AC-COSTS-TOKEN-USAGE-004.12:** Missing providers shall appear as unknown. Zero denominators or incomplete matched coverage shall show an unavailable rate. Phones shall expose every table metric.

### REQ-COSTS-TOKEN-USAGE-005: Empty and error states

**Intent:** Users receive setup guidance only when the selected scope lacks measurements.

#### Acceptance criteria

- **AC-COSTS-TOKEN-USAGE-005.1:** With no saved usage in the authorized scope, the view shall explain Session Cost installation and collection enablement.
- **AC-COSTS-TOKEN-USAGE-005.2:** With Session Cost installed, the empty state shall link to its settings. Without it, the action shall open its installation flow.
- **AC-COSTS-TOKEN-USAGE-005.3:** An empty date range with history elsewhere shall show a range-specific empty state, not an installation prompt.
- **AC-COSTS-TOKEN-USAGE-005.4:** A read failure shall show an error and retry action, not an empty-data or installation state.
- **AC-COSTS-TOKEN-USAGE-005.5:** Existing native measurements shall suppress installation guidance, even when no plugin measurement exists.

## Out of scope

- New ACP or Codex app-server collectors, remote transcript transport, and provider billing reconciliation.
- Budget enforcement, subscription quotas, currency conversion, and Office ledger migration.
- A generic plugin slot for the native statistics page.
- Hourly reports, latency per token, and cache multipliers from the reference screenshots.

## Implementation Plans

- [Token Usage work package](../../../plans/token-usage/plan.md)
- [System design](../system-design/token-usage.md)
