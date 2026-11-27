---
created: 2026-10-08
status: implemented
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004
system_design:
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
legacy_specs: []
---

# Fix plan: Resume unpushed task branches

## Problem and scope

After archive removes a task worktree, a successful managed repository refresh
sets `RemoteSyncHandled`. Recreation then requires `origin/<task-branch>` even
when the saved local task branch survives with unpushed commits. A real-Git
archive/unarchive regression fails with `required fetched remote ref ... is missing`.

Restore conformance with `AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.5`:
recoverable workspace and branch data must remain usable after unarchive.
The user requested a PR after the temporary reproduction established the defect.
One sequential [work order](task-01-preserve-local-branch.md) implements and validates
the correction.

## Approach and boundaries

For ordinary recreation with a verified saved local branch, accept that branch
when its origin tracking ref is absent. Preserve selection of refreshed history
when the remote ref exists. Explicit checkout branches and PR snapshots retain
their remote-ref requirements. Missing local branches keep the existing recovery
and explicit replacement policies.

No new UI, API, persistence, provider-conversation identity, or Git ref mutation
is introduced. The repair affects only host worktree recreation. Existing
recovery controls and public documentation remain applicable.

## Verification

Use the existing real-Git archive/unarchive test with a successful refresh,
negative tests for explicit checkout/PR selection, and the worktree package
suite with the race detector. Normal commit hooks validate changed Go packages
and documentation. Exact commands and results belong to the work order.

## Delivery result

The single work order is done. The real archive/unarchive regression now restores
the unpublished task branch after a successful fetch. Explicit remote selection
remains strict. The focused regressions and full worktree race suite passed;
documentation catalog and specification checks passed.
