---
id: "01-implementation"
title: "feat(mcp): add durable task message operation readback"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-TASK-MSG-OPID-001
  - REQ-PLATFORM-TASK-MSG-OPID-002
  - REQ-PLATFORM-TASK-MSG-OPID-003
  - REQ-PLATFORM-TASK-MSG-OPID-004
acceptance_criteria:
  - AC-PLATFORM-TASK-MSG-OPID-001.1
  - AC-PLATFORM-TASK-MSG-OPID-001.2
  - AC-PLATFORM-TASK-MSG-OPID-001.3
  - AC-PLATFORM-TASK-MSG-OPID-001.4
  - AC-PLATFORM-TASK-MSG-OPID-002.1
  - AC-PLATFORM-TASK-MSG-OPID-002.2
  - AC-PLATFORM-TASK-MSG-OPID-002.3
  - AC-PLATFORM-TASK-MSG-OPID-002.4
  - AC-PLATFORM-TASK-MSG-OPID-003.1
  - AC-PLATFORM-TASK-MSG-OPID-003.2
  - AC-PLATFORM-TASK-MSG-OPID-003.3
  - AC-PLATFORM-TASK-MSG-OPID-003.4
  - AC-PLATFORM-TASK-MSG-OPID-004.1
  - AC-PLATFORM-TASK-MSG-OPID-004.2
  - AC-PLATFORM-TASK-MSG-OPID-004.3
  - AC-PLATFORM-TASK-MSG-OPID-004.4
system_design:
  - ../../specs/platform/system-design/task-message-operation-identity.md
---

# feat(mcp): add durable task message operation readback

## Outcome and scope

Persist optional message operation identity, sender-scoped readback and durable once-only replay using existing delivery and queue settlement.

Out of scope: New delivery paths, metadata authorization, unknown-outcome replay, and changing sends without operation identity.

## Implementation acceptance

1. The affected native entrypoint follows the linked requirement and design while preserving unrelated behavior and history.
2. Rejected, stale and failed operations retain their existing refusal or rollback semantics.
3. The focused implementation checks and linked documentation coverage have honest recorded results; unexecuted follow-up checks remain pending.

## Files and dependencies

The source files are the existing PR #4720 diff. This work order records its
delivery and links the requirements and system design. The consolidated
maintainer PR also adds permanent handler, SQLite, and PostgreSQL regressions.

- [System design](../../specs/platform/system-design/task-message-operation-identity.md)
- [Plan](plan.md)

## Verification and results

Initial contribution validation: existing message/parent-question/tool-catalog tests and backend wiring build passed. Disposable race-enabled checks exercised actual registered MCP → native scope resolver → dispatcher → real SQLite, lost response after queue acceptance, readback/replay without duplicate delivery, conflicts/forged scope, once-only settlement, reopen and retention. Existing SQLite store/previous-stable upgrade checks passed; live PostgreSQL and provider/process E2E were not run at that stage. SQL guard, docs and all normal commit hooks passed. Existing catalog/upgrade expectations were adapted; no standing tests or CI were added in that phase.

Documentation-only correction: git diff --check and the existing .github/scripts/pr-docs.cjs validateCoverage entrypoint with the candidate changed paths and the linked candidate documents supplied on stdin. No GitHub status is synthesized locally.

## Review follow-up: identity, retained replay and held delivery

Internal row keys are now independent UUIDs. Operation lookup and settlement use
the same byte-wise comparison as their existing identity index, preserving
case-distinct caller IDs without changing the table or retained rows. Sender
validation and exact recorded-request comparison precede mutable target lookup;
a deleted target no longer prevents a pure retained replay.

The existing workflow-promotion queue path now returns its accepted queue ID
through the native service, real backend adapter and MCP settlement. The
original error-only queue method and its admission, marker and drain behavior
remain available. Two existing test fixtures only adapt their method sets; no
standing case is added.

Disposable actual SQLite/MCP checks reproduced all three reported defects
(handlers 0.801s). A row-key-only repair then exposed the related case-insensitive
lookup mismatch (0.260s), so the same comparison was corrected in read and
settlement rather than masking the primary-key error. The fixture changes only
temporary table collation defaults while retaining the real identity index.

The full candidate passed with race detection: handlers 1.958s and the actual
native receipt path 1.579s. Retained legacy-row replay, conflicting request,
foreign principal, fresh missing target, case-distinct settlement isolation and
queue-ID persistence were checked. The real backend adapter package compiled
with no selected runtime test; it is not described as an executed adapter test.
The conditional real PostgreSQL ICU-collation case skipped because the existing
DSN is unset; live PostgreSQL behavior remains unverified.

Existing narrow legacy dispatch, queue-full, parent-interrupt, WIP, terminal,
fast-drain and initial-brief ownership controls passed with race detection
(handlers 2.679s; orchestrator 1.582s). The normal whole-internal SQL guard passed.
No passing check is repeated without a relevant change. Temporary cases remain
outside collection and are removed after delivery; no standing test, operating
setting, model/limit change, fingerprint or new schema is introduced.

## Consolidated maintainer delivery

The latest contributor head already includes the outer pending-state guard
requested in review. The maintainer PR retains that fix and adds the permanent
handler and repository tests prepared during the original review task.

Race-enabled checks passed for selected MCP handlers, server catalogs, and task
repository tests. Coverage includes retained replay, conflicting retries, sender/session isolation,
safe retry readback, and SQLite schema replay and retention. Two PostgreSQL 18
concurrency cases cover duplicate claim election and stale cleanup waiting on
a concurrent successful settlement. Both database
cases executed with `KANDEV_TEST_POSTGRES_DSN` configured. Provider/process E2E
remains outside this change's validation scope.

Shared harness and specification test suites, repository-wide harness and
specification lint, and the targeted harness pre-commit hook passed. The root
engineering guide remains within its 300-line limit.

## Consolidated PR review: retention lifetime

The review reproduced 32 simultaneous detached retention batches. The handler
now permits one batch at a time under the backend lifetime. Shutdown cancels
and drains the batch before closing storage. New claims skip a busy or stopped
worker. Existing shared maintenance admission and batch limits remain in place.

Deterministic race-enabled tests passed for concurrent claims, cancellation,
shutdown drain, and rejection of work after shutdown. Backend wiring and the
existing message-operation regressions passed. The unused row-key argument was
removed without changing identity or replay behavior.
The final combined branch passed 10 desktop and 9 phone browser scenarios.
Normal changed-file hooks passed before the remediation commit.
