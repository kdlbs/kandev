---
spec: docs/specs/executors/requirements/remote-task-directory-reclamation.md
system_design: docs/specs/executors/system-design/remote-task-directory-reclamation.md
created: 2026-08-24
status: done
---

# Implementation Plan: SSH Remote Task-Directory Reclamation

## Overview

Build the v2 housekeeper bottom-up, so that the safety machinery exists and is
proven before anything can call it. Wave 1 delivers the two independent
primitives — the read-only safety probe plus guarded removal in the `lifecycle`
package, and the per-profile opt-in plumbing in `orchestrator/executor` — neither
of which can delete anything on its own. Wave 2 wires them into the durable
cleanup job, which is the only caller and the point at which removal becomes
reachable. Wave 3 adds the settings UI, the ADR, and the documentation
corrections.

The order is deliberate: task 01 is written and tested with no caller, so a
mistake in the probe cannot reach a real host through a half-built job phase.

## Safety constraint on all tasks

**No test may delete anything outside a `t.TempDir()`.** The removal path is
exercised through the `sshRemoteRunner` interface with recorded command outputs,
or against a temporary directory. No test creates, targets, or removes a path
under a real Kandev workspace root, and no test runs `rm -rf` against a path
derived from the developer's environment. A test that deletes its own workspace
destroys the run that produced it.

---

## Waves

| Wave | Tasks | Parallel-safe |
| --- | --- | --- |
| 1 | 01, 02 | Yes — disjoint packages |
| 2 | 03 | No — depends on 01 and 02 |
| 3 | 04, 05 | Yes — disjoint (web / docs) |

---

## Backend

### Task 01 — Probe and guarded removal (`lifecycle`)

New `executor_ssh_reclaim.go` with `SSHTaskDirReclaimer`, the four-probe safety
table, the path-shape guard, and `rm -rf` behind a `sshRemoteRunner` seam. Pure
verdict logic, no callers.

### Task 02 — Per-profile opt-in (`orchestrator/executor`, `lifecycle`)

`MetadataKeySSHReclaimTaskDir`, entry in `profileConfigAuthoritativeKeys` and
`persistentMetadataKeys`. Proves a task-supplied value cannot enable the feature.

### Task 03 — Cleanup-job phase (`task/service`)

Snapshot capture, ownership resolution, invocation, and the skip-versus-error
outcome folding. This is where the regression coverage for removal-on-terminal
and preservation-on-ordinary-stop lands.

---

## Frontend

### Task 04 — Settings toggle

Per-profile switch in the SSH executor profile settings with blast-radius copy,
routed through `t()` and translated into all five locales.

---

## Documentation and decisions

### Task 05 — ADR and doc corrections

ADR for the opt-in default and the cleanup-job placement;
`docs/public/feature-status.md` SSH row updated to describe shipped behavior.

`apps/backend/AGENTS.md` and the `Out of scope (v1)` item in
`docs/specs/executors/system-design/ssh-executor.md` is corrected as part of this design package
rather than at ship time — the first is a present-tense factual error about
today's code, and the second is the specification boundary this package moves.

---

## Task checklist

- [x] 01 — `task-01-reclaim-probe-and-removal.md`
- [x] 02 — `task-02-profile-opt-in.md`
- [x] 03 — `task-03-cleanup-job-phase.md`
- [x] 04 — `task-04-settings-toggle.md`
- [x] 05 — `task-05-adr-and-docs.md`

## Risks

- **The probe is the whole feature.** A false "safe" verdict is silent,
  irreversible data loss on a machine Kandev does not own. Task 01's verdict
  table must be exhaustive over probe-failure modes, and every unknown must
  resolve to skip.
- **Ownership signal coverage.** The environment-ownership check is the existing
  precedent, but a parent task whose `executor_running` rows were pruned while
  its directory survives would not be seen by the row-level check. The
  environment check is the primary defence and the row check is secondary; task
  03 must cover the `inherit_parent` case directly.
- **Archive is reversible, removal is not.** An unarchived task re-materializes
  its checkout. That is acceptable under an opt-in setting but must be stated in
  the settings copy (task 04).
