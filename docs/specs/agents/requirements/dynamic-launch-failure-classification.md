---
status: active
system: agents
created: 2026-10-08
owners:
  - kandev
---

# Dynamic Launch Failure Classification Requirements

## Overview

When a dynamic route launches a candidate, the launch can fail before the
agent process runs: workspace preparation, executor setup, command
validation, runtime detection, or the local agentctl control plane. These are
Kandev conditions. They say nothing about the candidate's provider. If such an
error is classified as a provider failure, dynamic routing opens the
candidate's circuit and moves to the next candidate although the provider was
never contacted, and the real problem is hidden behind a provider suspension.

## Requirements

### REQ-AGENTS-DYNAMIC-LAUNCH-FAILURE-001: Only agent startup failures are provider failures

**Intent:** A synchronous dynamic launch error counts as a provider failure only
when it carries agent startup provenance.

**User story:** As an operator, I want a Kandev-side launch failure handled by
the ordinary launch recovery, so that a healthy provider candidate is not
suspended and the failure explains itself.

#### Acceptance criteria

- **AC-AGENTS-DYNAMIC-LAUNCH-FAILURE-001.1:** A dynamic launch error that does
  not carry `routingerr.AgentStartupFailure` provenance and is not already a
  classified `routingerr.Error` shall reach the conductor unclassified, even
  when its text matches a provider or provider-neutral rule (for example a
  local `connection refused`, `i/o timeout`, or `service temporarily
  unavailable`).
- **AC-AGENTS-DYNAMIC-LAUNCH-FAILURE-001.2:** A launch error that carries agent
  startup provenance shall keep its classification, so a provider failure
  reported while the agent process or ACP session starts still routes.
- **AC-AGENTS-DYNAMIC-LAUNCH-FAILURE-001.3:** A launch error that is already a
  classified `routingerr.Error` shall be returned unchanged.
- **AC-AGENTS-DYNAMIC-LAUNCH-FAILURE-001.4:** A low-confidence startup
  classification shall stay unclassified, as before.

## Out of scope

- Classification rules themselves.
- Asynchronous startup failures reported through `agent.failed`, which already
  require startup provenance.
- Ceiling admission and route persistence errors, which return before the
  launch and are already unclassified.
