---
created: 2026-10-30
status: done
requirements:
  - REQ-TASKS-TURN-CHANGES-002
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-004
  - REQ-TASKS-TURN-CHANGES-005
  - REQ-TASKS-TURN-CHANGES-006
system_design:
  - ../../specs/tasks/system-design/turn-change-history-integrity.md
legacy_specs: []
---

# Fix plan: Turn continuity after a generated-branch rename

## Overview

Repair [issue #4961](https://github.com/kdlbs/kandev/issues/4961) by retaining proven branch identity through title resolution.
Deliver the guarded backend path first, then prove transcript and historical review behavior on desktop and phone.
Tasks owns the package because the failed contract is the comparison of a durable turn interval.

The issue is assigned to `carlosflorencio`.
Both work orders are implemented; their Results sections record the behavioral RED, regression, and browser evidence.

## Evidence and root cause

The inspected checkout is `b767ca83fd3792f2e0fbc66909dce32af3cf2dff`.
Current main was `edebfa5241ccaedde422844c883a1ff31360ca21` during investigation.
The compared history, title-rename, provenance, and owning specification files are identical at both heads.
If main moves, refresh this evidence before implementation.

`handleSetTaskTitle` accepts the title before `RenameGeneratedBranchesForTaskTitle` runs.
The orchestrator verifies a generated branch against repository-scoped environment metadata.
Its runtime call currently carries only the new name, repository, and primary marker.
Agentctl therefore receives no expected generated-branch identity.

`resolveTurnCheckpointComparison` calls `turnCheckpointContextChange` before direct eligibility.
The context check rejects `start.HeadRef != end.HeadRef` unconditionally.
`isTurnChangeCardVisible` exposes unavailable history even with zero file entries.
The frontend follows its existing contract; the failed backend classification causes the unwanted card.

A temporary Go overlay exercised real capture and v2 comparison with existing Git fixtures.
`TestIssue4961Diagnostic` passed five subtests that assert the current behavior:

| Case | Real HEAD equal | Snapshot trees equal | Current result |
| --- | --- | --- | --- |
| No operation | yes | yes | `direct`, complete, zero files |
| Shell branch rename | yes | yes | `history_unavailable`, `history_context_changed` |
| Existing API branch rename | yes | yes | `history_unavailable`, `history_context_changed` |
| Rename plus a local edit | yes | no | `history_unavailable`, zero reported files |
| Switch to another branch at the same commit | yes | yes | `history_unavailable`, `history_context_changed` |

The probe added no repository source or permanent tests.
This evidence confirms the defect and the need to distinguish renames from branch switches.
It does not validate the proposed implementation or a full browser flow.

Smallest reproduction: use `setupHistoryTarget`, capture start on `feature/provisional`, rename it, capture end, then call `compareHistoryEndpoints`.
The permanent behavioral RED test will use the existing rename API between captures and expect a complete zero-file direct comparison.
The generic API currently lacks generated-rename intent, so the test must introduce that intent as setup before the RED assertion.
Use a real HTTP agentctl fixture to exercise the existing route; an ignored optional field still reaches today's failing comparison.

## Scope

### In scope

- AC-TASKS-TURN-CHANGES-002.14 and .15 extend repository interval accuracy.
- Existing AC-002.3, .4, .12 and .13 constrain file edits, capture invariants, unsafe history, and legacy behavior.
- Existing AC-003.1 through .3 and AC-004.1 constrain boundaries, immutable receipts, and replay.
- Existing AC-005.6, .7 and .13 and AC-006.1, .3, .5 and .7 constrain transcript visibility and historical review.
- Guarded generated-branch intent through orchestrator, runtime, client, API, and checkout operator.
- Bounded branch-binding evidence in v2 receipts, with real Git and transport regression coverage.
- Desktop and phone tests through the real task-title MCP handler.

### Out of scope

- Rewriting accepted comparisons, repairing historical database rows, or backfilling proof.
- Changing manual rename semantics, branch templates, title ownership, or remote branch names.
- Hiding unavailable-history cards globally or changing card styles and localized copy.
- New runtime flags, SQL migrations, executor implementations, or a branch-identity persistence service.
- Commits, pushes, PR creation, persistent Kandev subtasks, or delegated implementation.

## Technical approach

Use the [history design](../../specs/tasks/system-design/turn-change-history-integrity.md#generated-branch-continuity)
and [proposed decision](../../decisions/2026-10-30-turn-checkpoint-branch-binding.md).

### Guarded title rename

Change the narrow title runtime seam in `task_title_branch_runtime.go` to require the expected generated branch and explicit primary identity.
Add the proposed lifecycle method `RenameGeneratedBranchForSession` in `manager_workspace_branch.go`.
Keep its metadata and running-snapshot updates on the existing successful-operation path.
The new client method sends `expected_generated_branch` on `/api/v1/git/rename-branch`.
Ordinary `GitRenameBranch` requests retain their existing payload and cannot transfer binding identity.

Agentctl checks actual symbolic HEAD against that expected old branch inside the capture lock.
Use explicit old/new operands for Git, then verify the new ref and unchanged commit.
An unexpected checkout must not cause a different branch to be renamed.
Failures leave the accepted task title intact and retain existing repository-scoped outcomes.

### Immutable branch-binding evidence

Add optional `HeadBindingID` to `common/turnchanges.Provenance`.
Keep one synchronized `{ref, UUID}` state per cached `GitOperator`.
An observed ref change without the guarded operation creates a fresh binding.
Only a successful guarded rename transfers the matching old binding.
Capture persists the ID and checks it again during provenance verification.

Extract one comparison predicate for stable branch binding.
Use it in context selection and unborn-first-root branch eligibility.
An ID match relaxes only the ref-name check.
All other target, object, ancestry, operation, conflict, and capability checks still run.
Both endpoint bodies retain actual refs and their original commit/tree identities.

Missing, malformed, mismatched, cross-checkout, or interrupted binding evidence must never authorize continuity.
After both endpoints exist, replay uses their immutable evidence with a fresh operator.
After agentctl replacement between a rename and end capture, missing live proof produces unavailable history.
Do not restore a token from an old start receipt or infer a rename from current refs.

### Compatibility and executor coverage

| Boundary | Transport and identity | Intended result | Evidence and fallback |
| --- | --- | --- | --- |
| Local/worktree task with current agentctl | Existing HTTP rename, expected old branch, per-checkout binding | Valid title rename preserves comparison | Real Git, API, runtime and browser regressions |
| Ordinary manual/API rename or branch switch | No generated intent or different observed binding | Unavailable when symbolic refs differ | Same-commit negative tests |
| Multi-repository task | Existing repository subpath and primary marker | Only eligible owner checkout transfers identity | Mixed generated/explicit-checkout fixture; untouched roots retain their state |
| Retained older agentctl or old endpoint | Optional request field ignored or ID absent | Existing conservative history result | Omitted-field/old-receipt fixtures; no fabricated successful zero |
| Older backend with newer agentctl | Generic rename payload | Existing behavior | Generic payload compatibility test |
| Container, SSH, Kubernetes, Sprites | Shared executor-local HTTP path, when supported | Same guarded contract | Shared transport coverage only; no new real-executor qualification claim |
| Replaced agentctl during the interval | Live binding lost | Unavailable for changed refs | Replacement fixture; finalized receipt replay remains valid |

### Documentation

Update existing specifications and add this delivery record now.
During implementation, add a short continuity statement beside title-driven renaming in `docs/public/git-operations.md`.
That page is a reference. The public update belongs with the implemented behavior.

The completed [history-integrity package](../turn-change-history-integrity/plan.md)
and [title-rename package](../agent-title-branch-renaming/plan.md) retain their recorded results.
This package adds the missing interaction tests; it does not reopen their unrelated work orders.

## ASCII UI preview

### UI-01: Completed rename-only turn, desktop and phone

Entry point: task transcript after title resolution and terminal capture.
Source currently keeps the unavailable-history exception visible.

```text
Before:
[Final assistant reply]
[This turn's change summary is unavailable.]

After:
[Final assistant reply]
[Next transcript item, when present]
```

The required outcome is the absent card after a complete zero-file comparison.
Desktop and phone share this composition. The transcript keeps its current scroll owner.
Map: AC-002.14, AC-005.6 and .7.

### UI-02: Renamed turn with one file edit

```text
Desktop:
[Final assistant reply]
[1 changed file +1 -0                 Open diff]
[turn.txt                            Added +1]

Phone:
[Final assistant reply]
[1 changed file +1 -0]
[Open diff]
[turn.txt       Added +1]
        |
        +--> [Existing full-height diff drawer]
```

Counts and spacing are illustrative. The existing card, desktop dock, and `MobileDiffSheet` are the required surfaces.
Phone actions retain 44-pixel targets, safe-area clearance, internal scrolling, and focus return.
Shared projection and selection own the data; responsive wrappers retain their current composition.
A real branch switch keeps the existing unavailable state on both viewports.
Map: AC-002.14 and .15, AC-005.13, AC-006.1, .3, .5 and .7.

## Tests

New names below are proposed regression tests.

| Criteria | Test and location |
| --- | --- |
| AC-002.14; AC-003.1-.3 | `TestHandleGitRenameGeneratedBranchPreservesTurnComparison` in `agentctl/server/api/git_generated_branch_rename_test.go`; real route between captured endpoints |
| AC-002.3, .4, .14; AC-006.3 | `TestTurnCheckpointGeneratedBranchRenameWithEdits` in `process/turn_checkpoint_branch_binding_test.go`; exact files/counts and exported patch |
| AC-002.12, .15 | `TestTurnCheckpointBranchBindingRejectsUnprovenChanges`; same-commit switch, recreated original branch name, generic rename, missing/malformed IDs, target change, active operation, and unsupported history |
| AC-002.4, .14; AC-003.1-.3 | `TestTurnCheckpointBranchBindingRenameRace`; expected-ref mismatch, failed rename, and capture/rename serialization with channel barriers |
| AC-004.1; AC-002.13 | `TestTurnCheckpointBranchBindingReceiptReplay`; original receipts, fresh comparison operator, legacy fields, and process loss between endpoints |
| AC-002.14, .15 | `TestRenameGeneratedBranchForSessionUsesExpectedBranch` and extended `TestRenameGeneratedBranchesForTaskTitleScopesMixedRepositories`; owner, repo and primary scoping |
| AC-002.14; AC-005.6-.7 | Desktop/phone `rename-only turn has no changes card` through real title MCP |
| AC-002.14; AC-004.1; AC-006.1, .3, .5, .7 | Desktop/phone `renamed turn retains its one-file historical diff` with reload and later workspace edit |
| AC-002.15; AC-005.7 | Desktop `same-commit branch switch remains unavailable` |

The mixed-repository test contains at least one eligible generated checkout and one explicit checkout.
Assert each repository independently; one valid binding never authorizes every root.

## E2E tests

Add `turn-changes-title-rename.spec.ts` to the `chromium` project.
Add `mobile-turn-changes-title-rename.spec.ts` to `mobile-chrome`.
Reuse existing turn-change helpers, page objects, and the shipped historical drawer.
Use a mock-agent scenario that calls the real task-title MCP tool before optional filesystem edits.
The negative scenario switches branches after the verified rename, keeping HEAD unchanged.
Do not seed a ready summary or manufacture comparison results in SQL.

## Work orders

- [x] [Task 01: Preserve proven branch bindings through title renames](task-01-preserve-branch-binding.md)
- [x] [Task 02: Prove transcript and historical review behavior](task-02-title-rename-browser-coverage.md)

Execute 01 then 02 sequentially in the primary conversation.

## Verification results

Investigation: five diagnostic subtests passed in 5.08 seconds against the existing implementation.
The probe ran with `go test -trimpath -overlay=<temporary-overlay> ./internal/agentctl/server/process -run '^TestIssue4961Diagnostic$' -count=1 -v`.
The temporary overlay and source were removed after the probe.

Design verification on 2026-10-30:

- `python3 scripts/list-docs.py validate`: passed; 421 decisions and 1848 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- PR-documentation `validateCoverage`: actual documentation-only diff was exempt.
  A simulated implementation trigger validated both work orders and their plan, requirement, acceptance, and design references as covered.
- Scoped `git diff --check` passed. Git status includes both untracked work orders and the proposed decision.
- GitHub assignment was read back successfully for `carlosflorencio`.

Implementation and browser checks on 2026-10-30:

- Behavioral RED: `TestHandleGitRenameGeneratedBranchPreservesTurnComparison` failed with
  `history_unavailable` / `history_context_changed` before the guarded rename, then passed with a
  complete direct zero-file comparison.
- Review follow-up RED: a verified rename to `feature/b` followed by recreating `feature/a` at the
  same HEAD produced a direct comparison until the equal-ref/different-ID rule was added; it now
  returns unavailable history. Legacy receipts without IDs retain the equal-ref fallback.
- Fixup follow-up RED: a verified rename, then an ordinary rename through an intermediate name, then
  recreating that intermediate name reused the stale binding until ordinary renames cleared it; the
  regression now returns unavailable history.
- Backend: the work-order `go test -race` command passed for `turnchanges`, `process`, `api`,
  `agentctl`, `lifecycle`, `orchestrator`, and `mcp/handlers`; `backendapp` compiled; changed-Go
  lint reported 0 issues; `git diff --check` passed.
- Mock agent: all six `TestTurnTitleRenameScenario` cases passed, including MCP failure,
  rejected/preserved rename, wrong branch, and malformed-result refusal.
- Browser: desktop `chromium` passed 3 cases (27.5 seconds); phone `mobile-chrome` passed 2 cases
  (24.1 seconds) after the equal-ref/different-ID review correction.
- Documentation: public-docs validation passed (63 tests, 54 pages); 421 decisions and 1848
  specifications validated; spec lint passed 36 tests and the full file scan.
- No explicit skips.

## Risks

- A matching HEAD commit is insufficient proof; a relaxed ref check can silently accept branch switches.
- Process-local evidence cannot bridge agentctl replacement during an unfinished renamed interval.
  Explicit unavailability is the required fallback; retained endpoint evidence remains replayable.
- Git and external writers can race. Pre/post checks fail closed, while existing shared-checkout attribution remains authoritative.
- Existing large test files approach lint limits. Add focused test files instead of growing those files.
- Browser tests need real generated worktrees and title-owner admission; a Local executor or synthetic summary cannot prove the repair.
- No full fresh-instance browser reproduction ran during planning. Task 02 supplies that evidence after implementation.
