---
title: "Token Usage and Retained Costs"
description: "Understand workspace usage totals, retained accounting, attribution, calendars, and incomplete history."
---

# Token Usage and Retained Costs

**Stats > Token Usage** explains token use and cost for the selected workspace.
The date range and provider filter apply to the summary, breakdown, and **Copy Stats** totals.
Table pages change the visible rows. They do not limit those totals.

## Retained accounting

Kandev saves accepted native agent usage and external collector observations separately from the task conversation.
Archive, task or session deletion, transcript pruning, and tool payload cleanup preserve this accounting.
An archived task keeps its link while the task remains available.
A deleted task or session shows an unavailable label with no navigation link.

Retained accounting can include:

- Input, output, cache read, cache write, reasoning, total, and source-reported token amounts.
- Cost amounts, currency, cost basis, pricing versions, and coverage or freshness indicators.
- Model, provider, agent client, source identifiers, revisions, and digests that identify corrections or repeated records.
- Consumption times, source dates and calendars, coverage intervals, and collection times.
- Captured workspace, organization, user, task, session, turn, and execution identifiers, plus provider conversation or response references.

Accounting does not require saved prompts, responses, source code, task titles, credentials, or personal display names.
Existing backups can contain data removed from the live database.
See [data operations](operations.md#database-operation) for backup and reset behavior.

Kandev keeps captured organization and user evidence with each observation.
Later ownership, membership, or profile changes do not rewrite that evidence.
User attribution comes from saved admission evidence, not the user who opens Stats or a collector account name.

Workspace access still controls retained statistics after task deletion.
When organizations are enabled, a workspace transfer does not give its new organization access to accounting captured by its former organization.
Organization suspension blocks access.

Task cleanup preserves accounting. Explicit workspace or organization deletion erases the affected retained accounting from the live database.
Backups and external provider records follow their own retention rules.

## Sources and collector corrections

Native agent observations and installed collectors can describe the same work.
Kandev selects token and cost evidence independently to avoid adding overlapping representations twice.
Collectors normalize their source data before they submit observations to the shared accounting store.
The same accounting rules apply to each collector.
A later accepted collector correction replaces the earlier revision of that observation after the updated history becomes available.
Repeated delivery does not add the same accepted observation again.

Collector state is for bounded progress markers.
Plugin state must not contain raw transcripts, copied source records, or copies of token and cost amounts.
See the [plugin storage decision table](plugins-authoring.md#storage-decision-table) for the storage contract.

## Tokens, costs, and attribution

The details distinguish these values:

- **Model** and **Provider** identify the supplied model and its provider.
- **Agent client** identifies the client. A client name does not establish a provider.
- **Token user attribution** and **Cost user attribution** describe each selected source independently.
- **Mixed users** means the evidence covers multiple users. **Unknown** means attribution is unavailable.
- **Reported cost**, **Estimated cost**, **Cost unavailable**, or **Mixed cost basis** describes the selected cost evidence.

Unknown provider and user values remain explicit.

Unknown categories remain unavailable. A measured zero remains zero.
Source totals can differ from category totals when a provider uses different token semantics.
Kandev preserves these distinctions and displays large integer amounts without rounding them through JavaScript numbers.

Reasoning can be part of output tokens. Cache reads can be part of input tokens.
Kandev preserves known relationships and the total reported by the source.
Adding all categories can count the same tokens twice.
Sources can also report reasoning separately.
When category relationships are unsupported, the affected reasoning amount or derived rate remains unavailable.

Several provider responses can belong to one Kandev turn.
Native response counts do not establish a user turn total.
Turns remain unavailable unless a source supplies a supported turn aggregate.

Reported cost comes from the selected source's monetary usage record.
Estimated cost comes from a pricing calculation.
Unpriced usage can retain known token amounts while its cost remains unavailable.
A known zero cost remains zero.

Costs keep their original currency. Kandev does not convert currencies or add unlike currencies into one monetary total.
An effective cost rate is available only when complete token and cost coverage supports that calculation in one currency.
Partial or unknown coverage can make the rate unavailable even when some amounts are known.

Coverage describes the source evidence for the selected metric and period.
Known amounts do not establish complete coverage of all providers, sessions, or users.
Token and cost coverage can differ.

## Calendars and lifetime values

Observations with an exact consumption time use the selected browser calendar.
Calendar conversion respects daylight saving changes and fractional UTC offsets.
Coarse daily evidence keeps its original source date and calendar when exact conversion is unavailable.
Details identify that source calendar. A range that overlaps only part of such an interval shows partial coverage.
An interval total does not identify the amount consumed on each day.

Lifetime values without consumption dates remain undated.
They can appear in lifetime breakdowns, but never become invented points on a daily or monthly chart.
Collection time describes freshness. It does not establish when consumption occurred.

## Preparation and saved values

On the first build, **Preparing usage history** means Kandev does not yet have a complete publication.
The page does not substitute a zero or an empty-history result.
During a later rebuild, the page keeps the last complete values and shows **Updating history**.

If a read fails, **Retry** requests the same selected scope again and keeps its saved values visible.
An access denial clears those values.
When a publication changes during a multi-page chart read, Kandev rejects the mixed result and offers a retry.

Stale values remain saved accounting with their last known freshness.
A successful read of saved values does not establish complete current collector coverage.

Older databases migrate in bounded background batches before retained reporting becomes ready.
A history-gap notice means some earlier usage cannot be included.
Evidence can be missing or lack trustworthy workspace and organization ownership.
A rebuild cannot recreate deleted source evidence or supply missing ownership proof.

If older native usage cannot be safely copied, task or session deletion stops before it removes those records.
You can archive the task while ownership is unresolved.

The phone view provides the same filters, totals, and details.
Open a row to inspect its categories, sources, attribution, and calendar.
Use **Copy Stats** for complete filtered totals, including rows outside the current table page.

## Storage and performance

Kandev stores durable daily summaries and indexed accounting records in the database.
Reports apply range and provider filters, grouping, totals, and pagination in SQL.
Stats reads saved accounting without rescanning conversations or collector files.

Benchmark evidence for five years of history is still pending.
Performance and capacity limits require measurements of the deployed workload.
