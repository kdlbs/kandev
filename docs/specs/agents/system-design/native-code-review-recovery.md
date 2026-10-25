---
status: current
system: agents
requirements:
  - REQ-AGENTS-NATIVE-CODE-REVIEW-001
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


## Permanent verification

The runner's recovery tests exercise the real review service and SQLite
repository for native settlement, history/finding preservation, multiple active
rows, external ownership and workflow-entry redelivery. Store-boundary fault
injection proves read/settlement failures stop inference and release the claim.
A real repository write barrier covers both cancellation/settlement orders;
live inference coverage proves rejoin and the separate lock scopes. See the
[work order](../../../plans/native-code-review/task-14-abandoned-native-admission.md)
for commands and the hosted acceptance requirements.
