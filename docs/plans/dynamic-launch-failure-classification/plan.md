---
created: 2026-10-08
status: implemented
requirements:
  - REQ-AGENTS-DYNAMIC-LAUNCH-FAILURE-001
system_design:
  - ../../specs/agents/system-design/dynamic-launch-failure-classification.md
legacy_specs: []
---

# Implementation Plan: Classify only agent startup launch failures

## Overview

`dynamicTaskDownstream.Launch` classified every `LaunchPreparedSession` error
text against the provider rules. Kandev-side errors whose text matched a
provider-neutral rule, such as a refused or timed-out connection to the local
agentctl, were routed as provider failures: the candidate's circuit opened and
the route moved on although the provider was never contacted.

One work order limits classification to errors that carry agent startup
provenance. See [task 01](task-01-classify-startup-launch-failures.md).

## Scope

### In scope

- `classifyDynamicLaunchFailure` in `apps/backend/internal/orchestrator`.
- The asynchronous initial-start path for a current typed, pre-prompt
  requested-model rejection and its existing candidate policy.
- Tests through `dynamicTaskDownstream.Launch` with a mocked agent manager.

### Out of scope

- Classifier rules.
- Other asynchronous `agent.failed` startup outcomes.
- Ceiling admission and route persistence.

## Verification

- `go test ./internal/orchestrator/ -run 'TestDynamicLaunch'`
- `go test ./internal/orchestrator/` compared with the base revision.
- `golangci-lint run ./internal/orchestrator/...`
- `make -C apps/backend build`

The typed-rejection follow-up has permanent regressions in
`dynamic_startup_model_test.go` and `dynamic_startup_evidence_test.go`.
They cover typed rejection, prompt delivery evidence, resume failures, route
ownership, candidate policy, recorded effects, and execution admission.
The work order records focused verification. No live-provider or full-backend
run is claimed.
