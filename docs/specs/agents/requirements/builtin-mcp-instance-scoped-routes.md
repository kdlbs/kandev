---
status: draft
system: agents
created: 2026-11-16
owners:
  - kandev
---

# Instance-scoped built-in MCP routes

## Intent and ownership

An agent's built-in MCP endpoint must select its helper instance as well as its
port. Stale configuration must fail before it can act on another task.
Agents owns helper identity, native MCP delivery, and provider approval rules.
Task authorization and workflow gates retain their existing owners.

On 2026-11-06, a hard-coded port
sent a clarification from workspace A to a session in workspace B. The wrong
task paused, and its user's answer reached the wrong agent. On 2026-11-02, port
reuse caused the same failure through stale configuration. João Manso (`JnManso`)
documented these incidents in the
[requirements for PR #5294](https://github.com/JnManso/kandev/blob/e9e13e3aca498b5f8521ba100522e7809d686f78/docs/specs/agents/requirements/builtin-mcp-caller-auth.md).

## Terms

- **Helper instance:** One agentctl instance with its own listener and task-session binding.
- **Scoped instance:** An instance that accepts built-in MCP only below its own
  `/i/<instance-id>` prefix.
- **Legacy instance:** An instance that uses the prior bare MCP paths for compatibility.
- **Retained instance:** The same live helper across a connection or backend restart.
- **Replacement instance:** A newly created helper that replaces a prior helper.

## Requirements

### REQ-AGENTS-MCP-INSTANCE-001: Route isolation

**Intent:** Reject wrong-port and stale endpoint calls before task effects.

- **AC-AGENTS-MCP-INSTANCE-001.1:** A scoped instance shall accept supported
  MCP operations only below its own `/i/<instance-id>` prefix. This includes
  `/mcp`, `/sse`, `/message`, and their `/packages/<package-id>` variants.
- **AC-AGENTS-MCP-INSTANCE-001.2:** Bare built-in routes and routes naming a
  different instance shall return HTTP 404, with or without a control credential.
  They shall cause no MCP dispatch, question, turn cancellation, or task mutation.
- **AC-AGENTS-MCP-INSTANCE-001.3:** After a port passes to a different task
  execution, the old scoped URL shall return 404. The new execution's scoped
  URL shall work.
- **AC-AGENTS-MCP-INSTANCE-001.4:** An SSE endpoint event shall advertise the
  same instance and package prefix. Its message URL shall accept the associated
  client's valid POST without a control credential.
- **AC-AGENTS-MCP-INSTANCE-001.5:** Missing or ambiguous instance identity shall
  prevent scoped-instance creation. Invalid identity shall not create a listener
  or leave an allocation behind.

### REQ-AGENTS-MCP-INSTANCE-002: Consistent delivery and approval

**Intent:** Preserve usable native tools through every supported delivery channel.

- **AC-AGENTS-MCP-INSTANCE-002.1:** ACP injection shall deliver URLs matching
  the instance's effective routing mode, for monolithic and packaged tool catalogs.
- **AC-AGENTS-MCP-INSTANCE-002.2:** Claude and OpenCode files, Codex arguments,
  Pi project files, and Cursor project files shall use that same effective mode.
  This includes SSH passthrough and remote project-file preparation.
- **AC-AGENTS-MCP-INSTANCE-002.3:** For scoped instances, native auto-approval and managed
  tool policy shall accept the exact host-injected scoped endpoint. Bare,
  wrong-instance, foreign, or forged entries shall not acquire that approval.
  Legacy mode shall retain its existing exact-endpoint checks.
- **AC-AGENTS-MCP-INSTANCE-002.4:** Selected packages shall retain their separate
  catalogs and transport sessions. Unselected and unknown packages shall remain
  unavailable. Routing mode shall not change tool names, grants, or provenance.
- **AC-AGENTS-MCP-INSTANCE-002.5:** A valid ACP clarification shall appear only
  in its owning task, and its answer shall reach the originating agent. Supported
  TUI context calls shall identify the current task session through delivered URLs.

### REQ-AGENTS-MCP-INSTANCE-003: Compatible default activation

**Intent:** Protect new instances without breaking either direction of version skew.

- **AC-AGENTS-MCP-INSTANCE-003.1:** A current backend shall request scoped
  routing for every newly created managed instance by default. This behavior
  shall require no feature flag, operator setting, or provider header support.
- **AC-AGENTS-MCP-INSTANCE-003.2:** With an older helper that does not support
  scoped routing, a current backend shall continue to deliver legacy bare URLs.
  Existing package-support admission checks shall still apply.
- **AC-AGENTS-MCP-INSTANCE-003.3:** With an older backend that does not request
  scoped routing, a current helper shall serve and emit legacy bare routes.
- **AC-AGENTS-MCP-INSTANCE-003.4:** After the helper confirms scoped routing,
  an unavailable or rejected scoped endpoint shall not trigger a bare-path retry.
  Invalid or unreadable explicit routing evidence shall stop
  dependent delivery without rewriting files to a guessed endpoint.
- **AC-AGENTS-MCP-INSTANCE-003.5:** A retained legacy instance shall keep its
  effective mode until replacement. Adoption shall not silently migrate its URLs
  or restart the agent to activate scoping.

### REQ-AGENTS-MCP-INSTANCE-004: Identity continuity and unchanged authority

**Intent:** Keep the endpoint bound to the live helper through recovery.

- **AC-AGENTS-MCP-INSTANCE-004.1:** Adoption and connection recovery of the same
  live helper shall preserve its instance identity and routing mode. Existing MCP
  URLs shall remain valid without a new native session.
- **AC-AGENTS-MCP-INSTANCE-004.2:** Generated URLs shall use the accepted helper
  instance ID for the owning execution. Different task executions shall not
  share that prefix. A replacement for the same execution can reuse its helper ID.
- **AC-AGENTS-MCP-INSTANCE-004.3:** During staged replacement, endpoint delivery
  shall use the replacement helper's identity and mode. Failed preparation shall
  not publish its endpoint or overwrite the retained helper's accepted state.
- **AC-AGENTS-MCP-INSTANCE-004.4:** Health probes, control authentication, the
  external backend MCP endpoint, and third-party MCP configuration shall retain
  their existing contracts.
- **AC-AGENTS-MCP-INSTANCE-004.5:** Routing shall preserve task-owner checks,
  question barriers, title ownership, completion gates, autopilot rules, and
  operation-specific authorization. TUI question availability shall not expand.

## Exclusions

Instance IDs are public routing identifiers. Deliberate copying of another
workspace's complete URL remains outside this protection. This is not caller
authentication or a local-process sandbox.

This change adds no bearer credential, MCP header policy, runtime toggle,
environment kill switch, provider probe, settings control, or rendered UI.
It does not migrate live third-party configuration or retrofit protection
onto old helpers. Existing desktop and phone interactions remain unchanged.

## Design and delivery

- [System design](../system-design/builtin-mcp-instance-scoped-routes.md)
- [Implementation plan](../../../plans/builtin-mcp-instance-scoped-routes/plan.md)
- [Existing package authority](builtin-mcp-management.md)
