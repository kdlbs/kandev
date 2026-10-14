---
id: "02-loopback-allocation-probe"
title: "Allocate task-process probe ports on loopback"
status: done
wave: 1
depends_on: []
plan: "./plan.md"
requirements:
  - REQ-EXECUTORS-PORT-COLLISION-SAFETY-001
acceptance_criteria:
  - AC-EXECUTORS-PORT-COLLISION-SAFETY-001.7
system_design:
  - ../../specs/executors/system-design/remote-executor-plugins.md
---

# Task 02: Allocate task-process probe ports on loopback

## Summary

`portutil.AllocatePort`, which fills `$PORT` placeholders in task process
commands, found a free port by binding `:0`. That wildcard bind makes every
newly built executable that runs it, including each temporary `go test`
binary, ask for Windows Firewall access. The probe only needs a port number,
so it binds `127.0.0.1:0`, the same loopback availability signal as
AC-EXECUTORS-PORT-COLLISION-SAFETY-001.7.

## In scope

- `AllocatePort` binds `127.0.0.1:0`.
- The allocator's regression test holds returned ports on loopback.

## Out of scope

Serving listeners (backend, agentctl, port tunnel) and the launcher probe in
Task 01.

## Acceptance

- `AllocatePort` returns an OS-assigned TCP port after closing its loopback probe.
- A caller can bind the returned loopback port, absent another process winning
  the probe-to-bind race. Once the caller holds that port, further allocations
  do not return it.
- Returned ports are not reserved, guaranteed unique across released probes,
  or guaranteed available on wildcard or non-loopback interfaces.
- No `portutil` test opens a wildcard listener.

## Verification

From the repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/common/portutil -count=1)
(cd apps/backend && go vet -trimpath ./internal/common/portutil)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- apps/backend/internal/common/portutil/portutil.go
- apps/backend/internal/common/portutil/portutil_test.go

## Dependencies

None.

## Risks

Low: a port free on loopback but held by a listener on one specific
non-loopback address can still be returned, as a port could already be taken
between the probe and the child bind.

## Parallelism

`sequential`

## Inputs

- REQ-EXECUTORS-PORT-COLLISION-SAFETY-001 and AC-EXECUTORS-PORT-COLLISION-SAFETY-001.7.

## Results

- `go test ./internal/common/portutil` passes, and the package opens only
  loopback listeners.
- Review follow-up: `TestAllocatePort` now binds the actual returned port, and
  `TestAllocatePortAvoidsHeldPorts` calls the allocator before holding each
  returned port. The former OS-only uniqueness test could not detect allocator
  regressions. Contained exact-source and mutation results are recorded in
  the PR's delivery evidence; full native platform verification runs in CI.
