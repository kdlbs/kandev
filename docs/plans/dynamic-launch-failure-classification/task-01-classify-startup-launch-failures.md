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

## Verification

- `go test ./internal/orchestrator/ -run 'TestDynamicLaunch'`
- `make -C apps/backend build`
