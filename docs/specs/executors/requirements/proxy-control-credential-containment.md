---
status: active
system: executors
created: 2026-10-06
owners:
  - kandev
---

# Proxy Control Credential Containment Requirements

## Overview

A browser preview or editor reaches an executor application through agentctl.
Executor control credentials authorize that forwarding hop, not the application.
Executors own the boundary because agentctl authenticates executor control and
forwards into the environment. Application authentication stays application-owned.

## Requirements

### REQ-EXECUTORS-PROXY-CREDENTIAL-001: Consumed control credentials stay at the control boundary

**Intent:** Applications must not receive a control credential merely because
Kandev authenticated the forwarding request.

#### Acceptance criteria

- **AC-EXECUTORS-PROXY-CREDENTIAL-001.1:** After agentctl accepts a control
  bearer credential for a port-preview or editor forwarding request, the
  destination application shall receive no such Authorization credential, for
  ordinary HTTP, streaming responses, or WebSocket upgrades.
- **AC-EXECUTORS-PROXY-CREDENTIAL-001.2:** Missing, invalid, expired, or
  superseded control credentials shall retain their existing admission and
  connection-fencing behavior. Containment shall not bypass authentication.
- **AC-EXECUTORS-PROXY-CREDENTIAL-001.3:** When agentctl authentication is
  disabled and does not consume Authorization, application Authorization shall
  retain its existing forwarding behavior, including Basic and Bearer values.
- **AC-EXECUTORS-PROXY-CREDENTIAL-001.4:** Forwarding shall preserve application
  request method/body/query, Host, interlock headers, cookies, and WebSocket
  application negotiation. A protected browser application with a valid local
  interlock shall behave the same directly and through an authenticated proxy.
- **AC-EXECUTORS-PROXY-CREDENTIAL-001.5:** Diagnostic evidence shall not expose
  control credentials. Application-facing authentication failures shall remain
  genuine application failures rather than being suppressed by the proxy.

## Out of scope

New channels for carrying application Authorization alongside control auth,
provider-specific lease-header policy, account permissions, gateway session
ownership, weakening browser settings interlocks, and running-instance upgrades.

## Implementation plans

- [MCP forwarding and session clarity](../../../plans/mcp-forwarding-session-clarity/plan.md)
