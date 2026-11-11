---
id: "01-resolve-probe-npm-prefix"
title: "Resolve the npm prefix in the probe"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-PROVIDER-PROBE-001
acceptance_criteria:
  - AC-OFFICE-PROVIDER-PROBE-001.1
  - AC-OFFICE-PROVIDER-PROBE-001.2
  - AC-OFFICE-PROVIDER-PROBE-001.3
  - AC-OFFICE-PROVIDER-PROBE-001.4
  - AC-OFFICE-PROVIDER-PROBE-001.5
system_design:
  - ../../specs/office/system-design/provider-health-probe.md
---

# Task 01: Resolve the npm prefix in the probe

## Summary

`ACPProbe.Probe` prepares the managed npm prefix on a copy of the resolved argv
before spawning, and reports a classified failure if preparation fails.

## In scope

- `routingerr/acp_probe.go`.
- Tests for the marker, the unchanged argv, and the unmutated resolver slice.

## Out of scope

Transient-error escalation to `user_action_required` and retries for cancelled or
archived tasks.

## Acceptance

`AC-OFFICE-PROVIDER-PROBE-001.1` through `001.5`.

## Verification

```bash
cd apps/backend
go test -trimpath ./internal/agent/runtime/routingerr -count=1
```

## Results

- RED: with the marker, the fake binary received `~/.kandev/managed-npm-runtime`.
- GREEN: the binary receives the resolved temp root (mode 0700) and the package
  tests pass.
- A real `npx` run with an empty `HOME` failed on the old probe and passed on the
  fixed one.
- A relative `TMPDIR` makes preparation fail; the probe returns a classified error
  and spawns nothing (`AC-OFFICE-PROVIDER-PROBE-001.4`).
