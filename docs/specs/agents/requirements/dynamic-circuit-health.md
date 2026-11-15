---
status: draft
system: agents
created: 2026-10-31
updated: 2026-11-01
owners:
  - cfl
---

# Dynamic Circuit Health Requirements

## Requirements

### REQ-AGENTS-DYNAMIC-CIRCUIT-HEALTH-001: Shared circuit admission and readback

#### Acceptance criteria

- **AC-AGENTS-DYNAMIC-CIRCUIT-HEALTH-001.1:** While `features.dynamicAgentRouting` is enabled, a new dispatch or prompt for a concrete profile shall respect its own shared resource circuit and owner-release account hold. Refusal shall preserve the configured profile and model, shall not choose a successor, and shall not stop an already running turn. An expired timed suspension or restored half-open probe shall not indefinitely refuse a fixed-profile dispatch.
- **AC-AGENTS-DYNAMIC-CIRCUIT-HEALTH-001.2:** A classified failure shall affect only the resource of the execution that produced it. Stale route generations, predecessor executions, unknown execution identity, and unclassified failures shall not charge another candidate. Recording a circuit shall not authorize failover after output or effects.
- **AC-AGENTS-DYNAMIC-CIRCUIT-HEALTH-001.3:** Fixed and dynamic profiles shall use the same shared circuit backoff policy. Enabling dynamic agents shall require no separate backoff opt-in. Existing routing selection, model order, exclusive probes, and owner-release holds shall retain their behavior. The default ladder and durable count contract are owned by AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.20 and AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.21.
- **AC-AGENTS-DYNAMIC-CIRCUIT-HEALTH-001.4:** A read-only API shall report a consistent copy of the live circuit state, resource scope, reason, next-attempt/probe time, retained failure count, and known recording time. A read shall not write, probe, retry, release, select, or launch. An unwired reader or failed read shall remain distinguishable from a real empty list.
- **AC-AGENTS-DYNAMIC-CIRCUIT-HEALTH-001.5:** The authorized integrated-control providers view shall display this installation-wide circuit readback, including open, half-open, and retained closed states. It shall distinguish loading, denied, unsupported, failed, and real empty reads; an empty list shall not claim provider reachability. A transient refresh failure may retain only the same authorized owner's dated snapshot, marked stale. Denied or unsupported reads shall clear it. Old-scope or cancelled reads shall not publish into a later control snapshot. Refresh shall follow the existing control read lifetime without polling.
- **AC-AGENTS-DYNAMIC-CIRCUIT-HEALTH-001.6:** The circuit view shall preserve desktop and phone readability, localization, and existing access boundaries. It shall not expose credentials, prompts, or raw logs. Provider-policy default/learned/override/effective budgets and their reset/display consumer remain a separate requirement; circuit readback shall not fabricate those values.

## System design

- [Dynamic circuit health](../system-design/dynamic-circuit-health.md)
- [Circuit backoff](../system-design/dynamic-circuit-backoff.md)
