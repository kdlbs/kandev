---
created: 2026-10-08
status: in_progress
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-02.md
legacy_specs: []
---

# Implementation Plan: Keep a superseded execution's failure off the current candidate

## Overview

After a failure, the orchestrator records the successor decision on the
session (`ExecutionProfileID` and `RouteGeneration`) before it relaunches. The
relaunch can be deferred by ceiling admission before the predecessor is
stopped, so the predecessor execution keeps serving the session while the
session already names the successor. A later failure of that predecessor
execution passed the failure-session check, which compared only the execution
ID, and was routed with the session's execution profile. The engine then
opened the successor's circuit and advanced the route, although the successor
never ran.

One work order fences failures to the generation and candidate that produced
them, in the orchestrator and in the engine. See
[task 01](task-01-fence-superseded-candidate-failure.md).

## Scope

### In scope

- `dynamicFailureSession` in `apps/backend/internal/orchestrator`.
- `Engine.ApplyFailureContext` in `apps/backend/internal/agent/runtime/dynamic`.
- Unit tests for the engine fence and for the orchestrator failure path.
- Concrete execution-profile attribution on lifecycle stream events and its
  propagation through `handleAgentErrorEvent`.
- Regression tests for stream-event publication, predecessor errors, current
  candidate errors, and legacy events without a concrete profile.

### Out of scope

- Recording the superseded execution's failure against its own profile.
- Launch-error classification and routing error rules.

## Verification

- `go test -tags fts5 ./internal/agent/runtime/dynamic/`.
- `go test -tags fts5 ./internal/orchestrator/`.
- `golangci-lint run ./internal/agent/runtime/dynamic/... ./internal/orchestrator/...`.
- `make -C apps/backend build`.

## Review follow-up

The stream-error publisher now captures the concrete profile from the execution.
The error handler passes that profile to the existing failure guard.
The logical profile and Office identity retain their existing meaning.
Legacy events without a concrete profile retain their existing behavior.

The added regression tests cover publication and error-handler attribution.
The review did not run tests, builds, linters, or commit checks, as requested.
Verification of the review follow-up remains pending.
