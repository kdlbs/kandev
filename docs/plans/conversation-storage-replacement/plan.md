---
created: 2026-09-16
status: in_progress
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-002
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-005
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-006
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-007
system_design:
  - ../../specs/plugins/system-design/conversation-source-reconciliation.md
legacy_specs: []
---

# Implementation Plan: Conversation storage replacement

## Overview

Replace mandatory conversation copies with authorized reads of original messages and turns.
Keep the useful browser Host API and core Prompt History panel.
Deliver compatible readers first, migrate plugin and core consumers, then remove legacy writers and storage.
Ship the package as one release. Intermediate dual-path work orders are not release candidates.

The user requested and authorized implementation of this design package on 2026-09-16.
The user later authorized committing the implementation and this follow-up plan.
Production database mutation remains outside the package. Branch delivery is
handled by the current implementation handoff.

## Inputs and authority

- [Requirements](../../specs/plugins/requirements/prompt-history-extraction-host.md): new requirements 006 and 007 replace retired transport criteria 002.13, 002.15, and 002.16.
- [Design](../../specs/plugins/system-design/conversation-source-reconciliation.md) and [ADR](../../decisions/2026-09-16-conversation-source-reconciliation.md).
- [Original package](../prompt-history-plugin-host/plan.md) and [recovery package](../pr-3588-conversation-recovery/plan.md) retain historical results.
- Source baseline: `88c6c0fe0a6ae5d25332070d603b99f7d9241645`. PR #3588 merged as `2b1d0cf7d1e118dbf7b57ce5243131bb49b9dae4`.
- Before implementation, resolve current main and recheck affected symbols and migrations. Do not treat this baseline as permanent.

The plugin system owns the Host access contract. The task repository owns original records and atomic revision changes.
The selected design retains only current state, not durable delivery of intermediate versions.
The user selected incremental updates without hashes on 2026-09-16.
This revision replaces the earlier refresh-on-change approach. Cleanup requirements and Task 04 remain applicable.
The facade recovers deleted or inaccessible previously loaded scopes as terminal unavailable state without revealing which cause occurred.

## Scope

### In scope

- Revision-only metadata, transaction-bound mutation receipts, source reads, and authorized incremental subscriptions.
- Plugin and core recovery, including direct SQL mutation and missed-event repair.
- Removal of copied payloads, obsolete triggers, event mirrors, ACK/poison persistence, and startup backfill.
- Safe SQLite/PostgreSQL upgrade, specific Host-file cleanup, storage evidence, and documentation.

### Out of scope

- External plugin implementation, publication, and a plugin-owned transcript archive.
- Core Prompt History removal, saved-layout migration, and UI redesign.
- Generic replay infrastructure, exactly-once delivery, historical snapshots, new feature flags, and automatic compaction.
- Message content hashes, hash backfill, and hashing on streaming writes.

## Technical approach

Task 01 adds source readers and revision metadata behind a private v2 route contract.
Task 02 moves plugin scopes to live changes by ID and discrepancy-driven recovery.
Task 03 migrates core delivery and repairs original message and turn caches.
Task 04 removes journal initialization before backfill can run, then removes legacy database objects and Host event files.
Task 05 proves parity and updates the API and operational reference.

Exact components, cursor fields, transaction semantics, failure behavior, and migration order are defined in the system design.
A normal mutation updates only source records and one small revision per affected session for this feature.
The gateway checks only subscribed sessions. Complete change batches carry scoped current records or removals by ID.
Irrelevant changes carry coverage only and trigger no history reads.
Fully applied revisions remain separate from observed revisions. A counter check alone cannot advance applied state.
Normal updates use in-memory projection. Initial load, pagination, and discrepancy recovery use bounded source pages.

## Tests

New test names are implementation targets, not existing proof.
Each work order must record its actual RED failure and final GREEN commands.

| Criteria                                            | Target evidence                                                                                                                                | Owner |
| --------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- | ----- |
| 002.1, 002.4, 002.7, 006.1-2, 006.5, 006.8          | `TestConversationSourcePages`, `TestConversationRevisionAtomicity`, and PostgreSQL variants in new `conversation_source*_test.go`              | 01    |
| 002.2-3, 002.5-6, 002.9-10, 002.14, 002.17, 006.3-8 | Existing Host test files: revision race, loaded-range deletion, churn, expiry, access loss, independent panels, teardown                       | 02    |
| 006.3-4, 006.6, 006.8                               | `TestConversationDeliveryRepairsMissedPublication`, `TestConversationDeliveryIdleHasNoReads`, authorization/slow-client variants               | 02    |
| 002.5-6, 002.9, 005.3, 006.3-4, 006.6-7             | `use-session-recovery.test.tsx`, `client.test.ts`, gateway tests: rich state, optimistic messages, missed events, restart                      | 03    |
| 006.1-2, 006.8, 007.1-4                             | `TestConversationJournalCleanupPreservesSources`, failure/replay/partial-schema/PostgreSQL variants and Host file cleanup tests                | 04    |
| 006.1-2, 006.8                                      | `BenchmarkConversationSourceStorage`, 10k/100k rows, one session then multiple sessions                                                        | 04    |
| 006.9-12                                            | `TestConversationRevisionReceiptMatchesCommit`, complete multi-write receipt/reset, and concurrent-writer tests in source tests                | 01    |
| 006.9-12                                            | Host/gateway tests: 100 agent updates with zero prompt rereads, matching ID updates, coverage-only batches, pagination after applied revisions | 02    |
| 006.10                                              | Drop 10-to-11, deliver 11-to-12, then report revision 12. Assert applied revision stays 10 until repair                                        | 02    |
| 006.9-12                                            | Core tests: matching updates without hydration, turn completion, duplicate/out-of-order batches, and reset recovery                            | 03    |
| 005.1-3, 006.3-12, 007.4                            | SDK tests, rendered fixtures, public-doc validators, spec validation                                                                           | 05    |

All abbreviated criteria use prefix `AC-PLUGINS-PROMPT-HISTORY-HOST-`.
PostgreSQL commands require a disposable `KANDEV_TEST_POSTGRES_DSN`. Skipped PostgreSQL tests do not satisfy the work order.
The migration matrix includes fresh, fully migrated, interrupted backfill, missing legacy objects, repeat boot, rollback, and source mutations after cleanup.

## E2E tests

All tests use managed fresh production builds and isolated fixtures.

| File and project                                                                                              | Required outcome                                                                              | Criteria                            |
| ------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- | ----------------------------------- |
| `tests/plugins/conversation-recovery.spec.ts`, chromium                                                       | Plugin and core recover dropped events and current-state rebind without remounting            | 002.5-6, 002.9, 006.3-6             |
| `tests/plugins/prompt-history-plugin.spec.ts`, chromium                                                       | Public SDK pagination, privacy, alias/favorite display, native navigation, and terminal state | 002.1, 002.4, 002.7, 005.2-3, 006.7 |
| `tests/task/prompt-history-panel.spec.ts`, chromium                                                           | Core ownership and current visible behavior remain valid                                      | 005.3, 006.7                        |
| `tests/plugins/mobile-prompt-history-plugin.spec.ts`, mobile-chrome                                           | Touch panel entry, older-page loading, reconnect repair, alias display, and Chat navigation   | 005.2-3, 006.3, 006.7               |
| `tests/session/session-stream-overload-isolation.spec.ts`, chromium, and its `mobile-` sibling, mobile-chrome | A slow stream cannot block another session and recovery stays bounded                         | 006.6, 006.8                        |

Inject changes between readiness/read, between pages, and before refresh commit through causal fixture controls.
Include equal timestamps, nullable task selection, two panels, deleted page boundaries, direct SQL edits, and no installed plugin.
After hydration, assert zero history rereads during 100 ordinary agent updates to a prompt-only panel.
Then drop one relevant change, deliver a later revision, and assert source repair before the view claims freshness.
Cover empty coverage batches, filter entry/exit, pagination after applied changes, concurrent commit/publication ordering, and bounded-buffer overflow.
These scenarios cover AC-PLUGINS-PROMPT-HISTORY-HOST-006.9 through AC-PLUGINS-PROMPT-HISTORY-HOST-006.12.
Do not use elapsed sleeps as correctness evidence. Assert visible state after controlled mutation and recovery completion.

## Mobile parity

This package changes data synchronization only. It does not change rendered layout, controls, scrolling ownership, or navigation adapters.
The existing mobile panel is the exemplar: Panels entry, inset picker, full-height content, and one vertical scroller.
Existing touch expansion and Chat navigation remain covered in the mobile fixture.
No new ASCII UI preview is needed because no rendered UI change is proposed.

## Work orders

- [x] [Task 01: Add revision-backed source reads](task-01-source-reads.md)
- [x] [Task 02: Switch plugin scopes to source reconciliation](task-02-plugin-reconciliation.md)
- [x] [Task 03: Migrate core conversation delivery](task-03-core-reconciliation.md)
- [x] [Task 04: Remove legacy journal storage safely](task-04-retire-journal.md)
- [x] [Task 05: Prove parity and publish the contract](task-05-parity-and-docs.md)

Execution order is 01 -> 02 -> 03 -> 04 -> 05. All work is sequential.
Shared schema and transport changes prevent independent release or parallel execution.

## Verification results

Initial design and incremental-update refinement validation passed on 2026-09-16:

- `python3 scripts/list-docs.py validate`: 280 decisions and 960 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- All five work orders have valid requirement IDs, design links, and existing source paths.
- Catalog queries discover the replacement design and ADR.
- `git status --short -- docs/plans/conversation-storage-replacement` confirms the new package files are untracked and uncommitted.
  Implementation, migration, performance, and E2E evidence completed on 2026-09-16:

- Backend source, receipt, gateway, plugin, test-harness, and service checks passed, including targeted race runs.
- Frontend source reconciliation, core v2 delivery, Host facade, and isolation tests passed: 64 tests.
- Desktop recovery/parity coverage passed: 6 tests after the source-change harness correction.
- Mobile prompt-history and stream-isolation coverage passed: 2 tests.
- Plugin SDK tests and typecheck passed.
- Public docs, specification, i18n, SQL guard, formatting, and diff checks passed.
- The broad `GOCACHE=/tmp/kandev-go-cache make -C apps/backend test` target was attempted. It reported the pre-existing Office migration fixture failure `TestMigrate_PriorityIdempotent` (`description` is absent) and a separate failure in `internal/task/service`; focused conversation-related package checks passed.
- PostgreSQL variants were not run at this historical checkpoint because this workspace had no `KANDEV_TEST_POSTGRES_DSN`; the follow-up package now records their successful disposable-database execution.
  Exact product commands are in each work order. Do not treat design lint as implementation verification.

## Risks

- Removing the journal before all callers migrate can break core Chat updates.
- Repeated delivery loss can still require expensive loaded-range recovery.
  Use direct updates for normal delivery. Bound recovery and record read counts before release.
- One-time DROP and the existing startup backup can still take time on large databases.
  This package removes payload backfill, not every source of startup I/O.
- Physical database shrink is separate from logical cleanup.
- Old browser wire state requires a full page reload. Mixed backend versions cannot write during cutover.
- File cleanup follows primary commit and needs independent idempotent retry.

## Completion and documentation

Keep all work orders pending until their tests pass.
At completion, record commands/results and mark the paired replacement design current.
Keep requirements active. Do not label requirement documents shipped.
Preserve historical companion results and link new evidence instead of overwriting old counts.
Update `PLUGIN-API.md`, public plugin authoring/manifest reference, and System database maintenance guidance during Task 05.
The implementation portion and the required follow-up gates are complete. The
public pages and active system design were updated during Task 05. Historical
companion plans retain their original context and now link to the replacement
package. The broad `make test` audit still reports unrelated process-probe,
home-config, and launcher failures in this workspace; those failures remain
explicitly recorded in the follow-up package rather than being attributed to
the conversation storage change.


## Review remediation

The user authorized fixes for four review findings and requested no commit.

- Recheck session access, active user status, plugin capability, and installation generation before delivery. Close revoked subscriptions with a terminal notification.
- Check subscribed source revisions every five seconds. Clients wait one second for pending updates before recovery. Equal revisions cause no history reads. Stop checks when subscriptions close.
- Renew short-lived plugin bindings before expiry, share concurrent renewal requests, and retry failed acquisition.
- Load every turn page at one revision before publishing hydrated history. Keep partial results private if a continuation fails.

Focused remediation verification is recorded below. The existing PostgreSQL and broader verification gates remain unchanged. No commit was created.

### Review remediation verification

- Gateway: `go test -race ./internal/gateway/websocket` passed. `golangci-lint run ./internal/gateway/websocket/...` passed with zero issues after extracting subscription validation helpers.
- Frontend: the seven focused source-scope, reconciliation, Host facade, isolation, WebSocket, recovery, and message-handler files passed (91 tests).
- After the final test-only constant extraction, `pnpm exec vitest run lib/plugins/conversation-source-scope.test.tsx` passed (11 tests).
- `pnpm run typecheck` passed. Focused ESLint checks passed without errors; the reported duplicate-string warning was removed and the affected file passed again.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed.
- These fixes change state and transport behavior only. No layout, navigation, scrolling, or touch behavior changed. Targeted tests cover both viewport-independent clients. Browser E2E and PostgreSQL checks were not rerun for this remediation.
- All edits remain uncommitted, as requested.


## Large legacy SQLite verification

The requested large-database verification passed on a 2.95 GB synthetic legacy
database. Backup, cleanup, task-repository initialization, and HTTP readiness
were measured separately. Both 60-second streaming runs completed without
write or readiness/probe failures. Active SQLite triggers, production workers,
and retired host files were audited. Manual compaction and subsequent readiness
passed; no automatic startup VACUUM was added.

See [the verification report](verification/large-sqlite-upgrade.md) for workload,
commands, measurements, evidence, and limits. The public operations guide now
includes explicit post-upgrade compaction. PostgreSQL and other existing plan
gates remain open. All changes remain uncommitted.


## Commit preparation

The six reported SQLite-package lint findings were addressed with constant
reuse, moving unchanged trigger SQL to a package constant, and flattening one
error branch. `go test -race ./internal/task/repository/sqlite -run
'^TestConversation' -count=1` passed. The remaining PostgreSQL, backend failure,
and recovery E2E work will be tracked in a separate follow-up package.


## Committed implementation and remaining work

Implementation is committed as `c0a048bc128f7ef9a1051caf95ed442627faf9df`.
Normal commit hooks passed, including Go lint and zero-warning web lint.
Earlier uncommitted and lint-failure notes describe their historical verification phase.

The [remaining-gates package](../conversation-storage-follow-up/plan.md) owns
the completed PostgreSQL coverage, Office migration fixture correction,
task-service comparison, and desktop/mobile recovery checks after final
remediation. All package work orders now have recorded results. The broad
backend audit has unrelated failures in this workspace and is not presented as
conversation-storage evidence.
No automatic startup VACUUM was added. Physical compaction remains explicit.
