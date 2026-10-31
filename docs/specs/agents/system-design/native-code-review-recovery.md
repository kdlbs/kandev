---
status: current
system: agents
requirements:
  - REQ-AGENTS-NATIVE-CODE-REVIEW-001
  - REQ-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-001
---

# Abandoned native review admission

## Ownership and recovery

`internal/review.Runner` owns native manual and workflow-step review workers.
Its task-scoped in-memory claim prevents concurrent workers. Admission first
acquires that claim; a refused claim rejoins the existing run without changing it.

After acquiring a new claim, admission reads `ReviewService.ActiveRun`. A retained
native manual or workflow-step run has no worker in this runner. This can follow
a failed terminal-state write even though the ordinary startup cancellation sweep
and detached failure-write context remain in place.

Admission settles that native run through the existing terminal-guarded
`FailRun` service, preserving its row, findings, recorded duration and history.
A confirmed terminal result is followed by another active lookup because the
newest row can hide older retained runs. Only exhausting native runs without an
older agent/legacy owner permits fresh admission. An inconsistent terminal active
row, read error or settlement error returns to the caller and releases the new
claim without dispatching work.
The runner serializes this settlement with its explicit cancellation write,
separately from the worker-claim mutex. This prevents a cancellation that already
returned from being overwritten by the service's later failure write. Inference
never holds this settlement mutex.

Agent-published runs and legacy runs without a classified native trigger are
not owned by this runner and remain untouched. The existing entry-ID lookup
still rejoins the original run after settlement, so workflow-entry redelivery
does not replay a completed or failed entry. An explicit new request without
that entry identity can start fresh work.

## Requirement mapping

AC-AGENTS-NATIVE-CODE-REVIEW-001.14 is satisfied by the task claim, classified
native settlement, terminal readback and retained entry-ID deduplication. No
schema, timer, provider retry policy or public command is added.

AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-001.1 through .4 are satisfied by the
task-bound local source fallback. `ReviewChangeSource` first uses the existing
execution lookup, which checks `session.exec` before it reads agentctl data. If
that read fails, the source checks `session.exec` again before it calls the
local reader. A missing access checker or a denied check stops the fallback
before the local reader opens the persisted checkout. An agentctl cumulative
diff response with an error state also enters the fallback. A remote executor
still requires agentctl.

The backend reader (`internal/backendapp/review_local_source.go`) admits only
`local`, `worktree`, and `local_pc` executors. It uses the persisted session
worktree paths and accepts no caller-supplied filesystem path. The local Git
collector rejects a path that is not absolute and local or that carries a `..`
segment. Its per-file, output, and total diff limits match agentctl. The
per-file limit checks the working file, base blob, and index blob before a
staged-only diff is read.

Working-tree, index-only, untracked, and committed changes remain separate
sources. Both agentctl and the local collector use the bounded formatter in
`internal/common/unidiff` for untracked files. This keeps normalized patch text
and hashes consistent across the two sources.

For committed changes, the reader finds the `TaskRepository` row for the
session task and repository. If that attachment has a validated
`comparison_target`, the reader resolves only its `ComparisonRef`. A missing or
unmergeable ref returns `review_workspace_unavailable`; it does not fall back
to a same-named origin branch or a stored base. Without an explicit target, the
reader keeps the live target branch (`origin/<branch>` first, then the local
branch), with the stored session base as its fallback. This excludes changes
already merged from an ordinary target. A single repository keeps the legacy
bare-path scope key. A multi-repository task uses its workspace-relative
repository path, matching the Review panel.

`CollectChanges` propagates a committed-source `review_workspace_unavailable`
error. It does not start a partial review when an explicit comparison target
cannot be resolved. Other committed-source errors remain non-fatal so valid
working-tree changes can still be reviewed when no committed range exists.


## Permanent verification

The runner's recovery tests exercise the real review service and SQLite
repository for native settlement, history/finding preservation, multiple active
rows, external ownership and workflow-entry redelivery. Store-boundary fault
injection proves read/settlement failures stop inference and release the claim.
A real repository write barrier covers both cancellation/settlement orders;
live inference coverage proves rejoin and the separate lock scopes. See the
[work order](../../../plans/native-code-review/task-14-abandoned-native-admission.md)
for commands and the hosted acceptance requirements.
