---
created: 2026-10-20
status: done
requirements:
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002
system_design:
  - ../../specs/platform/system-design/agent-runtime-availability.md
legacy_specs: []
---

# Implementation Plan: Windows owned process tree reclamation

## Overview

On Windows `processidentity.OwnedSessionTerminated` always returned
`ErrUnverifiableIdentity` and `TerminateOwnedSession` always refused, because
the launcher's Job Object was anonymous and could not be found again once its
creator was gone. Every caller therefore failed closed on Windows: retired
agentctl leases were never detached, retained journals and delivery evidence
could not prove the original runtime dead, and an exited adopted agentctl
could not be contained and replaced.

## Scope

In scope: `apps/backend/internal/common/processidentity` (shared identity and
the Windows implementation), `internal/common/winproc` (named kill-on-close
jobs), the agentctl launcher (`launcher.go`, `lifecycle_windows.go`), agentctl
startup (`cmd/agentctl/main.go`), and the configuration catalog exclusion.

Excluded: Linux and macOS containment, callers of the identity API, and any
change to when recovery or cleanup is attempted.

## Technical approach

Name the launcher's kill-on-close job, carry the name in `Identity`, and on
Windows prove termination from the job's active process count after the
original owner is confirmed exited.

## Work orders

- [x] [Task 01: Prove and contain Windows owned process trees](task-01-windows-owned-job.md)

## Changed-path coverage

The work order owns the processidentity, winproc, launcher, agentctl startup,
and configuration catalog files above, `docs/public/configuration.md`,
their permanent tests, `AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002.7`, and the
Windows containment paragraph of the agent runtime availability design.
