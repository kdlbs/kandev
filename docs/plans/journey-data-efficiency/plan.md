---
created: 2026-10-09
status: complete
requirements:
  - REQ-PLATFORM-INTERACTIVE-READS-006
  - REQ-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001
  - REQ-PLATFORM-JOURNEY-LOADING-001
  - REQ-PLATFORM-JOURNEY-LOADING-002
  - REQ-PLATFORM-JOURNEY-LOADING-003
system_design:
  - ../../specs/platform/system-design/journey-data-loading.md
legacy_specs: []
---

# Implementation Plan: Efficient Board and Task Journeys

## Overview

Remove unnecessary writer reservations and excess data loading from homepage, task detail, and multi-session task detail.
First isolate completion-gate inspection. Then batch summary reads, narrow session projections, and make detail demand follow visible panels.
Normalize boot data, load turn context incrementally, and share initial/refresh requests across consumers.

The [completed investigation](evidence.md) supplies source paths, lock evidence, production browser captures, and the synthetic fixture.
All eight work orders and six source-review corrections are complete. Results, final validation, and remaining writer-health limits are in [implementation evidence](implementation-evidence.md).
The user authorized PR publication after final validation. Deployment and live data mutation are outside its scope.

## Settled intent and ownership

The user requested improvements to the measured database and UI loading problems across the three journeys.
Platform owns shared read availability and bounded data delivery. Task records, completion authority, and UI navigation remain under their current domain contracts.
The plan preserves full history access, desktop split chats, phone navigation, saved sidebar views, compact attention indicators, and authorization.
It changes data demand rather than product layout. No material product question remains unresolved.

Existing ADRs already require reader snapshots and normalized task ownership. The package applies those boundaries rather than adding another ADR.
Source-only Quick Chat and conditional-polling candidates are explicitly deferred. They need separate behavioral evidence before replacing their notification or refresh contracts.

## Scope

### In scope

- Standalone completion-gate reads on the reader pool, with coherent observations.
- Bounded batch gate summaries and narrow session projections for list/boot paths.
- Explicit rich-data demand for visible desktop, phone, split, and preview chats.
- Versioned, normalized, route-scoped boot data and authorized requested-session selection.
- Message-window turn context without an unconditional complete-history read.
- Shared task, board, and active-panel metadata read ownership.
- Deterministic structural tests and before/after measurement on synthetic fixtures.

### Out of scope

- Changes to completion mutation admission, evidence locks, message persistence, or historical task semantics.
- Larger pools/timeouts, health bypasses, speculative indexes, or a general cache-library migration.
- Quick Chat unseen/completion redesign, cheap polling revisions, and new runtime flags.
- Claiming that the historical 503 writer or separate agentctl file errors are identified.
- Replacing the separate [message-update optimization](../message-update-writer-occupancy/plan.md).

## Technical approach

The [design](../../specs/platform/system-design/journey-data-loading.md) is the technical authority.
Read requirements in [interactive reads](../../specs/platform/requirements/interactive-read-availability.md), [status delivery](../../specs/platform/requirements/bounded-task-status-delivery.md), and [journey loading](../../specs/platform/requirements/journey-data-loading.md).

### Read availability and projections

`GetTaskCompletionGate` uses `r.ro` for standalone inspection. Supplied mutation transactions remain unchanged.
`GetTaskCompletionGateSummaries` (new) reads up to 100 tasks per snapshot with at most six data queries.
`ReconcileTaskStatusSummaries` receives keyed observations rather than calling a gate read inside each task iteration.
Typed session summary/list projections replace full-session reads in `boot_state.go` and `task_http_handlers.go`.
No schema migration is planned. Necessary missing/stale-summary writes retain existing CAS and recovery rules.

### Detail demand

Dockview visibility determines `detailActive`. Keyboard focus and unread acknowledgement remain separate concepts.
`TaskChatPanel` and its data hooks retain cached state and drafts while hidden, but release rich subscriptions and expensive loaders.
Existing workspace lifecycle/pending events keep sibling tabs current. A visible preview and two visible split panes remain valid detail consumers.

### Boot and conversation windows

Boot version 2 contains unique task/session entities and ID memberships. The decoder accepts versions 1 and 2.
The selected board loads its own membership. Task detail loads one saved sidebar page, compact siblings, and the authorized selected session.
The existing normalized store and coverage controller remain authoritative. Additional multi-board surfaces load on demand.

An optional `include_turns` message-list extension adds turn context and explicit window coverage.
Boot, pagination, search, and reconnect use that same window contract. The full-turn endpoint remains available.
Partial coverage never sets a full-history completion marker.

### Resource ownership

Use store/auth/workspace scoped owners following `TaskNavigationReads` and `agent-list-resource.ts`.
Task sessions, workflow snapshots, and shared metadata each have one in-flight read per key and generation.
Preserve response-option distinctions, revision ordering, initial boot-to-live gap repair, and endpoint-specific recovery.
A final consumer release cancels obsolete work. One consumer cannot abort another consumer's read.

### Compatibility matrix

| Boundary | Intended behavior | Evidence / fallback |
| --- | --- | --- |
| SQLite | Deferred reader snapshots; existing immediate writer guards | Real separate factory handles and held-writer tests |
| PostgreSQL | Read-only repeatable snapshot for inspection; unchanged mutation locks | Disposable-server gate/projection/window tests; a skip is incomplete |
| Desktop | Only visible rich chats, including split/preview | Production Chromium tests and subscription frames |
| Phone | One focused chat, existing picker/drawer | `mobile-chrome` tests; preserve desktop draft/layout on resize |
| Boot v1/v2 | New decoder accepts both; canonical v2 wire entities | Golden decoder tests; unknown versions use bounded route recovery |
| HTTP/WS messages | Additive optional turns and coverage | Existing clients unchanged; older backend uses bounded legacy fallback |
| Sidebar ordering | Existing SQLite local profile and server fallback | Existing coverage/filter/paging tests; no fabricated complete coverage |
| Plugins | Existing public DTOs, no private boot dependency | Host contracts unchanged; documentation audit if an API option is public |

## ASCII UI preview

The drawings describe retained UI structure and data ownership. Spacing and example labels are illustrative.
No new user-facing labels or controls are required. Reuse current localization and primitives.

`UI-01: Board and task entry` applies to AC-PLATFORM-JOURNEY-LOADING-001.1–001.7 and -003.1–003.5.

```text
Desktop: homepage                      Desktop: task detail
+----------------+------------------+  +----------------+----------------------+
| Workspace/nav  | Selected board   |  | Tasks page     | Task / session tabs  |
| Tasks page     | Existing cards   |  | <=100 rows     | Selected detail      |
| Filters/pages  | Other boards:    |  | Filters/pages  | Existing work panels |
|                | load on demand  |  |                | Composer             |
+----------------+------------------+  +----------------+----------------------+

Phone: existing focused layout
+-----------------------------------+
| Workflow/task/session picker      |
+-----------------------------------+
| One board column OR task detail   |
| Existing primary scroll region    |
+-----------------------------------+
| Existing primary action/composer  |
+-----------------------------------+
Picker opens existing bottom drawer; selection navigates to its destination.
```

Sidebar/header controls retain their fixed regions. Board/detail content keeps its existing scroll owner.
Task detail must not wait for failed optional sidebar or repository enrichment.
A cold page shows existing loading. Success with no rows shows existing empty state. A failed read shows existing Retry.
Eligible stale rows remain visible only under current scope/freshness rules, never as newly verified data.

`UI-02: Session visibility and history` applies to AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.12–001.14 and AC-PLATFORM-JOURNEY-LOADING-002.1–002.4.

```text
Desktop, one visible chat              Desktop, intentional split
[Agent A | Agent B | Agent C]           [Agent A]          [Agent B]
[Agent A conversation      ]           [Conversation A]   [Conversation B]
[Composer / retained draft]           [Draft A       ]   [Draft B       ]
A: detail data                         A and B: detail data
B/C: compact badges + retained drafts  Hidden siblings: compact badges

Phone
[Task / session picker]
[One conversation scroll region]
[Composer / retained draft]
One selected session has detail data.
```

Reveal uses existing loading/refresh/error presentation while retaining the draft. Older-page and search navigation keep turn context.
A visible preview still loads detail even when read acknowledgement is suppressed.
The phone uses `SessionTaskSwitcherSheet` and `MobilePickerSheet`, preserving safe areas, focus return, and existing touch targets.
Desktop-phone-desktop transitions preserve saved layout and drafts. A new hidden desktop tree must not mount behind the phone layout.

## Tests

New test files and named tests are planned deliverables. Work orders own exact runnable commands and red/green results.

| Acceptance coverage | Test file / named proof | Owner |
| --- | --- | --- |
| INTERACTIVE-READS-006.1–006.3 | `completion_gate_read_test.go`: `TestCompletionGateReadDuringWriter`, `TestCompletionGateReadPostgres`; `journey_read_test.go`: `TestJourneyReadDuringWriter` | 01 |
| INTERACTIVE-READS-006.2–006.5 | `completion_gate_batch_test.go`: `TestCompletionGateBatchBounds`, `TestCompletionGateBatchPostgres`; `service_status_summary_batch_test.go`: `TestTaskStatusSummaryBatchRepair` | 02 |
| INTERACTIVE-READS-006.3–006.5; JOURNEY-LOADING-001.2 | `session_summary_projection_test.go`: `TestSessionSummaryProjectionMetadataIndependence`, `TestSessionSummaryProjectionPostgres`; handler projection parity | 03 |
| BOUNDED-TASK-STATUS-DELIVERY-001.3–001.5, .12–.14 | `visible-session-demand.test.tsx`: hidden siblings, visible splits, preview, draft retention, last-consumer release | 04 |
| JOURNEY-LOADING-001.1–001.7 | `boot_journey_test.go`: `TestBootJourneyScopeAndBytes`, `TestBootJourneySessionSelection`; `src/boot-payload.test.ts`: v1/v2 graph decode and normalization | 05 |
| JOURNEY-LOADING-002.1–002.4 | `message_turn_window_test.go`: `TestMessageTurnWindowBounds`, `TestMessageTurnWindowPostgres`; turn hydration and live-refresh tests | 06 |
| JOURNEY-LOADING-003.1–003.5 | `use-task-sessions.test.ts`, `use-workflow-snapshot.test.ts`, `task-navigation-reads.test.ts`: concurrent consumers, gap repair, invalidation, scope cancellation | 07 |
| JOURNEY-LOADING-003.1–003.5; BOUNDED delivery .12 | `journey-metadata-resources.test.ts`: option-aware keys, shared consumers, late responses, failure retention | 08 |

Prefixes in the table expand to `AC-PLATFORM-`. Work-order frontmatter lists exact IDs.
Backend tests use `-trimpath -tags fts5 -race`. Benchmarks use non-race builds. PostgreSQL uses disposable data through the existing harness.
Implementation must update exact commands if file ownership changes and include every changed test suite before marking its work order done.

## E2E tests

Use the managed production-build runner. Do not run overlapping suites or override worker limits.

| File under `apps/web/e2e/tests` | Project | Flow / acceptance |
| --- | --- | --- |
| `session/visible-session-demand.spec.ts` | chromium | One chat/eight tabs, split/merge, preview, attention events, task switch/reconnect; BOUNDED delivery .3–.5, .12–.14 |
| `session/mobile-visible-session-demand.spec.ts` | mobile-chrome | Picker selection, one stream, draft persistence through responsive transition; same criteria |
| `task/journey-boot-loading.spec.ts` | chromium | Single/multi-board, deep link, foreign session, no sessions, saved/archived sidebar, 1k/10k scope and request budgets; JOURNEY-LOADING-001 and -003 |
| `task/mobile-journey-boot-loading.spec.ts` | mobile-chrome | Same loading/selection/retry outcomes through phone picker; JOURNEY-LOADING-001 and -003 |
| `session/message-turn-window.spec.ts` | chromium | Initial window, older history, search jump, completion/reconnect, old backend fallback; JOURNEY-LOADING-002 |
| `session/mobile-message-turn-window.spec.ts` | mobile-chrome | Phone history and selected-session recovery; JOURNEY-LOADING-002 |

Use `fixtures/test-base`, existing session page objects, and deterministic mock events. Count requests by method/action, full options, identity, and generation.
Wait for explicit state/readiness or event barriers. Do not copy the investigation's four-second observation sleeps into permanent E2E assertions.
Resource captures must show production assets rather than Vite development modules. Assert subscription release, not just initial counts.

## Measurement gates

- Warm inspection: zero writer transactions. Held-writer controls finish reads before release.
- Completion summaries: at most one reader snapshot and six data queries per 100 requested tasks.
- Metadata: 4 KiB versus 1 MiB unrelated session metadata changes neither projected payload nor full-metadata decoding count.
- Boot fixture: task detail <=512 KiB, single-board <=1.25 MiB before compression; unique task/session entities.
- Unrelated tasks: 1,000 to 10,000 adds no initial task/session records. Keep selected-board membership fixed for this comparison.
- History: boot needs at most 51 turns for 50 messages. Client cold/reconnect reads need at most 101 for 100 messages. Adding older turns changes no initial set.
- Rich demand: one visible desktop/phone chat = one session; two visible splits = two; ordinary sidebar rows = zero.
- Shared reads: one in flight per scoped resource/generation, with at most one trailing invalidation repair.

Task 01 owns the deterministic backend fixture, benchmark, and ten-sample baseline. Each work order owns its structural proof.
Task 08 records the integrated ten-sample comparison, idle and under the prior writer workload, in `implementation-evidence.md`.
Record latency median/p95, response bytes, errors, health deadlines, reader wait, writer wait/occupancy, and machine/source/fixture identity.
Do not turn host-dependent timings into fragile unit-test deadlines. Use held-resource barriers and structural counts in CI.
Any repeatable performance/health regression must be resolved. Remaining historical or writer-workload failures remain explicit limitations.

## Companion package inventory

These packages retain their existing statuses and historical verification. This plan does not reopen completed work or copy its test results as new evidence.

| Package | Recorded status / boundary |
| --- | --- |
| [Writer investigation](../database-writer-contention/plan.md) | Completed experiment; supplies writer workload and diagnostic observer |
| [Message update occupancy](../message-update-writer-occupancy/plan.md) | Implemented and verified separately; preserve its transaction-authority scope |
| [Original bounded delivery](../bounded-task-status-delivery/plan.md) | Implemented; preserve event semantics, response priority, idempotency |
| [Stream overload isolation](../session-stream-overload-isolation/plan.md) | Complete; preserve coalescing and final transcript correctness |
| [Runtime state ownership](../backend-runtime-state-ownership/plan.md) | Completed; preserve startup summary convergence |
| [Deleted-session errors](../deleted-session-error-summary/plan.md) | Done; preserve retained-error repair |
| [Summary equality](../task-summary-semantic-equality/plan.md) | Complete; preserve no-op semantics and CAS rebase |
| [Recent runtime logs](../recent-runtime-log-remediation/plan.md) | Implemented; preserve probe attribution and deleted-task behavior |
| [Task navigation availability](../task-navigation-availability/plan.md) | Complete; preserve bounded recovery and inbox ownership |
| [Sidebar shared state](../sidebar-query-memory/plan.md) | In progress; existing normalization/coverage authority; no filter redesign |
| [Archived update freshness](../archived-sidebar-update-freshness/plan.md) | Implemented; existing journal/tombstone authority; no stale resurrection |

The viewport delivery design supplies already-implemented compact pending events. No second event rollout is part of this plan.
Before implementation or rebase, refresh companion status and source references. If another package changes these contracts, reconcile scopes before editing.

## Work orders

Execute sequentially in the primary session. Dependency edges identify actual prerequisites, not permission for agents.

- [x] [Task 01: Keep completion-gate inspection off the writer](task-01-isolate-completion-gate-reads.md)
- [x] [Task 02: Batch completion-gate summary observations](task-02-batch-completion-summary-reads.md)
- [x] [Task 03: Read narrow session projections for navigation](task-03-project-compact-session-data.md)
- [x] [Task 04: Activate detail data only for visible chat panels](task-04-scope-visible-session-demand.md)
- [x] [Task 05: Normalize boot data and scope it to the route](task-05-bound-route-boot-data.md)
- [x] [Task 06: Load turn context with each message window](task-06-hydrate-turns-by-message-window.md)
- [x] [Task 07: Share task and board refresh ownership](task-07-share-navigation-refresh-ownership.md)
- [x] [Task 08: Share metadata reads across active task panels](task-08-share-active-panel-metadata.md)

Dependencies: 01 -> 02 -> 03 -> 05 -> 06 -> 07 -> 08. Task 04 is independently implementable and precedes 08.
All work orders remain sequential because several modify shared loaders, state, or fixtures.

## Verification results

Implementation and review corrections are complete. The final checks passed: 728 frontend tests, six desktop and five phone browser cases, TypeScript, ESLint on 140 changed files, affected Go race suites, backend lint/build, and documentation checks. [Implementation evidence](implementation-evidence.md#review-remediation) records the corrected source fingerprints. The investigation's results remain historical in `evidence.md`.
Design-package validation on 2026-10-09:

- `python3 scripts/list-docs.py validate`: passed (368 decisions, 1,495 specifications).
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `node docs/plans/journey-data-efficiency/validate-package.cjs`: all eight work orders covered, zero errors. The preflight uses a prospective production-file trigger to exercise coverage validation. It does not claim production source changed.
- Relative document links and owned-file references: passed. Future test files are explicitly declared.
- `git diff --check -- docs/plans/journey-data-efficiency docs/specs/platform`: passed.
- Worktree status: package remains unstaged and uncommitted. Prior diagnostic and message-update package edits remain intact.

No production or permanent application test changed during package creation.

## Risks

- Consistent inspection cannot replace atomic mutation revalidation.
- A compact projection can accidentally omit a status input or clear a rich cached field.
- Mounted-panel retention can preserve hidden loaders unless every detail effect receives demand.
- Boot normalization can falsify complete coverage or break stale-response protection.
- Partial turn coverage can appear globally complete unless every consumer respects its new scope.
- Deduplication can suppress necessary gap repair or mix authorization/response-option scopes.
- Shared-host benchmark variance and remaining writer saturation limit latency claims.
