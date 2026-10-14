---
status: current
system: agents
requirements:
  - REQ-AGENTS-DYNAMIC-LAUNCH-FAILURE-001
---

# Dynamic Launch Failure Classification System Design

## Purpose and boundaries

This design covers how `dynamicTaskDownstream.Launch` turns a synchronous
`LaunchPreparedSession` error into the error the dynamic conductor routes on.
It does not change the classifier rules, the asynchronous startup failure path,
or ceiling admission.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-AGENTS-DYNAMIC-LAUNCH-FAILURE-001` | [Launch error classification](#launch-error-classification) |

## Components and responsibilities

- **`internal/agent/runtime/lifecycle`** wraps agent process start and ACP
  session initialization errors with `routingerr.NewAgentStartupFailure`.
- **`internal/orchestrator`** (`classifyDynamicLaunchFailure`) decides whether a
  launch error is routed as a provider failure.
- **`internal/agent/runtime/dynamic`** applies the candidate's policy to a
  classified failure and opens resource circuits.

## Launch error classification

`classifyDynamicLaunchFailure(err, executionProfileID)`:

1. An error that already wraps a `routingerr.Error` is returned unchanged.
2. An error without `routingerr.AgentStartupFailure` in its chain is returned
   unchanged and unclassified. Workspace preparation, executor setup, command
   validation, runtime detection, and local agentctl errors take this branch,
   so the ordinary launch recovery owns them and no candidate circuit opens.
3. An agent startup failure is classified with `PhaseProcessStart` for the
   execution profile. A low-confidence result stays unclassified; any other
   result wraps the original error with the classification.

Before this rule, every launch error text was classified, and the
provider-neutral network and availability rules matched local control-plane
errors such as `dial tcp 127.0.0.1:<port>: connect: connection refused`. That
suspended the candidate and advanced the route without contacting the
provider. The asynchronous path (`handleDynamicAgentStartupFailure`) already
required startup provenance; both paths now share that boundary.
