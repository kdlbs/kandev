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
