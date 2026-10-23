---
id: "01-session-memory-guard"
title: "Hold launches under host memory pressure"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-CEILING-003
acceptance_criteria:
  - AC-AGENTS-SESSION-CEILING-003.1
  - AC-AGENTS-SESSION-CEILING-003.2
  - AC-AGENTS-SESSION-CEILING-003.3
  - AC-AGENTS-SESSION-CEILING-003.4
  - AC-AGENTS-SESSION-CEILING-003.5
system_design:
  - ../../specs/agents/system-design/session-memory-guard.md
---

# Task 01: Hold launches under host memory pressure

## Summary

With `features.sessionMemoryGuard` on, defer a launch that would add an agent
process while host available memory is below the floor or commit charge is
above the ceiling.

## In scope

- `internal/system/hostmemory`: `Reading`, `Gate.Pressured`, `ReadConfig`,
  `readingFromProc` (commit judged only under strict Linux overcommit), and
  Windows, Linux, and unsupported readers.
- `sessionCeilingController.configureMemoryGate`, `memoryPressuredLocked`
  (automatic origin only), `memoryReadingLocked` (one-second sample), and the
  `memory_pressure` refusal in `admit`, `handOffOrAdmit`, and
  `decideLocked`; the coordinator exemption; the card note text.
- Runtime flag `features.sessionMemoryGuard` /
  `KANDEV_FEATURES_SESSION_MEMORY_GUARD`, off in prod, dev, and e2e.

## Out of scope

Lanes, CPU pressure, stopping running sessions, and the count ceiling itself.

## Verification

- `go test ./internal/system/hostmemory/ -count=1`
- `go test ./internal/orchestrator/ -run 'TestSessionCeilingMemory|TestCoordinatorTaskMemoryExemption' -count=1`
- `go test ./internal/runtimeflags/ ./internal/profiles/ -count=1`
- `GOOS=linux` and `GOOS=darwin` vet of `internal/system/hostmemory`


## Windows CI package coverage follow-up

Existing host-memory cases, including the Windows native probe, are included in the existing Windows native-suite command. No new standing case or workflow job is introduced.
The native/process suite split, original package targets and flags, permissions,
runner selection, timeouts and required gate are preserved.
The process-cohort runner targets only agentctl/server/process and therefore
does not supply these separate package cases. Three existing Windows workflow-contract
checks, changed-command selection checks, documentation coverage and whitespace
validation passed. Runtime Go sources/cases are unchanged and were not rerun
in this configuration-only follow-up. Hosted native execution still requires
the upstream workflow's normal approval; it is not claimed as locally executed.
