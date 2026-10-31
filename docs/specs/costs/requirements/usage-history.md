---
status: active
system: costs
created: 2026-10-28
owners:
  - kandev
---

# Retained Usage History Requirements

## Overview

Users need years of token and cost history after tasks disappear or collectors change.
The costs system owns this history independently of task content and collector installation.
This extends [Token Usage](token-usage.md) with a separate accounting lifecycle.
The accepted product direction does not imply that its implementation is complete.

## Terminology

- **Usage subject:** A durable identity for work, independent of its live task or session.
- **Observation:** A source's measurement for identified work and coverage.
- **Canonical fact:** The contribution core selects after it resolves equivalent and overlapping observations.
- **Actor:** The user whose authenticated execution or turn produced usage.
- **Scope:** The organization and workspace ownership captured for usage.
- **Calendar:** The timezone and date boundaries that a dated measurement represents.

## Requirements

### REQ-COSTS-USAGE-HISTORY-001: Independent history lifetime

**Intent:** Routine task management does not erase recorded consumption.

#### Acceptance criteria

- **AC-COSTS-USAGE-HISTORY-001.1:** Archived tasks shall continue to contribute to usage history and statistics.
- **AC-COSTS-USAGE-HISTORY-001.2:** Task deletion shall preserve accepted tokens, models, providers, costs, dates, and accounting attribution.
- **AC-COSTS-USAGE-HISTORY-001.3:** Session pruning and task-ID reuse shall not erase or reassign earlier usage.
- **AC-COSTS-USAGE-HISTORY-001.4:** Retention shall cover native usage and external collector measurements, including measurements accepted before migration.
- **AC-COSTS-USAGE-HISTORY-001.5:** Collector disablement, replacement, restart, or uninstall shall not remove retained history.
- **AC-COSTS-USAGE-HISTORY-001.6:** Existing explicit workspace and organization erasure shall remove their retained data without moving it into an unowned scope.

### REQ-COSTS-USAGE-HISTORY-002: Tenant and actor attribution

**Intent:** History remains attributable without crossing organization or user access boundaries.

#### Acceptance criteria

- **AC-COSTS-USAGE-HISTORY-002.1:** Core shall derive scope from validated ownership. A collector-supplied user or organization shall not override it.
- **AC-COSTS-USAGE-HISTORY-002.2:** Actor attribution shall use execution or turn provenance, independently of the refresher, collector, task assignee, or workspace owner.
- **AC-COSTS-USAGE-HISTORY-002.3:** Usage without provable actor attribution shall remain unknown or mixed. Combined session totals shall not be split by assumption.
- **AC-COSTS-USAGE-HISTORY-002.4:** Auth and MultiTenancy modes shall preserve their existing access rules, including the single-user synthetic identity when Auth is disabled.
- **AC-COSTS-USAGE-HISTORY-002.5:** Unauthorized reads, writes, corrections, summaries, and collection-target discovery shall reveal no foreign usage or existence information.
- **AC-COSTS-USAGE-HISTORY-002.6:** Later task reassignment or ownership changes shall not rewrite historical actor or organization attribution.
- **AC-COSTS-USAGE-HISTORY-002.7:** Missing historical identities shall remain explicit after feature-flag changes. Migration shall not invent a historical actor.

### REQ-COSTS-USAGE-HISTORY-003: Preserved measurement meaning

**Intent:** Consumers can distinguish measured amounts, unknown values, and accounting estimates.

#### Acceptance criteria

- **AC-COSTS-USAGE-HISTORY-003.1:** Records shall preserve available input, output, cache-read, cache-write, reasoning, and source-total token counts.
- **AC-COSTS-USAGE-HISTORY-003.2:** Records shall distinguish exact model IDs, model providers, agent clients, and collector provenance. Missing dimensions shall remain unknown.
- **AC-COSTS-USAGE-HISTORY-003.3:** Totals shall respect provider category semantics without counting cache or reasoning subsets twice.
- **AC-COSTS-USAGE-HISTORY-003.4:** Cost shall retain its currency and reported, estimated, or unknown basis. Different currencies shall not produce one combined monetary total.
- **AC-COSTS-USAGE-HISTORY-003.5:** Records shall preserve occurrence or coverage time, collection time, calendar, source version, revision, and completeness separately.
- **AC-COSTS-USAGE-HISTORY-003.6:** Accounting retention shall not require copies of prompts, responses, source code, credentials, task titles, or personal display names.
- **AC-COSTS-USAGE-HISTORY-003.7:** Persistence and public transport shall preserve supported integer amounts without overflow or JavaScript precision loss.

### REQ-COSTS-USAGE-HISTORY-004: Collector interoperability

**Intent:** A new tool can measure existing work without losing or duplicating history.

#### Acceptance criteria

- **AC-COSTS-USAGE-HISTORY-004.1:** A documented, versioned measurement contract shall accept native and external observations independently of tokscale.
- **AC-COSTS-USAGE-HISTORY-004.2:** Equivalent work reported by different collectors shall contribute once. Unproven equivalence shall remain visibly incomplete.
- **AC-COSTS-USAGE-HISTORY-004.3:** Replays shall be idempotent. Older revisions shall not replace newer values. New corrections shall support increases and decreases.
- **AC-COSTS-USAGE-HISTORY-004.4:** Older supported collectors shall retain their existing contract. Unsupported semantics shall produce explicit upgrade or unsupported-version results.
- **AC-COSTS-USAGE-HISTORY-004.5:** Corrections after task deletion shall require an authorized retained binding and shall not permit arbitrary orphan writes.
- **AC-COSTS-USAGE-HISTORY-004.6:** Resumable target discovery shall avoid duplicates or skipped targets caused by offset shifts during concurrent task changes.

### REQ-COSTS-USAGE-HISTORY-005: Bounded historical queries

**Intent:** A recent-range query does not repeatedly process years of unrelated detail.

#### Acceptance criteria

- **AC-COSTS-USAGE-HISTORY-005.1:** Date filtering shall use consumption coverage with inclusive starts and exclusive ends, independently of collection time.
- **AC-COSTS-USAGE-HISTORY-005.2:** A bounded-range chart shall not load or pairwise-compare the workspace's complete observation history.
- **AC-COSTS-USAGE-HISTORY-005.3:** Provider filters, sorting, pagination, and full filtered totals shall describe the same canonical dataset.
- **AC-COSTS-USAGE-HISTORY-005.4:** Coarse source-day data shall retain its calendar. A timezone change shall not invent hourly distribution or proportional costs.
- **AC-COSTS-USAGE-HISTORY-005.5:** Verification shall report five-year query plans, visited rows, latency, and memory. Adding unrelated years shall not trigger raw-history scans for recent charts.
- **AC-COSTS-USAGE-HISTORY-005.6:** Availability and freshness shall remain correct for empty ranges without broad history loads. Trend results shall not silently truncate.

### REQ-COSTS-USAGE-HISTORY-006: Durable daily summaries

**Intent:** Charts read compact derived history without weakening correction or coverage rules.

#### Acceptance criteria

- **AC-COSTS-USAGE-HISTORY-006.1:** Daily summaries shall preserve scope, actor attribution, model, provider, currency, calendar, and accounting completeness.
- **AC-COSTS-USAGE-HISTORY-006.2:** Corrections and source replacement shall replace affected contributions. Repeated cumulative snapshots shall not increment totals again.
- **AC-COSTS-USAGE-HISTORY-006.3:** A committed summary shall correspond to a committed canonical revision. An interrupted update shall not expose a half-applied revision.
- **AC-COSTS-USAGE-HISTORY-006.4:** Restart recovery and rebuild shall reproduce the same totals from retained observations.
- **AC-COSTS-USAGE-HISTORY-006.5:** Incomplete rebuilds shall preserve the last complete generation or return explicit preparation status, rather than authoritative zeros.
- **AC-COSTS-USAGE-HISTORY-006.6:** Partial amounts and unknown cost shall remain distinguishable from complete measured zero. Unavailable effective rates shall remain unavailable.
- **AC-COSTS-USAGE-HISTORY-006.7:** Undated lifetime measurements shall remain separate from dated summaries, without double counting or invented first-collection dates.

### REQ-COSTS-USAGE-HISTORY-007: Safe adoption and visible history

**Intent:** Existing installations adopt retained history without silent gaps or misleading statistics.

#### Acceptance criteria

- **AC-COSTS-USAGE-HISTORY-007.1:** Migration and reconciliation shall resume bounded durable work after interruption without duplicate history.
- **AC-COSTS-USAGE-HISTORY-007.2:** Task deletion shall not remove legacy observations before their retained copies commit. Failure shall keep deletion retryable.
- **AC-COSTS-USAGE-HISTORY-007.3:** New accepted native events shall survive task deletion without depending on a later polling cycle or a volatile callback.
- **AC-COSTS-USAGE-HISTORY-007.4:** Deleted-task rows shall retain amounts and display a localized unavailable-task label without a broken navigation link.
- **AC-COSTS-USAGE-HISTORY-007.5:** Desktop and phone views shall expose the same retained amounts, coverage, and attribution states through existing accessible controls.
- **AC-COSTS-USAGE-HISTORY-007.6:** Missing pre-migration history shall remain a disclosed gap. Migration shall not claim recovery of unavailable transcripts or deleted records.
- **AC-COSTS-USAGE-HISTORY-007.7:** An interrupted rollout shall retain accepted observations. Reverting reads shall not delete the new durable history.

## Out of scope

- New organization-wide access roles, a billing dashboard, invoice reconciliation, or budget enforcement.
- New model-protocol collectors, remote transcript transport, or an automatic retention expiry.
- A new history-erasure UI, currency conversion, or reconstruction of already destroyed data.

## Implementation Plans

- [Retained Usage History package](../../../plans/usage-history/plan.md)
- [System design](../system-design/usage-history.md)
