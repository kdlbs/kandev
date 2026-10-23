---
created: 2026-10-20
status: done
requirements:
  - REQ-AGENTS-SESSION-CEILING-003
system_design:
  - ../../specs/agents/system-design/session-memory-guard.md
legacy_specs: []
---

# Implementation Plan: Session memory guard

## Overview

The session ceiling bounds the number of automatic sessions, but nothing stops
a new agent process from starting on a host that is already short of memory,
and manual starts pass the ceiling entirely. Each agent tree costs hundreds of
megabytes to over a gigabyte, so a burst of launches can push the host into
swap or a watchdog restart that takes every session down. Behind
`features.sessionMemoryGuard`, off in every shipped profile, the ceiling now
also holds automatic launches that would add an agent process while the host is
under memory pressure. Manual launches keep their existing admission so no user
input waits on the host condition.

## Scope

In scope: the new `apps/backend/internal/system/hostmemory` package, the
orchestrator ceiling controller (`session_ceiling.go`,
`session_ceiling_memory.go`, `ceiling_surface.go`, `service.go`), the runtime
flag (`FeaturesConfig`, `runtimeflags/registry.go`, `profiles.yaml`,
`backendapp/orchestrator.go`, frontend `defaultFeatureFlags`), and
`docs/public/configuration.md`.

Excluded: lanes or per-profile capacity, CPU pressure, stopping running
sessions, and any change to the session-count ceiling itself.

## Technical approach

Read host memory inside the existing admission critical section and refuse with
a distinct `memory_pressure` reason, reusing the ceiling's deferral, card note,
and retry sweep.

## Work orders

- [x] [Task 01: Hold launches under host memory pressure](task-01-session-memory-guard.md)

## Changed-path coverage

The work order owns the hostmemory package, the orchestrator ceiling files
above, their permanent tests, the flag plumbing,
`REQ-AGENTS-SESSION-CEILING-003`, and the session memory guard design.
