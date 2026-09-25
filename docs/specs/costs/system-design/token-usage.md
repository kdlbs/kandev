---
status: draft
system: costs
requirements:
  - REQ-COSTS-TOKEN-USAGE-001
  - REQ-COSTS-TOKEN-USAGE-002
  - REQ-COSTS-TOKEN-USAGE-003
  - REQ-COSTS-TOKEN-USAGE-004
  - REQ-COSTS-TOKEN-USAGE-005
---

# Token Usage System Design

## Purpose and boundaries

This design extends core accounting with external measurements and a native statistics projection.
The costs system owns measurement semantics. Task services supply ownership and session identity.
The plugin system supplies the authenticated Host connection. Session Cost owns tokscale execution and collection settings.

The [core projection decision](../../../decisions/2026-09-13-core-session-usage-projections.md) records the selected storage boundary.
New names in this document are proposed implementation symbols, not existing APIs.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-COSTS-TOKEN-USAGE-001 | Existing ledger boundary, Data and contracts, Source selection, Security |
| REQ-COSTS-TOKEN-USAGE-002 | Collector and recovery |
| REQ-COSTS-TOKEN-USAGE-003 | Saved session display |
| REQ-COSTS-TOKEN-USAGE-004 | Native statistics, Usage tables and effective rates, Time attribution |
| REQ-COSTS-TOKEN-USAGE-005 | Empty and error states |

## Existing ledger boundary

`internal/task/usage.Writer` records `TaskUsageEvent` rows in `task_usage_events`.
`internal/task/repository/sqlite/usage_events_schema.go` defines that table and its deletion rules.
`TaskUsageTotals` and `task_usage_handlers.go` expose event-based totals.
These boundaries follow the [task ledger contract](../../task-cost-ledger/spec.md).

The new service reads native events as one source candidate.
It never mutates ledger rows, increments existing session rollups, or adds cumulative external reports as ledger events.
Office budget calculations and existing task usage endpoints retain their current semantics.
No new native protocol collector belongs to this package.

## Components and responsibilities

| Boundary | Responsibility |
| --- | --- |
| `internal/analytics/service` | New `UsageService`: validation, attribution, source selection, writes and queries |
| `internal/analytics/repository` | Typed storage interface and transaction boundaries |
| `internal/analytics/repository/sqlite` | Portable SQLite/PostgreSQL schema and indexed queries |
| `internal/plugins` | Host capability checks and typed service adapter |
| `pkg/pluginsdk`, `proto/kandev/plugin/v1/plugin.proto` | Additive public DTOs and RPCs |
| Session Cost `server/plugin.go`, `server/tokscale.go` | Collection, transcript normalization and usage writes |
| Session Cost `ui/bundle.js` | Saved cost, refresh state and touch disclosure |
| `apps/web/app/stats` | Native Token Usage route and shared stats navigation |

The existing analytics repository is read-oriented. Its writer connection currently creates indexes.
The new usage repository introduces explicit write transactions without routing writes through HTTP handlers or plugin SQL.

## Data and contracts

### Core storage

Create `session_usage_measurements` for cumulative lifetime measurements.
Create `session_usage_buckets` for dated measurements.
Both contain these logical fields:

- Server-assigned source namespace and stable source record key.
- Stable usage identity, transcript identity, nullable session reference and mandatory task reference.
- Model, client/provider identity and source contract version.
- Source revision, payload digest, observed timestamp and successful collection timestamp.
- Nullable input, output, cache-read, cache-write, reasoning and normalized total counts.
- Nullable `cost_subcents`, currency and cost basis: estimated, reported, or unknown.
- Coverage classification, source timezone and attribution status.

Buckets also contain a half-open UTC interval and the original source-local date.
The uniqueness key includes source, stable usage identity, model, provider and coverage key.
Scope queries use task/workspace joins and indexes on session, task, model and interval.
Token counts and cost use checked 64-bit integers. Existing monetary units use 10,000 subcents per USD.
The tokscale adapter converts decimal amounts once, with explicit rounding and overflow rejection.

A single transaction validates and replaces a bounded batch.
An equal revision and equal digest returns unchanged. An equal revision with a different digest returns conflict.
A lower revision returns stale. A higher revision can correct totals downward.
The server does not infer revision order from process-local counters that reset after restart.
The plugin persists its revision checkpoint before submission and retries the same batch after an uncertain result.

Collection health uses a separate record with attempt time, success time and bounded error code.
It never replaces usage with zero after a failed command.
Plugin-private checkpoints can use Host state. Measurements always use core tables.

### Token semantics

Adapters normalize disjoint categories and preserve presence information.
They must not add cached tokens twice when a source includes them in input.
They must not add reasoning twice when a source includes it in output.
A known total can coexist with incomplete categories, with an explicit completeness flag.
The service rejects negative values, overflow, invalid intervals and inconsistent declared totals.
The native adapter preserves the ledger's stored total and its contract version.
It does not retroactively reinterpret historical ledger categories.

### Host surface

Add `Host.Usage()` with `UpsertBatch` and `ListSessionUsage` accessors.
Wire methods are `UpsertSessionUsage` and `ListSessionUsage`.
Capabilities are `api_write:session_usage` and `api_read:session_usage` under the shipped v1 model.
These new methods do not claim to implement the proposed exact-operation Host protocol.

Writes include a workspace, session targets, idempotency key, source revisions and typed records.
The Host assigns `plugin:<id>` provenance. Plugin payloads cannot set native source identity or source priority.
A batch has at most 200 records and one workspace. Invalid batches have no partial writes.
Results distinguish applied, unchanged, stale, conflict and invalid outcomes.
Read responses include saved values, chosen source, coverage, timestamps and collection health.
Pagination uses existing `Page` and `PageInfo` conventions.

`SessionFilter` gains additive session-ID and updated-since filters where the collector needs them.
The task service applies filters and pagination in SQL.
This replaces `fetchSessionsForFilter` task-by-task enumeration for broad session reads.
Existing filter intersections and workspace authorization remain intact.

### Browser API

Add an authenticated usage endpoint under the existing analytics route group.
It accepts workspace, range and timezone and returns summary, dated buckets and paginated breakdowns.
Grouping supports model/provider, day, month, task and session. Sorting and aggregate totals apply before pagination.
A separate all-history existence field applies to the same authorized scope, independent of the selected range.
The response distinguishes no data, partial data and unavailable data.
No user-facing API reads plugin_state or executes tokscale to construct statistics.

## Source selection

Selection uses stable work identity, not row count or the largest reported cost.
The initial external source is tokscale. The existing ledger supplies the native source candidate.
The server applies these rules independently to cost and token groups:

1. A complete measurement for the requested coverage takes precedence over a partial candidate.
2. For equivalent complete coverage, a native reported measurement takes precedence over an external estimate.
3. Otherwise, retain the existing selected source until a strictly better coverage candidate exists.
4. An initial tie uses a stable server-defined source order, then its latest accepted revision.
5. Unknown cost can use another source only for exactly the same coverage. The response identifies that cost source separately.

A session lifetime snapshot and its daily buckets are alternative representations, never additive rows.
Partial native and external totals are not added because their overlap is unknown.
The projection selects one candidate and marks incomplete coverage.
Disjoint intervals can combine only when their boundaries and work identities establish that they do not overlap.

If one external transcript maps to several Kandev sessions, the adapter does not duplicate its lifetime total.
Explicit disjoint execution coverage can partition it.
Without that evidence, external attribution remains unresolved and the native candidate remains available.
No source wins merely because it claims a reported cost in an untrusted payload.
Host-assigned source policy controls eligibility for native precedence.

## Time attribution

Occurrence time and observation time are different fields.
Native ledger buckets use their recorded event time, with the ledger's documented arrival-time limitation.
External daily buckets use the transcript date and timezone, not the time of the collection command.
Source-local midnight boundaries convert to UTC with daylight-saving rules.
Date requests never divide an aggregated source-day value proportionally across a different timezone boundary.
They retain the source bucket and disclose its timezone when finer attribution is unavailable.

The current tokscale session/model report has no dates.
Task 01 verifies the pinned 4.15.1 dated output through fixtures before the collector implementation.
The preferred path is one dated export for all eligible sessions.
The bounded fallback performs one source-local day query per queued sweep, with active dates before historical dates.
All such queries share the same subprocess lock and interval budget.
An undated lifetime report supports the popover and a separate all-time total, but never invents daily consumption.

Initial enablement collects active sessions. Historical import is an explicit, resumable action with visible progress.
This avoids scanning all history silently at enablement.

## Collector and recovery

Settings are `collect_statistics=false` and `collection_interval_minutes=5`, with a minimum interval of one minute.
Disabled collection retains manual calculation but does not persist new measurements.
The plugin can still read saved core usage while collection is disabled.

After `SetHost`, one worker owns the timer, pending sessions and subprocess lock.
Manual refresh joins the same in-flight report.
A compatible cached report satisfies other session requests without another command.
Different date requests queue behind the same worker.
The 120-second command timeout remains the upper bound for a single report.
A timeout cancels the child process tree, including any npx wrapper descendants.

Events mark sessions dirty. Bounded SQL reconciliation repairs missed events and restart gaps.
The collector reads canonical session state before attribution.
A final collection remains pending after completion and retries delayed transcript writes.
Retries use capped backoff and do not delay agent execution.
Successful reports update only changed measurements in batches.
An empty eligible set skips the command. Historical work runs only after an explicit import request.

The plugin persists pending final collections, source revisions and import cursors.
Settings restarts cancel the old worker before the new process starts collection.
An unavailable Host API produces an upgrade-required state without silently discarding intended writes.
Local transcript access is the supported first release scope.
No credential probing or arbitrary remote executor command belongs to this collector.

## Saved session display

The existing `sessionCost` path performs a calculation before it returns.
Split saved reads from refresh requests so a long command cannot block the initial saved response.
Use declared authenticated session actions for browser access and verify task/session ownership.

The popover reads saved usage immediately, then requests refresh separately.
Saved values remain visible with a progress indicator.
No saved measurement produces the calculation placeholder.
On success, enabled collection persists the result before authoritative readback.
Disabled collection shows the manual result without a DB write.
A failed refresh retains the saved value with retry status.
Request identity includes the session ID so late results cannot cross session tabs.

Fine-pointer users retain the popover. Touch users open an explicit drawer with the same view model.
The drawer uses the existing UI primitives and returns focus to its trigger.

## Native statistics

Add `/stats/token-usage` beside `/stats`.
`StatsPageClient` supplies a shared navigation shell with Overview and Token Usage before the date controls.
`stats-api.ts` gains a typed usage client, and the native route uses core analytics DTOs.
Range and workspace belong to route state. A changed request key cancels or ignores old responses.

Summary cards show cost and token categories. Charts show dated usage and cost.
Model/provider, daily, monthly and task/session tables use paginated queries.
Undated lifetime totals remain a separate summary with a coverage explanation.
Copy Stats includes only the selected scope, source basis and coverage.

### Phone composition

The existing `mobile-stats-nav.spec.ts` establishes Stats access through the navigation sheet.
The curated `kanban-with-preview.tsx` pattern supplies direct phone navigation instead of a squeezed split view.
The Token Usage page is a primary destination because users inspect multiple metrics and drill into sessions.

The phone header has two rows: view navigation, then a range picker and visible Copy action.
The body has summary cards, a trend, and one breakdown list with model, daily, monthly, task and session choices.
A detail row opens a focused route or full-height surface, not a wide desktop table.
One page body owns vertical scrolling. Fixed chrome and overlays respect dynamic viewport height and safe areas.
Controls use 44px touch targets. Fine-pointer desktop controls retain the standard 28px size.
The UI shares filters and data selectors across viewports, without changing stored desktop preferences.

## Usage tables and effective rates

The screenshot references establish dense numeric comparisons and explicit time tables as the intended outcome.
The native page retains its summary and trend, then provides Models, Daily, Monthly, Tasks and Sessions views.
Models is the default table. Daily and Monthly are tables, not only chart bucket settings.
The chart retains its own Cost/Tokens and Day/Week/Month controls.

### Columns and identity

Models columns are Model, Provider, Input, Output, Cache read, Cache write, Total tokens, Total cost and Cost/1M.
Daily and Monthly replace Model/Provider with Date or Month and keep the numeric columns.
Reasoning appears in row details when available. The normalized total counts reasoning only once.
Provider means the provider recorded for the usage, not an inferred vendor from a model name.
Collector provenance, such as tokscale, remains separate from provider identity.
Missing providers use an unknown group. Identical model labels from different providers remain separate rows.
A provider filter can narrow Models without losing that grouping identity.

Include provider in measurement and bucket identity so different providers cannot overwrite the same model's data.
The plugin adapter must preserve provider detail from the selected export.
A report that already merges providers cannot establish that detail. Such rows remain unknown until an unmerged export supplies it.
The pinned-version fixture must verify provider grouping as well as session/model/date support.

### Effective rate

The server computes the effective rate from the selected canonical records:

```text
cost_per_1m_usd = (sum(cost_subcents) / 10000) * 1000000 / sum(total_tokens)
```

The numerator and denominator cover the same records, period and currency.
Use checked decimal/rational arithmetic and round only the displayed value.
Never average model, session, daily or monthly row rates.
The total-token denominator includes normalized cache categories without duplicate input or reasoning tokens.
This is a blended historical rate, not a model's advertised input or output price.
Cost basis remains visible, including estimated cost for subscription usage.

An explicit zero cost with positive complete tokens produces a zero rate.
Zero tokens, unknown total tokens, unknown cost, or incomplete matched coverage produces a nullable rate and a reason.
The UI shows an unavailable marker with an accessible explanation, not zero or infinity.
Partially known cost and token sums remain visible as partial totals, but they do not produce a misleading rate.
Different currencies never share a rate. The first release supports USD without conversion.

### Aggregation and sorting

The service computes all breakdowns after source selection and before pagination.
API requests include grouping, an allowlisted sort field, direction and cursor.
Responses include numeric rows, full filtered totals, rate availability and period completeness.
The totals row describes all filtered records, not just the visible page.
Sorting uses raw numeric values and a stable identity tie-breaker, never formatted K/M/B strings.
Models defaults to descending total cost. Daily and Monthly default to newest period first.
Users can sort either period table by total cost, total tokens or effective rate.
Unknown values sort last in either direction.

Monthly rows aggregate the canonical dated buckets that intersect the selected range under the established timezone contract.
A month clipped by that range is labelled partial. It does not pull in other days from that month.
Monthly and daily totals reconcile over the same date range and dated coverage.
Undated lifetime totals remain in their separate summary and never enter either table.
Absent daily coverage does not create zero-spend days.
Copy Stats includes the active breakdown, provider filter, selected range, full filtered totals and effective-rate definition.

### Phone table access

The existing phone breakdown selector gains Models, Daily, Monthly, Tasks and Sessions.
Model rows show model/provider, total tokens, total cost and Cost/1M immediately.
Period rows show date/month and those same summary metrics.
A tap expands token categories and coverage in a focused detail surface.
A visible Sort control supports the same numeric ordering as desktop.
No required value depends on a wide horizontally scrolling table or hover.

## Empty and error states

No measurements in the authorized scope produces the installation/setup state.
An installed plugin changes the action to collection settings.
An enabled collector with no measurements shows pending collection and diagnostic status.
Existing native data or retained plugin history suppresses installation guidance.
An empty range links to All Time. A failed query offers Retry and never implies an empty database.
No selected workspace uses the existing workspace-selection state.

## Persistence and security

Schema initialization and migrations follow the existing repository lifecycle for both supported databases.
Task deletion cascades usage. Session deletion nulls its live reference while preserving a stable attribution key and task history.
Plugin uninstall leaves core measurements intact but removes plugin-private checkpoints under existing rules.

Host writes derive source identity from the connection and validate session ownership through task services.
Browser reads apply the same user/workspace restrictions as existing statistics handlers.
A browser cannot use a plugin's instance-global read authority to access another user's session.
Batch limits, checked arithmetic and bounded error payloads apply before persistence.

## Observability and verification

Record report duration, active subprocess count, batch size, changed rows, stale/conflict outcomes and last successful collection.
Do not log transcript content or secrets.
Coverage is a separate diagnostic from successful process exit.
The [work package](../../../plans/token-usage/plan.md) maps each acceptance criterion to planned evidence.
This document describes intended behavior. Feature tests and benchmarks remain pending implementation.

## Related decisions

- [Core usage projections](../../../decisions/2026-09-13-core-session-usage-projections.md)
- [Host data API](../../../decisions/0043-plugin-host-data-api.md)
- [Authenticated plugin actions](../../../decisions/2026-07-31-authenticated-plugin-actions.md)
