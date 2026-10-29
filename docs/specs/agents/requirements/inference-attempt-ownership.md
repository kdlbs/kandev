---
title: Inference attempt ownership
system: agents
status: active
created: 2026-10-15
owners: [backend]
---

# Inference attempt ownership

### REQ-AGENTS-INFERENCE-OWNERSHIP-001: Observe actual one-shot children

Agentctl owns each ACP or Codex inference child separately, including legacy
execution while admission policy is disabled. Capability/model probes do not
create inference attempts. Parent task/session identity comes from the instance
configuration; an attempt token identifies one invocation, not its parent.

#### Acceptance criteria

- **AC-AGENTS-INFERENCE-OWNERSHIP-001.1:** Authenticated instance APIs and the existing
client expose live attempt identifiers, owner identity, dispatch state and
known/settled status without prompts, credentials, environment, PIDs or paths.

- **AC-AGENTS-INFERENCE-OWNERSHIP-001.2:** HTTP return, cancellation and a failed
inference result do not prove termination. Only no child start or observed
owned-process termination settles an attempt; uncertainty remains explicit.

- **AC-AGENTS-INFERENCE-OWNERSHIP-001.3:** Reconnection preserves the live owner
inventory. Retained owner metadata permits restart reconciliation; unavailable
history, ambiguous spawn intent and failed observations never become zero.

- **AC-AGENTS-INFERENCE-OWNERSHIP-001.4:** Preserve native execution, model order,
probe behavior, authentication and cleanup. Add no admission policy, second
queue, poller, operating changes or permanent tests in this increment.

### REQ-AGENTS-INFERENCE-OWNERSHIP-002: Share capacity and settle in order

Owned inference attempts participate in the same optional overall and grade
capacity as task launches. The backend persists the exact attempt binding before
starting the child, admits it under the shared owner, and releases the slot only
from definite evidence. Current inventory and historical continuity remain
distinct.

#### Acceptance criteria

- **AC-AGENTS-INFERENCE-OWNERSHIP-002.1:** Finite `all` capacity counts task-launch
  rows and owned inference bindings in the same overall and grade limits. Existing
  ownership keeps its slot across capacity-setting changes, and an unknown
  population still refuses new admission.

- **AC-AGENTS-INFERENCE-OWNERSHIP-002.2:** A retained attempt token is persisted
  with its exact owner, instance, attempt and reservation identity before the
  child starts. Terminal observation, provider settlement, binding removal and
  retained-token acknowledgement keep that order.

- **AC-AGENTS-INFERENCE-OWNERSHIP-002.3:** A live current inventory and retained
  historical continuity are reported separately. Reconnection preserves the live
  inventory; unavailable history alone does not make a complete current registry
  unknown, and one unreadable execution does not blank the whole population.

- **AC-AGENTS-INFERENCE-OWNERSHIP-002.4:** Teardown forwards the owner's stop
  observation through the retained workspace stop path. Only no child start or
  observed owned-process termination settles an attempt; unresolved outcomes stay
  unknown. A recovered run owner leaves the capacity census only after terminal
  persistence.

- **AC-AGENTS-INFERENCE-OWNERSHIP-002.5:** Preserve opt-in-off behavior, native
  execution, model/profile ordering, authentication and cleanup. Add no second
  queue, background poller, database or standing test framework.
