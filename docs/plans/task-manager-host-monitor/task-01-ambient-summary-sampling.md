---
id: "01-ambient-summary-sampling"
title: "Add ambient summary sampling"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-TASK-MANAGER-MONITOR-001
  - REQ-PLUGINS-TASK-MANAGER-MONITOR-003
acceptance_criteria:
  - AC-PLUGINS-TASK-MANAGER-MONITOR-001.1
  - AC-PLUGINS-TASK-MANAGER-MONITOR-001.2
  - AC-PLUGINS-TASK-MANAGER-MONITOR-001.3
  - AC-PLUGINS-TASK-MANAGER-MONITOR-001.4
  - AC-PLUGINS-TASK-MANAGER-MONITOR-003.1
  - AC-PLUGINS-TASK-MANAGER-MONITOR-003.2
  - AC-PLUGINS-TASK-MANAGER-MONITOR-003.3
  - AC-PLUGINS-TASK-MANAGER-MONITOR-003.4
  - AC-PLUGINS-TASK-MANAGER-MONITOR-003.5
system_design:
  - ../../specs/plugins/system-design/task-manager-host-monitor.md
---

# Task 01: Add Ambient Summary Sampling

## Summary

Add the backend contract that the ambient monitor needs: normalized operator
configuration, platform host readings, attributed CPU-only summaries, and a
bounded partial-success webhook. Preserve the existing detailed usage response.

## In scope

- Manifest capability, summary webhook, refresh choices, and disk path.
- Config loading and normalization through the injected Host.
- Host CPU, memory, disk, temperature, and load collectors across declared
  platforms.
- Task CPU per-core and relative calculations without PSS or task-title reads.
- Request validation, cancellation, serialization, partial errors, and tests.

## Out of scope

- Personal preference storage or UI.
- Changing the detailed Task Manager dialog.
- Copying Kandev's AGPL collector source into the MIT plugin.

## Acceptance

- A valid summary request returns only requested resource families, correct raw
  values, effective interval/core metadata, and independent availability.
- Relative CPU is clamped to 0%-100%; task per-core CPU retains values above
  100%; the task summary never calls the expensive memory/title paths.
- Invalid selectors/config fail safely, while a disk or platform collector
  error leaves other metrics available and all existing usage tests green.

## Verification

```bash
rtk go test ./server/...
rtk go vet ./server/...
rtk make package
```

Run from `kdlbs-kandev-plugin-task-manager`.

## Files likely touched

- `kdlbs/kandev-plugin-task-manager/manifest.yaml`
- `kdlbs/kandev-plugin-task-manager/server/plugin.go`
- `kdlbs/kandev-plugin-task-manager/server/plugin_test.go`
- `kdlbs/kandev-plugin-task-manager/server/sampler.go`
- `kdlbs/kandev-plugin-task-manager/server/sampler_test.go`
- New `kdlbs/kandev-plugin-task-manager/server/hostmetrics*.go` and focused
  platform/common tests.

## Dependencies

None.

## Risks

- Cross-platform unit tests run only on their owning operating system; the
  package build must still compile every manifest target.
- Back-to-back dialog and summary requests share cumulative task CPU baselines
  and must remain serialized without manufacturing short-interval spikes.

## Parallelism

`sequential`

## Inputs

- Requirement sections `REQ-PLUGINS-TASK-MANAGER-MONITOR-001` and `003`.
- System-design sections **Existing baseline**, **Operator configuration**,
  **Summary contract**, **Metric semantics**, and **Collection flow**.
- Existing `server/sampler_test.go` fake clock/scanner patterns.

## Results

Completed on 2026-09-05. The plugin now exposes the authenticated `summary`
webhook, validates and caches install-wide configuration, samples host CPU,
memory, disk, temperature, and load independently, and keeps task CPU
sampling free of PSS/RSS and title enrichment. Platform readers compile for
Linux, macOS, Windows, and unsupported targets.

Verification: `go test ./server/...`, `go vet ./server/...`, five-platform
`make package`, and Darwin/Windows/FreeBSD `go test -c` all passed.
