---
status: active
system: agents
created: 2026-11-11
owners:
  - kandev
---

# Built-in MCP Caller Authentication Requirements

## Overview

Every agentctl instance serves the built-in Kandev MCP server for exactly one
task session. The server binds every tool call, including
`ask_user_question_kandev`, to that session. It does not identify the caller.
Today any local process that reaches the instance port can call the tools and
act as that session.

On 2026-11-06 an agent in one workspace called the built-in MCP endpoint of a
session in another workspace through a hard-coded port. Kandev created the
clarification in the wrong session, cancelled that session's turns while the
question waited, and delivered the user's answer to the wrong agent. Earlier, on
2026-11-02, port reuse produced the same effect in a third task.

This capability makes the built-in MCP endpoint accept only the agent that
Kandev launched for that instance. The agents system owns it because built-in
MCP package delivery, the injected server identity, and provider delivery
strategies are agent-facing runtime contracts. The executor system keeps
ownership of the agentctl control credential and of port allocation.

## Terminology

- **Instance:** One agentctl instance bound to one task session and one port.
- **Built-in MCP endpoint:** The core and package MCP routes that an instance
  serves for the streamable HTTP, SSE, and SSE message transports.
- **MCP caller credential:** A secret that an instance issues for its own
  built-in MCP endpoint. It is separate from the agentctl control credential.
- **Managed agent:** An agent process that Kandev launches and supplies with the
  built-in MCP server configuration.
- **Delivery path:** One mechanism that gives a managed agent the built-in
  server entries: ACP session setup, native Codex app-server, a passthrough
  strategy, or project-file delivery.
- **Enforcement:** The mode in which the built-in MCP endpoint rejects requests
  that do not present the instance's own credential.

## Requirements

### REQ-AGENTS-BUILTIN-MCP-CALLER-AUTH-001: Instance-bound built-in MCP access

**Intent:** A tool call reaches a session only when it comes from the agent that
Kandev configured for that session.

#### Acceptance criteria

- **AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-001.1:** When enforcement is enabled and a
  request to any built-in MCP endpoint route carries no MCP caller credential,
  the instance shall respond with HTTP 401. It shall not run a tool, create a
  clarification or message, or change session state.
- **AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-001.2:** When enforcement is enabled and a
  request carries another instance's credential, the instance shall respond with
  HTTP 401. This includes the credential of an earlier instance that used the
  same port.
- **AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-001.3:** When a request carries the
  instance's own credential, tool discovery, tool calls, progress notifications,
  and long-running `ask_user_question_kandev` waits shall behave as they do
  without this capability.
- **AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-001.4:** The MCP caller credential shall
  not authorize any non-MCP instance route or control-server route. The control
  credential shall not authorize a built-in MCP endpoint route.
- **AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-001.5:** Each new instance created with
  enforcement enabled shall receive a new, unpredictable credential of at least
  256 bits. The credential shall be valid only while that instance exists.
- **AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-001.6:** The instance health route shall
  stay reachable without any credential.

### REQ-AGENTS-BUILTIN-MCP-CALLER-AUTH-002: Credential delivery to managed agents

**Intent:** Every managed agent keeps working native Kandev tools after
enforcement starts.

#### Acceptance criteria

- **AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-002.1:** When enforcement is enabled and
  Kandev supplies built-in server entries through any delivery path, each entry
  shall include the instance's credential in the form that the path supports.
- **AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-002.2:** When enforcement is enabled and
  an ACP session is restored through load or resume, or when an agent is
  relaunched against a surviving instance, the supplied entries shall carry the
  same credential as the original launch. This includes a relaunch after a
  backend restart.
- **AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-002.3:** When a built-in entry carries the
  credential, automatic approval of injected Kandev tools and managed
  conversation checks shall recognize it exactly as they recognize the current
  built-in entry. An entry with a different URL, credential, or extra header
  shall not receive that treatment.
- **AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-002.4:** The mock agent used by
  end-to-end tests, in ACP and TUI modes, shall present the credential when
  enforcement is enabled. Clarification end-to-end scenarios shall pass with
  enforcement enabled.

### REQ-AGENTS-BUILTIN-MCP-CALLER-AUTH-003: Rollout control and diagnostics

**Intent:** Operators can enable enforcement safely and see rejected callers
without exposing the credential.

#### Acceptance criteria

- **AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-003.1:** When the
  `builtinMcpCallerAuth` runtime feature is disabled, built-in MCP endpoints
  shall accept requests without a credential, as before this capability.
  Kandev shall not issue or deliver a credential. It shall build header-less
  entries that preserve the pre-capability provider configuration.
- **AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-003.2:** The enforcement mode of an
  instance shall be fixed when the instance is created. A change to the feature
  value shall apply only to instances created later.
- **AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-003.3:** When an instance rejects a
  built-in MCP request, it shall log the route, the instance session, and
  whether the credential was missing or invalid. Logs shall not repeat a rejection
  for the same route and reason more than once per minute.
- **AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-003.4:** The credential value shall not
  appear in Kandev backend or agentctl logs, ACP debug frame logs, diagnostic
  bundles, session events, or API responses visible in the web application.
- **AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-003.5:** When enforcement rejects a
  request without a credential, the response shall not advertise OAuth
  protected-resource metadata.

## Out of scope

- Isolation from a process of the same operating-system user that reads another
  agent's command line, environment, or configuration file on purpose. Some
  delivery paths put the credential in argv or in workspace files.
- Rejecting new clarification requests on a session without an active turn.
- The 180-second MCP tool-call limit in the third-party `atomica-code` agent.
- Authentication of the backend's own external `/mcp` endpoint, which has its
  own token contract.
- The OpenCode plugin host, host utility agents, and debug MCP sentinels, which
  do not serve the built-in MCP endpoint.
- Making enforcement the production default. That is a separate promotion
  decision after provider compatibility evidence exists.
