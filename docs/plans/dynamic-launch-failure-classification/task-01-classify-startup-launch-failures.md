---
id: "01-classify-startup-launch-failures"
title: "Classify only agent startup launch failures as provider failures"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-LAUNCH-FAILURE-001
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-LAUNCH-FAILURE-001.1
  - AC-AGENTS-DYNAMIC-LAUNCH-FAILURE-001.2
  - AC-AGENTS-DYNAMIC-LAUNCH-FAILURE-001.3
  - AC-AGENTS-DYNAMIC-LAUNCH-FAILURE-001.4
  - AC-AGENTS-DYNAMIC-LAUNCH-FAILURE-001.5
system_design:
  - ../../specs/agents/system-design/dynamic-launch-failure-classification.md
---

# Task 01: Classify only agent startup launch failures as provider failures

## Summary

Move the launch error classification out of `dynamicTaskDownstream.Launch`
into `classifyDynamicLaunchFailure` and require agent startup provenance before
a launch error is classified.

## Acceptance

1. Workspace preparation, command validation, runtime detection, and local
   agentctl errors (including `connection refused`, `i/o timeout`, and
   `service temporarily unavailable` texts) return unclassified.
2. An `AgentStartupFailure` with a provider network error returns a classified
   `network_unavailable` failure.
3. An already classified error keeps its code.
4. A current asynchronous dynamic startup with an explicitly unsent prompt
   and a typed requested-model rejection applies the existing candidate policy.
   Fixed profiles, resume failures, stale attempts, and unknown delivery keep
   their existing recovery behavior.

## Asynchronous model rejection implementation

- `apps/backend/internal/orchestrator/dynamic_startup_model.go` accepts only the
  typed rejection and current no-output/no-effect startup evidence.
- `apps/backend/internal/orchestrator/event_handlers_agent.go` associates that
  evidence with the current execution, generation, session and profile.
- `apps/backend/internal/orchestrator/dynamic_launch.go` preserves the proven
  pre-prompt evidence for the existing dynamic candidate policy.

## Verification

- `go test ./internal/orchestrator/ -run 'TestDynamicLaunch'`
- `make -C apps/backend build`
- On this change, three temporary startup-model boundary and ownership checks
  passed with a Go overlay, as did the existing startup admission and dynamic
  successor launch checks. Specification lint, documentation catalog validation
  and normal commit hooks passed. No live-provider or full-backend run is claimed.
