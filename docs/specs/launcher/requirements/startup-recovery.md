---
status: active
system: launcher
created: 2026-08-24
owners:
  - kandev
---

# Startup Recovery Requirements

## Overview

Operators can bind Kandev to loopback, a specific network interface, multiple
interfaces, or a wildcard address. The native launcher must reach the backend
without requiring an undocumented loopback listener. If startup fails, the
launcher must identify what failed and give the operator a direct next step.

## Terminology

- **Bind address:** An effective address on which the backend tries to accept
  HTTP connections.
- **Health target:** A local HTTP URL that the launcher probes for readiness.
- **Access URL:** The URL that the launcher prints or opens for a local user.
- **Owned backend:** A backend whose health response contains the launcher's
  per-launch health token.

## Requirements

### REQ-LAUNCHER-STARTUP-001: Bind-aware readiness

**Intent:** Let an operator select a valid bind address without also having to
understand the launcher's readiness implementation.

#### Acceptance criteria

- **AC-LAUNCHER-STARTUP-001.1:** When the effective configuration contains one
  specific non-loopback bind address, the launcher shall probe that address and
  shall not require a loopback listener.
- **AC-LAUNCHER-STARTUP-001.2:** When the effective bind address is an IPv4 or
  IPv6 wildcard, the launcher shall probe the corresponding loopback family.
- **AC-LAUNCHER-STARTUP-001.3:** When the effective configuration contains
  multiple bind addresses, the launcher shall try every health target and
  shall become ready after any target returns the matching health token.
- **AC-LAUNCHER-STARTUP-001.4:** When at least one configured address serves the
  owned backend, a failed or unavailable sibling address shall not block
  startup.
- **AC-LAUNCHER-STARTUP-001.5:** When the launcher prints or opens an access
  URL, that URL shall use a reachable effective bind and shall prefer loopback.
  For an IPv4 wildcard, readiness shall use `127.0.0.1` while the default
  browser URL shall remain `http://localhost:<port>` to preserve the established
  browser origin. An IPv6 wildcard shall use `[::1]` for readiness and access.
- **AC-LAUNCHER-STARTUP-001.6:** The `dev`, `start`, and `run` modes shall use
  the same bind and readiness rules.

### REQ-LAUNCHER-STARTUP-002: Actionable startup failure

**Intent:** Explain a startup failure without requiring the operator to infer
the cause from unrelated backend or ACP log lines.

#### Acceptance criteria

- **AC-LAUNCHER-STARTUP-002.1:** When backend readiness fails, the launcher
  shall classify the result as an early backend exit, an unreachable backend,
  an unhealthy HTTP response, or a response from a different process.
- **AC-LAUNCHER-STARTUP-002.2:** When the backend is still running at the
  readiness deadline, the launcher shall state that it stopped the backend
  after readiness failed. It shall not describe that result as a backend crash.
- **AC-LAUNCHER-STARTUP-002.3:** A readiness failure shall show the effective
  bind addresses, attempted health targets, last safe outcome for each target,
  configuration source, backend log path, and one relevant next step.
- **AC-LAUNCHER-STARTUP-002.4:** When a health response contains a missing or
  different launcher token, the failure shall identify a different process on
  the selected port as the likely cause.
- **AC-LAUNCHER-STARTUP-002.5:** Startup diagnostics shall not print health
  tokens, secrets, database credentials, or other configuration values marked
  as sensitive.
- **AC-LAUNCHER-STARTUP-002.6:** When the launcher cannot classify the cause,
  it shall preserve the backend output and shall link to the startup
  troubleshooting guide.

### REQ-LAUNCHER-STARTUP-003: Proxy configuration guidance

**Intent:** Help an operator distinguish startup readiness from reverse-proxy
trust configuration.

#### Acceptance criteria

- **AC-LAUNCHER-STARTUP-003.1:** Public startup troubleshooting shall state
  that ACP probe closure messages and forwarded-host warnings do not cause a
  launcher health timeout.
- **AC-LAUNCHER-STARTUP-003.2:** Public trusted-proxy guidance shall show both
  exact-IP and CIDR configuration and shall identify the immediate proxy peer,
  not the browser client, as the trusted address.
- **AC-LAUNCHER-STARTUP-003.3:** Public guidance shall warn that a trusted CIDR
  lets directly connected hosts in that range supply forwarded identity
  headers.

## Out of scope

- Automatically trusting private, container, or cluster networks.
- Inferring a subnet mask from one observed proxy address.
- Changing the `/health` response or health-token contract.
- Parsing arbitrary backend log text to guess subsystem failures.
- Adding a new configuration key or Settings UI for startup configuration.
