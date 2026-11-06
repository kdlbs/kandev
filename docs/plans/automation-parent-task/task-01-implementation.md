---
id: "01-implementation"
title: "feat(automation): support optional parent tasks"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATION-TARGETS-004
acceptance_criteria:
  - AC-OFFICE-AUTOMATION-TARGETS-004.1
  - AC-OFFICE-AUTOMATION-TARGETS-004.2
  - AC-OFFICE-AUTOMATION-TARGETS-004.3
  - AC-OFFICE-AUTOMATION-TARGETS-004.4
  - AC-OFFICE-AUTOMATION-TARGETS-004.5
  - AC-OFFICE-AUTOMATION-TARGETS-004.6
system_design:
  - ../../specs/office/system-design/automation-target-modes.md
---

# feat(automation): support optional parent tasks

## Outcome and scope

Support an optional validated parent for both automation target modes, preserve retry snapshots, clear future continuation when the parent changes, and expose explicit relationships.

Out of scope: Changing ordinary board filtering, rewriting old tasks/history, and exporting instance-local parent IDs.

## Implementation acceptance

1. The affected native entrypoint follows the linked requirement and design while preserving unrelated behavior and history.
2. Rejected, stale and failed operations retain their existing refusal or rollback semantics.
3. The focused implementation checks and linked documentation coverage have honest recorded results; unexecuted follow-up checks remain pending.

## Files and dependencies

The source files are the existing PR #4723 diff. This work order adds its delivery record and links existing requirements and system design; it introduces no runtime gate or permanent test.

- [System design](../../specs/office/system-design/automation-target-modes.md)
- [Plan](plan.md)

## Verification and results

Validation: 25 existing frontend tests and changed-file ESLint passed. Focused Go checks plus disposable real SQLite journeys covered both task modes, persisted ParentID and relationship readback, invalid/foreign/nested parents, update/clear/unchanged continuation, retry snapshots and export omission. All normal commit hooks passed after a cache-warming retry of a zero-issue lint timeout. Temporary fixtures were removed; existing web fixtures were adapted without new standing test cases or CI.

Documentation-only correction: git diff --check and the existing .github/scripts/pr-docs.cjs validateCoverage entrypoint with the candidate changed paths and the linked candidate documents supplied on stdin. No GitHub status is synthesized locally.

### Review follow-up

The original PR head reproduced stale-parent disable refusal, older firing writes
after parent change/clear/ABA or a newer binding, and reuse of a task belonging
to another parent. The same temporary fixtures exercise the real task-preparation
entrypoint and SQLite store. The fixes retain immutable older firings while
protecting only the current continuation selection. Existing unscoped setter
calls, parent validation on changed selections/enabling, and task history remain.

An actual picker fixture reproduced typed input never reaching workspace search
on the original source. The candidate selects a task beyond the first 50 results,
retains its selected label, and rejects late workspace responses. The shared
command subtree is factored to retain the existing function-size limit.

Candidate checks passed: automation 0.090s and orchestrator 0.425s including the
focused existing continuation/retry selections; two temporary UI cases plus four
existing Combobox cases; changed-file ESLint; full web typecheck. The first
typecheck exhausted the process's default 4 GiB Node heap; the unchanged command
passed with process-local 8 GiB and measured 30 GB available RAM. No global
settings, runtime automation or CI limits changed. No standing tests were added.
Normal commit hooks and final source identity are recorded in the source handoff.

## ASCII UI preview

UI-01: Existing affected surface, after this change.

```text
Automation editor
  Parent task (optional): [Search tasks]
  Target: hidden run | normal task
  [Save]

Clearing the picker removes future parent assignment. Desktop and phone keep the existing editor and save flow.
```

This sketch records the affected structure; spacing is illustrative. It does not add a new control or approval requirement.


## Committed retry adoption after parent removal

Parent validation belongs to the branch that will create a new task, after
existing continuation/committed-task resolution. The retained task's original
parent can have been deleted without cascade; its historical committed task
identity still governs retry adoption. The child is neither recreated nor
reattached, and immutable retry snapshots plus future continuation CAS remain.

A disposable fixture uses real SQLite scoped child reparenting and parent
deletion, then the existing committed retry-operation fixture. The original
source fails adoption when the captured snapshot includes the deleted parent;
the fresh-creation refusal control passes. This is task-repository and dispatcher
proof, not a new full live-provider or complete durable-ledger E2E run.

The corrected immutable-parent baseline failed committed adoption (0.320s),
while refusal of fresh creation with the removed parent passed. The candidate
passed both variants and three existing adoption/ambiguous-lookup controls
under race (1.503s). The first fixture attempt omitted the newly captured parent
field and was corrected before treating its result as product evidence.

Ten additional existing retry controls passed under race (1.599s), including
real retry-store ledger, transient lookup/binding failures, foreign identity,
retry fences and post-dispatch preservation. No already-passing candidate
cases were repeated.
