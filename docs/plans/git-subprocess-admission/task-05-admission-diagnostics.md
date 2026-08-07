---
id: "05-admission-diagnostics"
title: "Admission diagnostics"
status: done
wave: 2
plan: "plan.md"
spec: "../../specs/platform/git-subprocess-admission.md"
depends_on:
  - "01-class-aware-admission"
---

# Task 05: Admission Diagnostics

## Acceptance

- The agentctl snapshot is inaccessible without its existing bearer credential.
- The authenticated agentctl endpoint reports the process-local admission
  snapshot without exposing or changing the bearer credential.
- Metrics distinguish inflight work, waiters, acquisitions, and wait time per
  class while preserving aggregate Git keys and adding no mutable settings or UI.

## Verification

```bash
# Direct Go commands are required because the backend Makefile has no focused or
# race-test targets.
cd apps/backend && go test ./internal/agentctl/server/api ./internal/agent/runtime/agentctl -run 'Test.*SubprocessAdmission' -count=1
cd apps/backend && go test -race ./internal/agentctl/server/api ./internal/agent/runtime/agentctl -run 'Test.*SubprocessAdmission' -count=1
```

## Files Likely Touched

- `apps/backend/internal/agentctl/server/api/control_server.go`
- Focused agentctl control-server tests
- `apps/backend/internal/agent/runtime/agentctl/control.go`
- Focused `ControlClient` tests

## Dependencies

Task 01 (`01-class-aware-admission`).

## Parallelism

Parallel-safe with Tasks 02 and 03 after Task 01. This task owns control-server,
control-client, debug, and backend wiring files, not Git call sites.

## Inputs

- Spec sections: diagnostics, API and permissions, persistence guarantees.
- Plan section: Authenticated diagnostics.
- Existing agentctl bearer middleware and `ControlClient` auth transport.

## Output Contract

Report red authorization tests, response shape, files changed, exact command
outcomes, race-test evidence, security boundary, residual risks, and synchronized
task/plan status.

## Results

- RED: the authenticated control-server test returned 404 for the new
  admission route and the client test failed before the route/client contract
  existed.
- GREEN: added the bearer-protected agentctl admission snapshot route, the
  authenticated `ControlClient` probe, and standalone/lifecycle provider wiring.
- `cd apps/backend && go test ./internal/agentctl/server/api ./internal/agent/runtime/agentctl -run 'Test.*SubprocessAdmission' -count=1` — passed.
- `cd apps/backend && go test -race ./internal/agentctl/server/api ./internal/agent/runtime/agentctl -run 'Test.*SubprocessAdmission' -count=1` — passed.
- The endpoint uses the existing control-server bearer middleware and returns
  only the owning agentctl process's in-memory snapshot.
