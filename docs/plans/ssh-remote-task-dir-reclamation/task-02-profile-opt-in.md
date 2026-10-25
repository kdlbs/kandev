---
id: "02-profile-opt-in"
title: "Per-profile reclamation opt-in"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements: "../../specs/executors/requirements/remote-task-directory-reclamation.md"
acceptance_criteria:
  - AC-SSH-TASKDIR-RECLAMATION-006.1
  - AC-SSH-TASKDIR-RECLAMATION-006.2
  - AC-SSH-TASKDIR-RECLAMATION-006.3
system_design: "../../specs/executors/system-design/remote-task-directory-reclamation.md"
---

# Task 02: Per-profile reclamation opt-in

## Summary

Add the `ssh_reclaim_task_dir` executor-profile setting and plumb it into launch
metadata as an **authoritative** key, so the profile is the only thing that can
enable reclamation and the default everywhere is disabled.

## Scope

- `MetadataKeySSHReclaimTaskDir = "ssh_reclaim_task_dir"` in
  `executor_backend.go`.
- Entry in `persistentMetadataKeys` so the value reaches
  `ExecutorRunning.Metadata` and survives a backend restart.
- Entry in `profileConfigAuthoritativeKeys` in
  `orchestrator/executor/executor_state.go` — **not** `profileConfigPassthroughKeys`.
- A reader helper treating only the exact string `"true"` as enabled.

## Exclusions

- No settings UI (task 04).
- No consumption of the flag (task 03).

## Implementation acceptance conditions

1. A profile with no `ssh_reclaim_task_dir` key, an empty value, or any value
   other than `"true"` resolves to disabled.
2. A task supplying `ssh_reclaim_task_dir: "true"` in its own metadata while the
   profile has no value resolves to disabled — the authoritative projection
   overwrites it. This has a dedicated regression test.
3. The value round-trips through `ExecutorRunning.Metadata` persistence and is
   readable after a simulated restart.

## Verification

```bash
make -C apps/backend test ARGS='-run "TestApplyProfileConfigToMetadata|TestSSHReclaim" ./internal/orchestrator/executor/... ./internal/agent/runtime/lifecycle/...'
make -C apps/backend lint
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go`
- `apps/backend/internal/orchestrator/executor/executor_state.go`
- Test files alongside both.

## Dependencies

None.

## Inputs

- System design section `Per-profile setting`.
- The existing `ssh_workdir_root` / `ssh_shell` authoritative-key comment in
  `executor_state.go`, which states the redirect-vector rationale this key shares.

## Risks

Placing the key in the passthrough list instead of the authoritative list would
let task metadata enable a destructive operation. The regression test in
condition 2 is the guard.

## Safety

No deletion path is reachable from this task.

## Output contract

Set `status` to `done`, tick the box in `plan.md`, and report the key name,
lists it was added to, and tests run.
