---
created: 2026-10-06
status: implemented
requirements:
  - REQ-WORKSPACES-SAVED-FILE-CONTENT-001
  - REQ-EXECUTORS-SURVIVAL-001
  - REQ-EXECUTORS-SURVIVAL-004
system_design:
  - ../../specs/workspaces/system-design/saved-file-content.md
  - ../../specs/executors/system-design/agent-survival-across-restart-03.md
legacy_specs: []
---

# Implementation Plan: Prevent Overlapping File Saves

## Overview

Preserve both submitted edits when distinct-file saves overlap. One sequential
work order first reproduces the actual loss using real Git and current caller
payloads, then isolates request patch files, verifies cleanup and compatibility,
and checks the registered HTTP outcome. ROOT reviewed the four-artifact package
after the actual design handoff and released implementation in this primary.

## Confirmed root cause and evidence

`ApplyFileDiff` writes shared workspace `.kandev-patch.tmp` before Git admission.
Alpha waits; beta replaces the patch; alpha applies beta's patch and acknowledges
its own unchanged file as applied. The editor trusts success/new_hash and marks
its submitted snapshot clean when no later edits occurred.

Accepted ROOT invocation8566 (initial18fef5, terminal483150) actually joined
exit1 at 2026-10-06T22:14:01Z after 15.706s: alpha edit missing/false ACK,
beta and both sequential controls pass. Evidence archive:
`/tmp/kandev-root-file-save-patch-isolation-next-candidate_test.go`, mode0400,
SHA256 `2bfcaca5583d0db3119dbacea94f8bf11a852d343d3d96a3aee567e801514895`.
Read only; never replay/copy/import/modify/remove. Receipts and classification are
`/tmp/kandev-root-file-save-patch-isolation-proof*`. The proof source is not a
repository artifact or permanent regression. All proof-owned groups are gone.

Admission base is `62b39941214ffe63ce72d307b6e599a7bd2a7b63`; relevant agentctl
server and actual save caller bytes match proofbase
`72a840a860143c6b55e401fe27c92167415b1e4a`. No moving-main rebase is planned.

## Scope and technical approach

- Own only the `ApplyFileDiff` temporary patch lifecycle and focused process/API
  tests. Use private create-temp, checked descriptor write/close, absolute patch
  argv, and per-invocation best-effort removal.
- Preserve desired-content fallback, hash conflicts, cancellation, symlink
  rewriting, Git admission/execution budgets, and notifications.
- Exclude same-file order/merge policy, global locks, frameworks, schema/API/UI/
  store/database changes, runtime/harness changes, and the separate ROOT-owned
  header-target candidate. No broad audit, Go suite, browser, E2E, or build.

## Tests and traceability

| Criteria | Planned test in process/API package |
| --- | --- |
| .1, .2 | `TestApplyFileDiff_ConcurrentDistinctFiles` (shared/independent trackers), `TestApplyFileDiff_SequentialDistinctFiles` |
| .2, .3 | `TestApplyFileDiff_PatchCleanup`; existing regular-file, symlink and conflict tests |
| .4 | `TestApplyFileDiff_CancelledQueuedSave`; cleanup and independent peer completion |
| .1, .2 | `TestHandleFileUpdate_ConcurrentDistinctFiles`, `TestHandleFileUpdate_SequentialDistinctFiles` through the registered router |
| .3 | Existing `TestHandleFileUpdate_ReportsHashConflict` and `TestHandleFileUpdate_Rejections` |

All IDs above use `AC-WORKSPACES-SAVED-FILE-CONTENT-001` as their prefix. Exact
paths, anchored selectors and commands are in the work order. Expected hashes
must use an independent standard-library SHA256 oracle, not the producer helper.

## End-to-end operation evidence

Real `ApplyFileDiff` plus real Git proves disk/results; the existing registered
HTTP fixture proves producer-to-response forwarding. Source inspection explains
the editor's clean-state implication. No browser, WebSocket execution, backend
database persistence, or phone rendering is claimed or scheduled. Mobile-parity
and public-doc assessments are recorded in the owning design.

## Work orders

- [x] [Task 01: Isolate save patches](task-01-isolate-save-patches.md) (`done`, sequential)

## Verification results

Design validation passed: catalog validation (357 decisions, 1414 specs), all
36 spec-linter tests, full spec lint, catalog discovery of the new pair, and
tracked diff whitespace checks. Static four-artifact traceability was checked.
The original local Node invocation failed because `node` was unavailable on its
PATH. ROOT then authorized exactly one bounded recovery with the existing Node
24.21.0 binary: the exported-evaluator prospective preflight passed. That
recovery made no repository or dependency changes.

Implementation RED independently reproduced the missing alpha edit and false
applied hash for shared and independent trackers, with both sequential controls
passing. GREEN passed. Final race-enabled checks passed all ten process tests and
five registered HTTP tests, and exact-base scoped lint returned zero issues.
Detailed command/results are in the work order. Implementation catalog validation,
all 36 spec-linter tests, full spec lint, diff whitespace checks and actual-diff
exported-evaluator coverage passed with no coverage errors.

Hosted Windows process compilation at initial head `b96495a` failed with
four undefined `waitForAnyGitWaiter` references: that reused fixture helper is
excluded by `!windows`. The focused correction gives the save fixture its own
portable context-bound admission waiter. Verification is limited to the two
affected process tests and the mandatory full changed-code backend lint in the work order;
the earlier Linux checks do not prove corrected native Windows execution. The
affected race tests passed; the single full changed-code lint hit its six-minute
outer timeout (exit124, no diagnostics), so publication was checkpointed to
ROOT without an automatic retry. ROOT explicitly released one identical
recovery; it passed with zero issues and both originals are joined with their
groups gone. The original failed timeout and unproved cause remain recorded.

## Risks and delivery

The admission cap is process-global: run the anchored tests serially and restore
it only after draining all requests. Descriptor closure before Git/cleanup is
required for Windows compatibility; Linux execution does not prove Windows.
Queued cancellation does not prove rollback after a write. Temp-removal errors
remain best effort, matching existing behavior.

Follow [repository delivery guidance](../../../AGENTS.md#github-operations) and
the local [PR fixup skill](../../../.agents/skills/pr-fixup/SKILL.md). ROOT's
task-plan release/checkpoint contract remains authoritative for heavy scheduling,
normal hooks, exact-head review, and separately authorized serial merge. Design
handoff alone authorizes none of those actions.

## Current backend blocker correction

The same sequential order now includes ROOT's separately released retained-outcome
synchronization correction. The current-head API startup-evidence test failed
when stopping its first child. Its actual interleaving remains unproved, but
source exposes Stop waiting on an exit waiter while holding the mutex that the
waiter's terminal retention needs. A private recorder-wiring mutex removes only
that dependency; existing retained identity, copy and stamp behavior stays intact.
The deterministic RED failed both lock-held cases. GREEN passed all 11 affected
outcome tests and the exact failed API test, each with race and count10. Scoped
and the single mandatory full changed-code lint passed with zero issues; originals
are joined and gone. Corrected-head hosted gates remain pending. The original
hosted interleaving remains unproved, and no merge authorization was granted.
