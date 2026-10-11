---
status: draft
system: executors
created: 2026-10-07
owners:
  - kandev
---

# Kubernetes validation isolation

## Overview

Heavy repository validation on an opted-in full-toolchain Kubernetes worker must
have its own bounded resource lifetime so a validation OOM does not terminate
agentctl or the native agent conversation. The executor system owns this runtime
contract. It complements [failure recovery](kubernetes-failure-recovery.md) and
[Docker workloads](kubernetes-docker-workloads.md).

## Requirements

### REQ-EXECUTORS-K8S-VALIDATION-001: Bounded task validation

**Intent:** Preserve the agent session while executing heavy validation.

#### Acceptance criteria

- **AC-EXECUTORS-K8S-VALIDATION-001.1:** An explicitly enabled full-worker
  validation runner shall execute lint, compilation/build, and browser validation
  in a separately limited workload under the task's admitted Docker companion.
  A workload OOM shall fail that check without restarting the agent container or
  changing its active native conversation.
- **AC-EXECUTORS-K8S-VALIDATION-001.2:** All supported heavy-check entry points
  in the enabled worker shall share one task-wide admission slot, including
  requests from sibling sessions. The runner shall bound memory, swap, CPU,
  process count, queue wait, and execution time. Memory admission shall reserve
  companion capacity for its daemon and account for other running workloads.
- **AC-EXECUTORS-K8S-VALIDATION-001.3:** Unsupported or unverifiable daemon
  accounting, missing immutable image, unavailable isolation, invalid budgets,
  and admission timeout shall fail before heavy work starts. Enabled entry points
  shall not silently fall back to validation in the agent container.
- **AC-EXECUTORS-K8S-VALIDATION-001.4:** The runner shall preserve argument
  boundaries, working directory within the canonical workspace, output and exit
  status, and durable workspace artifacts. Cancellation or timeout shall stop
  only its owned workload; abandoned or completed workloads shall be safely
  reconciled before reusing the slot.
- **AC-EXECUTORS-K8S-VALIDATION-001.5:** Validation workloads shall receive
  only explicit workspace access and an allowlisted test environment. They shall
  not receive Kandev control/auth volumes, native-agent HOME, provider tokens, or
  a privileged daemon socket. The current task trust boundary remains unchanged.
- **AC-EXECUTORS-K8S-VALIDATION-001.6:** Existing workers and host/CI commands
  without explicit enablement shall retain their current behavior. The recipe
  shall document entry-point coverage, resource accounting evidence, rollout,
  and unsupported commands; arbitrary direct provider shell commands are outside
  the runner's enforcement boundary.

## Exclusions

Universal interception of native-agent shell tools, adversarial isolation between
trusted task sessions, node scheduling redesign, automatic production profile
mutation, additional privileged companions, Docker-executor E2E parity, and error
UI changes are excluded. Container/browser checks requiring daemon access or
agent-only callback fixtures remain separate workloads with explicit policy.
