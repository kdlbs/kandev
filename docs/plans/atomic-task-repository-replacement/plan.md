---
created: 2026-10-04
status: in_progress
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-002
system_design:
  - ../../specs/tasks/system-design/attach-workspace-source-replacement.md
legacy_specs: []
---

# Implementation Plan: Replace Task Repositories Atomically

## Overview

One sequential work order repairs the complete repository association replacement from service input
through the dialect-aware store and registered transport results/events. Preparation happens before
association writes; canonical inheritance and delete/all inserts happen under one task-scoped database
transaction. The owning [requirement](../../specs/tasks/requirements/attach-workspace-sources.md) is
`REQ-TASKS-ATTACH-WORKSPACE-SOURCES-002`, not a new incident or UI specification.

## Evidence and assumption check

Confirmed main/checkout: `b767cee4108a0e823556624c3729a1fc2240c3af`; branch at design:
`feature/replace-task-reposit-zsc`. ROOT's archived proof is accepted without replay:
`/tmp/kandev-task-repository-replacement-repro_test.go` and
`/tmp/kandev-root23-replacement-proof-receipt.json`. Real private SQLite service proof
`TestRootTaskRepositoryReplacementProof`, race/fts5, handle 77276 **actually joined exit 1**:
test 0.270s/package 0.343s. Direct/update late resolver failure retained `[]`; direct/update
valid second insert aborted by SQLite trigger retained `[repo-a]`. Each should preserve exact
`repo-original`. Successful two-repository UpdateTask control passed. ROOT removed the disposable
in-repo test; proof files are read-only until ROOT independently verifies merge and cleans them.

Verified source root cause: private replacement reads/preserves/validates, commits
`DeleteTaskRepositoriesByTask`, then resolves and loops independently committed inserts. The
provider-only update preflight does not cover complete resolution. Existing atomic source-batch and
raw task-row-lock patterns provide the local precedent; the current association interface provides
no complete replacement or service transaction seam.

Confirmed scope: preserve the association set, typed errors, policies, checkout behavior and order;
keep F19 runner mutability bypass. Task field edits before replacement and fresh-branch Git operations
remain separate. No material unanswered decision blocks the package. Selected local design: one
pure finalizer inside a domain-specific replacement store operation, with side-effectful repository
resolution outside. This is recorded in the design, not a new ADR.

## Companion package audit

The owning design links existing packages: attach-workspace-sources (completed, 14 orders),
restore-live-add-branch (completed, 3), inherited-live-add-branch (implemented, 1),
multi-repo-chat-file-links (completed, 2), and owned-link-target-mismatch-repair (completed, 5).
Runner-switch-before-materialization is completed with 1 order. This repair changes neither their
materialization/link/UI scope nor their E2E matrices or completed receipts. Preserve those records;
the new association replacement work is tracked solely by this package. Multi-branch migration/draft
history and current launch-resolution residuals remain unchanged.

## Scope

### In scope

- Complete preparation, locked canonical policy/checkout finalization, one commit for full/empty
  replacement, parent existence and SQLite/PG serialization before canonical association reads.
- Real database direct/update RED/GREEN, storage/cancellation/compatibility/concurrency tests,
  registered REST/WS results and success-event suppression on failure, scoped SQLguard/conformance.
- Owning docs and a short failure-boundary clarification in the existing public task guide.

### Out of scope

Runner gating, launch inventory policy, unrelated task-field rollback, repository/Git compensation,
CreateTask insert atomicity, generic transaction APIs, schema/migrations, stable successful row IDs,
legacy writer redesign, global revision/event ordering, UI/copy/localization/browser/app/build/E2E,
broad local product suites, agents/model/session changes and automatic retries.

## Technical approach and compatibility matrix

Use `TaskRepoRepository`, `sqlite.Repository`, existing raw task lock and transaction/query helpers.
The single work order owns service preparation/finalization plus store seam, dialect tests and
registered transport proof together. Keep provider preparation out of a held SQLite writer to avoid
self-deadlock when resolution creates repository entities. The design specifies deferred policy
outcomes and tx-observed environment existence; neither a pre-lock inheritance read nor a blind
rows-only delete/insert transaction satisfies this read-modify-write boundary.

| Input / transport / dialect | Identity and behavior | Verification / unsupported fallback |
| --- | --- | --- |
| Existing ID | Task workspace lookup, safe worktree redirection, typed reference error | Direct/update invalid-later-ID and cross-workspace cases; preserve exact original rows |
| Local path | Workspace/canonical-path resolver and default branch | Valid local-path resolution plus late failing input; entity side effects are outside association rollback |
| GitHub/GitLab/Azure URL | Built-in provider identity and existing branch/metadata conventions | Reuse remote-resolution compatibility tests and targeted preparation cases; unsupported host keeps existing error |
| Plugin provider selection | Existing preflight trusted descriptor/scoped provider resolver | Test existing typed provider failure at update boundary; no new public trust bit or provider API |
| Policy/checkout/contribution | Canonical inherited immutable policy; existing explicit snapshot, base and option rules | Edited/deleted policy, branch rewrite, omitted/equal/changed options, ambiguity and requested metadata cases |
| REST PATCH / WS task.update | Registered router/dispatcher, real service/database, omitted/null versus [] | Failure DB rows plus no success evidence; full/empty success response/event |
| SQLite | Writer-before-read no-op task UPDATE and tx-only canonical reads | Shared pool and independent file-backed writer tests; contention may fail safely, no retry promise |
| PostgreSQL | Separate raw task row lock then READ COMMITTED canonical read | Independent physical holder/worker/observer, server-observed wait then committed predecessor inheritance |

## Regression and acceptance coverage

All proposed new names below are exact command anchors, not executed evidence. Implement permanent
regressions in ordinary repository files after ROOT's later implementation interrupt.

| Acceptance | Test and evidence |
| --- | --- |
| 002.1 | `TestTaskRepositoryReplacementPreservesOriginalSet`: direct/update x late resolver/second valid insert failure, two original rows with all columns and nested sentinel metadata; SQLite trigger `RAISE(ABORT)` after first new row succeeds. `TestTaskRepositoryReplacementStoreRollback`: delete/late insert/serialization/finalizer/constraint failure and reopened persistence |
| 002.2, 002.3 | `TestTaskRepositoryReplacementCompleteSet`: direct/update successful ordered multi-row/multi-branch replacement, explicit clear/already-empty, update omitted/null; registered transport same cases |
| 002.4, 002.7 | `TestTaskRepositoryReplacementCompatibility`: edited/deleted/explicit policies, generated fresh base with PreserveBaseBranch, complete policy fields, checkout omission/equal/change-after-environment/ambiguous branches, supported PR/contribution metadata and typed cross-workspace/duplicate/invalid inputs |
| 002.5 | `TestTaskRepositoryReplacementSQLiteSerialization`: independent writers on one file and shared writer pool, canonical builder not entered before acquisition, complete predecessor inheritance, no partial union. `TestPostgresTaskRepositoryReplacementSerialization`: independent PIDs and `pg_stat_activity`/`pg_locks` wait evidence before release, changed predecessor snapshots visible after lock, replacement-vs-replacement winner complete |
| 002.1, 002.8 | `TestTaskRepositoryReplacementCancellation`: already-canceled preparation, waiting lock cancellation, cancellation after a staged insert before commit via a real transaction boundary; exact original rows and joined workers. `TestPostgresTaskRepositoryReplacementRollback`: valid late insert failure and lock-wait cancellation with exact row retention |
| 002.6 | `TestRegisteredTaskRepositoryReplacement`: RegisterTaskRoutes REST + dispatcher WS with real private SQLite; late resolution/trigger failure and full/empty/omitted/null control; capture service event publication and inspect response classification/payload |
| 002.8 | `TestTaskRepositoryReplacementBoundary`: UpdateTask title remains committed while failed replacement retains old rows; fresh-branch handler replacement storage failure keeps original DB row and existing 5xx after already performed Git work, no Git rollback assertion |

Test files are listed in the work order. Use production operations and real SQL constraints/triggers,
not business-store failure mocks or synthetic semantic assertions. The first permanent service
regression compiles against unchanged code and fails on observed row loss. Store/transport coverage
then proves the designed seam and integration. Existing F19, snapshot, checkout and ordering tests
are explicit compatibility controls. Public user-facing proof is the registered transport integration;
no rendered interaction changes justify an artificial browser test.

## Work orders

- [ ] [Task 01: Replace task repository associations atomically](task-01-atomic-replacement.md)

One order, wave 1, no dependencies; sequential in this primary session. No delegation.

## Verification strategy

Exact resource-capped commands, working directories and stopping rules are in the
[work order](task-01-atomic-replacement.md#verification). First-turn validation is limited to catalog,
spec lint, reference coverage and whitespace. No Go test, fixture or install is run during design.

Local PostgreSQL skips are never executed proof. Use an existing authorized test DSN or an owned
private fixture after implementation release; otherwise extract actual hosted PostgreSQL test results
and server-wait assertions at the published head. Before allocating a private fixture, record its
planned unique name/ownership labels/data paths; immediately after allocation record exact container
ID, labels and **all** mounts/volumes before any test. Register bounded cleanup then. Prefer isolated
PGDATA tmpfs to avoid anonymous volumes; inspect actual mounts regardless. Remove only identities
proven owned, including any owned volume/bind data, and actually join cleanup. Never remove older
containers or dangling volumes. Testutil isolated schemas/connections also need complete teardown.

## Risks

- A callback that resolves a repository or uses `r.ro` can deadlock the SQLite single writer or use a
  stale canonical snapshot. Keep callbacks pure and reads on tx; prove independent connections.
- Eager policy lookup failure can reject a legitimate deleted-policy snapshot; finalize precedence
  under lock and use stored typed errors only when no snapshot applies.
- A no-op task UPDATE is required for independent SQLite writer-before-read, unlike the current
  helper's deliberate no-op; do not alter unrelated task timestamps or runner eligibility.
- Existing legacy single-row/compensation writers are not redesigned. Their broader concurrency and
  launch inventory races stay explicit residuals, not claims this fix closes.
- Fresh-branch Git work and ordinary UpdateTask fields can remain on association failure. The public
  guide and boundary regressions must not overstate the transaction scope.
- Published-head review/CI limits are hard gates; resource timeout, unrelated CI failure or missing
  authenticated full semantic evidence requires a ROOT checkpoint, not weakening or blind retries.

## Verification results

Design checks passed on 2026-10-04: `python3 scripts/list-docs.py validate` validated 347 decisions
and 1332 specifications; `python3 scripts/lint-spec-files.py --all` passed. `git diff --check` passed.
The four artifacts pass explicit trailing-whitespace/newline and reference checks, including
unstaged/untracked files. `.github/scripts/pr-docs.cjs.validateCoverage` returns exempt for the actual
docs-only diff and covered with zero errors for the prospective owned service edit, validating the
work-order/plan/design/requirement/eight-criterion chain. These are documentation preflights, not
product test evidence. The initial `node` invocation was unavailable on PATH (exit 127); the same
preflight ran successfully using existing `/home/jcfs/.nvm/versions/node/v24.18.0/bin/node`, without
installation. These statements describe the completed design checkpoint; ROOT later released implementation.
The new design is now current for the reviewed contract, and Task01 is in progress. Preserve
all unrelated multi-branch draft/migration/history status and existing source materialization behavior.

ROOT released Task01 in a later implementation continuation. The domain transaction and actual caller
regression matrix are implemented. See [Task01 Results](task-01-atomic-replacement.md#results) for
joined RED/GREEN, compatibility, registered transport and real database conformance receipts.
The pinned full CHANGED lint hit its GNU 6m limit and exited 124 without diagnostics; local lint
remains FAILED. ROOT authorized one identical bounded warm-cache recovery: handle 44426 joined
exit 1 with four new-test lint findings. ROOT then authorized their minimal test-only correction;
six affected service scenarios and twelve REST/WS cases passed, and corrected-code full CHANGED
lint handle 85224 joined exit 0 with zero issues at the same base and bounds. The first timeout's
cause remains unproved; it is not cleanliness evidence.
Published-head hooks/CI/full semantic review/expected-head squash/independent content verification
remain external completion gates recorded in the existing Kandev task plan.

## Handoff and delivery constraints

Design ends before implementation. ROOT reviews this concrete package and later sends the explicit
implementation continuation with delivery_mode=interrupt. Keep primary task/session identity and
user-edited Kandev task plan. No operator approval/model-switch question.

After release, follow TDD then affected checks and normal active hooks; one pinned pnpm 9.15.9 frozen
install from apps only if missing dependencies. Commit/push/ready PR and normal expected-head squash
are authorized after task checks. Authenticate configured CodeRabbit App 347564 full semantic report,
all changed files, every finding disposition. A completed current-head full report is sufficient;
Claude 0s ACK is not semantic evidence. Freeze published SHA absent a real corrective finding. Never
rebase because main moves or restart near/completed CI. Use one retained scripts/pr-await terminal
waiter (90m practical), actually join it; no duplicate/manual timer polling. Notify ROOT before
out-of-package changes or rerun for unrelated CI evidence; only ROOT authorizes a specific same-head
job rerun. A later pass establishes occurrence transience, not a proven cause.

Use cheap final live-main merge-tree/content comparison. Completion requires actual expected-head
normal squash plus independent merge SHA/parent/tree/all owned blob/remote inclusion receipts and
joined proven-owned cleanup. Leave managed worktree/dependencies clean for ROOT archival. Child
callbacks to ROOT are queued; ROOT interactions are interrupt. Persist resources and crash-recovery
next action in the existing Kandev task plan; keep parent proof read-only through ROOT verification.
