---
status: active
system: agents
created: 2026-10-12
owners:
  - kandev
---

# Agent Install Detection Bounds Requirements

## Overview

Kandev answers "is this agent installed?" by running each agent's `IsInstalled`
probe. Several probes execute a real capability check, and discovery runs every
enabled agent's probe concurrently under one overall budget. A check that is
bounded too tightly therefore reports a healthy agent as missing whenever
unrelated checks compete for the same machine. The agent system owns this
contract because availability is part of the agent capability surface that
task, Office, and settings consumers read.

## Terminology

- **Capability check:** A non-interactive command an agent's probe runs to
  confirm the installed binary can serve Kandev, such as `hermes acp --check`.
- **Bound:** The time limit for one run of a capability check.
- **Discovery budget:** The overall time limit for one concurrent detection
  sweep across every enabled agent.
- **Conclusive failure:** A missing binary, or a capability check that answered
  with a nonzero exit.

## Requirements

### REQ-AGENTS-INSTALL-DETECTION-001: Honest install detection under concurrent load

**Intent:** An installed agent is reported available unless its own check
actually fails. Contention from a concurrent detection sweep must not turn a
healthy installation into a "not installed" result, and a conclusive failure
must not be re-run, because the extra attempt would only add latency for an
install that is already known to be unusable.

**User story:** As a user with Hermes installed, I want Kandev to list Hermes as
available even when every agent is being detected at once, so that I can select
it without restarting or refreshing detection until the machine is idle.

#### Acceptance criteria

- **AC-AGENTS-INSTALL-DETECTION-001.1:** When a capability check fails because
  its bound elapsed before the command answered, the system shall distinguish
  that outcome from a check whose command answered with a nonzero exit.
- **AC-AGENTS-INSTALL-DETECTION-001.2:** When detection is cancelled by its
  caller, the system shall report a discovery error; a missing binary, a nonzero
  exit, and an elapsed bound shall report the agent as unavailable without an
  error.
- **AC-AGENTS-INSTALL-DETECTION-001.3:** When the first bound of an agent whose
  check is measurably slower than the shared bound elapses, the system shall
  measure that check once more at a larger bound and treat the second
  measurement as final.
- **AC-AGENTS-INSTALL-DETECTION-001.4:** When a binary is missing or its check
  answers with a nonzero exit, the system shall not measure the check again.
- **AC-AGENTS-INSTALL-DETECTION-001.5:** The bounds used by any single agent's
  detection shall sum to less than the discovery budget.
- **AC-AGENTS-INSTALL-DETECTION-001.6:** Agents other than the one whose check
  is measurably slower shall keep their existing detection behavior and bounds.
- **AC-AGENTS-INSTALL-DETECTION-001.7:** The system shall not add a
  configuration key, setting, or exported API to control a detection bound.

## Why

`WithCommandCheck` bounded every capability check at one shared timeout and
returned the same unavailable result for an elapsed bound as for a nonzero exit.
A caller could not tell an unhealthy install from a slow measurement, so it had
no basis to measure again.

On the reporting host, `hermes acp --check` exits 0 in about 2.0s when run
alone, about 3.0s at 8-way concurrency, and 5.5s to 12.1s at the 24-way
concurrency of a full discovery sweep. Against the shared 5s bound, a healthy
Hermes was reported as not installed on every sweep, not intermittently.

## Failure modes

- If the second measurement also fails, the agent is reported unavailable. A
  genuinely unusable or hanging install is never reported as available.
- If the caller cancels during either measurement, discovery surfaces the
  cancellation error as before.
- Callers that check agents one at a time wait up to the sum of the slower
  agent's bounds instead of the shared bound, but only when its first bound
  elapses.
- If the discovery budget ever tightens below the sum of an agent's bounds, that
  agent's bounds must be revisited so the sweep stays bounded.

## Out of scope

- Changing the discovery budget, installation-probe concurrency, or cache
  policy. Host CLI version checks may begin as installation results arrive
  within the existing sweep budget.
- Changing the bound of any agent other than the one whose check is measurably
  slower.
- New configuration, settings, or an exported API for detection bounds.
- Replacing capability checks with a faster probe, or detecting installation
  through package metadata instead of the agent's own check.
- Authentication and ACP protocol compatibility, which remain with the ACP
  probe path.

## Implementation plan

[Agent install detection bounds](../../../plans/agent-install-detection-bounds/plan.md)
