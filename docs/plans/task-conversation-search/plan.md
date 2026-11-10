---
created: 2026-10-19
status: in_progress
requirements:
  - REQ-TASKS-CONVERSATION-SEARCH-001
  - REQ-TASKS-CONVERSATION-SEARCH-002
  - REQ-TASKS-CONVERSATION-SEARCH-003
system_design:
  - ../../specs/tasks/system-design/conversation-search.md
legacy_specs: []
---

# Implementation plan: Task conversation search

## Overview

Extend Cmd+K task discovery to ordinary user and assistant messages. Build the
indexed projection first, then its worker/API, then the shared desktop/phone
consumer. Complete scale and lifecycle evidence before shipping.

The user authorized implementation after the design handoff. The implementation and local checks are complete. Work orders 01 through 03
are done. Work order 04 retains the unmeasured reference-host qualification
and operational rehearsal gates; development-host measurements are complete.
Measured evidence is recorded in [performance results](performance-results.md).

## Scope

In scope: SQLite/PostgreSQL indexes, durable incremental indexing, old-history
backfill, workspace-authorized bounded results, conversation excerpts, archived
ranking, phone entry, localization, and targeted performance evidence.

Out of scope: tool output, reasoning, plans, attachments, repository search,
semantic search, exact message scrolling, and changes to existing metadata APIs.
No external search server, persistent Kandev child tasks, or delegated workers.

## Intent and evidence

The user requested conversation content search after a history investigation,
with performance as a priority. Scope is interpreted as persisted ordinary user
and assistant text. Details below are proposed engineering choices, not answers
to questions the user was never asked.

The existing metadata endpoint already searches descriptions. `SearchMessages`
scans only one session. `useInlineTaskSearchEffect` owns the palette's debounce
and archive fallback. Reuse those boundaries without broadening other callers.
Tasks owns this feature because it owns the records, visibility, and discovery
contract. No parallel UI specification is needed.

## Technical approach

1. Add a rebuildable message projection, canonical dirty-ID triggers, and
   dialect-specific native indexes in the task repository. Source deletion
   also deletes projection text transactionally.
2. Add a task-owned background worker and a narrow search repository interface.
   Wire explicit startup, shutdown, and restore quiescence in backendapp.
   Add an authorized conversation-search endpoint without counts or transcripts.
3. Run metadata and conversation reads independently from the palette hook.
   Merge unique task rows with active-first ordering. Add a plain excerpt and
   matching-session navigation for conversation-only rows.
4. Measure realistic scale, common-term behavior, write overhead, cancellation,
   historical startup, and index storage. Publish only observed results.

| Database | Indexed lookup | Compatibility proof | Missing-capability behavior |
| --- | --- | --- | --- |
| Shipped SQLite with fts5 | Trigram MATCH on normalized projection | Fresh/replay/mutation/cascade tests and EXPLAIN QUERY PLAN | Metadata search works, conversations unavailable |
| PostgreSQL with pg_trgm | GIN plus escaped locale-independent LIKE | Real isolated database behavior, races, EXPLAIN ANALYZE | Metadata search works, administrator guidance |
| SQLite custom build without fts5 | No conversation scan | Capability test without build tag | Explicit unavailable state |

PostgreSQL tests require `KANDEV_TEST_POSTGRES_DSN` pointing to a disposable
server. A skipped test is not compatibility evidence. Tests must use isolated
schemas and must not remove a shared extension during cleanup.

## UI previews

The user requested Kandev rich output instead of ASCII drawings. These stable
view definitions are the source for the rich preview and UI work order.

| View | Entry and composition | Result and primary action |
| --- | --- | --- |
| UI-01 Desktop results | Cmd+K, existing search field and scope tabs, one result list | Task title and state above `Conversation match` and a highlighted excerpt. Enter opens matching session. |
| UI-02 Phone results | Labelled Search tasks action in navigation menu, inset search drawer, fixed field and close action | Task title/state, then wrapped excerpt. Entire row is a 44px-or-larger tap target. Tap opens matching session. |
| UI-03 Pending/error | Metadata rows stay visible. Secondary status sits beside the result group | Indexing, unavailable, or query guidance. Changing the query or reopening search makes a new request. |
| UI-04 Empty/limited | Empty means completed search with no match. Limited means candidate/deadline budget reached | Show refine-query guidance for limited search. Never label incomplete search as no matches. |

Desktop retains its current density. Phone follows the task-switcher drawer
exemplar, with a dynamic viewport, safe-area padding, and one contained scroll
body. Keep query state and selection shared across both wrappers. New results
arrive without animation. Match highlighting uses text and semantic marks.

## Tests and traceability

The following files contain the delivered regression coverage. See the work
orders for commands and the measured report for the limits of this evidence.

| Acceptance | Evidence |
| --- | --- |
| 001.1, 001.2, 002.2, 002.5 | `conversation_search_test.go`: `TestConversationSearchMatching`, `TestConversationSearchConsistency`, PostgreSQL equivalents |
| 001.3, 001.4 | `use-command-panel-task-results.test.ts`, `use-command-panel-task-navigation.test.ts`, real endpoint tests |
| 001.5 | `command-panel-dialog.test.tsx`, desktop and mobile Playwright scenarios |
| 002.1 | Repository transfer, cascade, edit, rollback tests and HTTP workspace denial tests |
| 002.3, 002.4 | Worker restart, token races, backfill checkpoint, streaming fairness and restore/shutdown tests |
| 003.1, 003.4 | opt-in `TestConversationSearch*PerformanceBudget` and `TestConversationSearchStreamingCost`, measured report (reference-host qualification pending) |
| 003.2, 003.3 | Candidate/time budgets, payload bounds, driver cancellation, obsolete-response hook tests |

All short AC references above use prefix `AC-TASKS-CONVERSATION-SEARCH-`.

## E2E tests

- `tests/task/conversation-search.spec.ts`, project `chromium`: find a task
  from message text, inspect highlighting, open its matching session, and confirm
  edited-away text no longer matches.
- `tests/task/mobile-conversation-search.spec.ts`, project `mobile-chrome`:
  touch entry, readable excerpts, matching-session navigation, input size,
  target size, and no horizontal overflow.
- `tests/command-panel.spec.ts` and
  `tests/task/mobile-command-palette-task-actions.spec.ts`: existing palette
  behavior and phone command-confirmation handoffs.

Additional lifecycle, archive ordering, metadata deduplication, and cancellation
cases are covered at the repository and hook layers. The broader proposed
browser matrix (including simulated virtual-keyboard geometry) is not claimed
as completed.
Use causal HTTP/WS waits from `e2e/helpers/causal-waits.ts`. Seed ordinary
messages through fixture APIs and poll explicit indexing readiness. Never wait
an arbitrary duration and assume the index is ready. Run projects sequentially.

## Work orders

- [x] [Task 01: Indexed projection](task-01-indexed-projection.md)
- [x] [Task 02: Background indexing and search API](task-02-search-service.md)
- [x] [Task 03: Palette and phone discovery](task-03-palette-results.md)
- [ ] [Task 04: Scale evidence and public guidance](task-04-performance-evidence.md)

Dependency order is 01 -> 02 -> 03 -> 04. No parallel agent work is authorized.
Each linked work order contains its exact verification commands.

## Verification results

Implementation checks are recorded in the work orders and performance report.
The commands below record the earlier design validation. Final implementation
checks pass: backend correctness/race tests, 55 frontend unit tests, 15 desktop
and three phone browser tests, Go lint, ESLint, TypeScript, localization, and
document validation. The reference-host and operational qualification limits
in Task 04 remain open; the requirements and system design remain draft until
those gates are resolved.

Design checks completed on 2026-10-19:

- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Catalog discovery found both new task-owned artifacts.
- `.github/scripts/pr-docs.cjs` `validateCoverage` accepted all four work orders
  against the proposed runtime path. This was a local linkage preflight, not a
  live PR check or a claim that production files changed.
- Relative document links and whitespace checks passed for all seven new files.
- Implementation adds production and test changes. The user authorized commit, push, and a ready-for-review PR after testing the isolated instance.

## Risks

- Native indexes add storage proportional to conversation size. Measure bytes
  and build time at one million messages before accepting the design.
- A broad query can still spend time ordering index hits. Enforce the deadline
  and benchmark common phrases separately from selective matches.
- Streaming writes and background publication can race. Dirty tokens, canonical
  joins, and deletion triggers require real concurrent database tests.
- PostgreSQL extension permissions and collation require real deployment proof.
  Do not replace failed index setup with a full history scan.
- The shared phone palette shell also hosts command confirmations. Preserve its
  existing confirmation behavior and test that flow after the shell change.

## Documentation delivery

Use `/docs-maintainer` during Task 04. Update the task-discovery how-to in
`docs/public/tasks-and-workflows.md` and database capability reference in
`docs/public/configuration.md`. Both public pages now describe the implemented behavior and capability requirements.

## Final layout validation

Conversation-only rows use the expanded excerpt layout. Metadata-only and idle
rows retain the compact desktop layout. Rendered checks confirm 32-pixel
desktop metadata rows, 44-pixel phone targets, a 4-pixel label-to-excerpt gap,
and 6-pixel bottom padding. The desktop browser regression covers idle and
metadata-search rows before checking conversation navigation.

## PR integration and review

The feature branch is updated to the current reviewed base. Conflict resolution
preserves integrated-control navigation ownership and the database restore
preflight guidance. Conversation search remains inside ordinary phone navigation.
The compact task-row regression and conversation excerpt spacing are retained.

Post-integration local validation is complete: 55 frontend unit tests,
TypeScript, SQLite/PostgreSQL repository races, service/HTTP authorization,
worker fairness, runtime stop/join, 15 desktop browser checks, and three phone
browser checks pass. Remote CI and review verification are tracked against the pushed head.
Greptile remediation adds direct API encoding/cancellation tests, generated
Traditional Chinese search copy, package-level worker leak checks, and
rate-limited content-free indexing failure logs.
Final CI and review disposition are recorded against the pushed head outside
this document; a later push invalidates earlier CI evidence.

CodeRabbit follow-up adds eligible-only historical pages, PostgreSQL
`SKIP LOCKED` publication with deletion fencing, fixed setup-failure reasons,
causal HTTP waits for message-edit removal, and expected-hit/page-state checks
in the measurement harness. The PostgreSQL reverse-order deletion regression
failed with SQLSTATE 40P01 before the fix and passes afterward. Historical scale
samples are retained as timing observations, without retroactively claiming
that their result-page outcomes were validated. Backend race validation of the follow-up changes passes across the repository,
worker, runtime wiring, service, and HTTP handler packages. The strengthened message-edit browser regression and phone search check both
pass. Remote CI remains in progress. Its phone-header fixture failure reproduced locally and is corrected with the real command registry provider. All 55 targeted frontend tests pass after the correction.

Backend CI also exposed a test-fixture PID collision in `TestRunOrphanReapPhaseDoesNotReportCapForTrailingPreSignalSkips`. Its fixed fake-PID range could include the current test process, which production ownership checks correctly protect. A forced collision reproduced the exact 255-versus-256 signal assertion. The fixture now allocates fake PIDs above the current process. Twenty repeated orphan-cleanup race runs pass. This correction changes test setup only and preserves production cleanup behavior.

The phone Quick Chat cancellation E2E also reproduced a palette regression: synchronous selection unmounted the drawer before the touch click finished bubbling, dismissing the underlying dialog. The palette now stops that click at its content boundary. Both existing Quick Chat cancellation scenarios pass on a fresh build. The expected modal behavior is unchanged; this is a regression correction, with no public-copy change.

Final local validation of both CI corrections passes: 55 frontend tests, TypeScript, five phone E2E scenarios, twenty repeated orphan-cleanup race runs, and the complete task-service race suite (`GOMAXPROCS=3 go test -tags fts5 -race ./internal/task/service -count=1 -timeout 10m`, 599.166 seconds). The initial full-package attempt exceeded a five-minute local budget while still entering later tests; the ten-minute package budget matches CI. No per-test timeout was changed.
