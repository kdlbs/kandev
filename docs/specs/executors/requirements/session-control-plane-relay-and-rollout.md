---
status: draft
system: executors
created: 2026-10-26
owners:
  - kandev
---

# Session control-plane relay and rollout requirements

## Overview

Companion to [session-control-plane-isolation](session-control-plane-isolation.md), which
holds the definitions and REQ-001. This document holds the Git broker relay (REQ-002), the
bind decision (REQ-003) and the runtime flag rollout (REQ-004). Terms are defined there.

## Requirements

### REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-002: Git credential broker through the session's agentctl

**Intent:** For same-host executors the Git credential helper reaches the broker
through the session's own agentctl port, so a network rule can allow Git without
allowing the backend's REST API or external `/mcp`.

#### Acceptance criteria

- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.1:** For a same-host executor whose instance holds an upstream broker URL, the broker URL given to any process agentctl starts for the instance (agent, terminals, readiness probe) in its environment, Git or helper configuration, or CLI shim shall be `http://<loopback listen address>:<instance port>/git-credentials/resolve`, composed by agentctl from its own listener. The create request shall keep the broker URL environment value (the managed-launch signal) and also send it as the upstream broker URL.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.2:** The relay shall forward `GET` (readiness) and `POST` (resolve) at exactly `/git-credentials/resolve` to the upstream broker URL, and `POST` at exactly `/git-credentials/reissue` to the upstream URL with its trailing `/resolve` replaced by `/reissue`. It shall return the upstream status and body unchanged, not following 3xx. Any other method on those paths gets 405; any other path under `/git-credentials/` gets 404. An upstream URL that does not end in `/resolve` is rejected at create with HTTP 400.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.3:** The caller shall not influence the upstream URL, path, or query. The relay forwards only `Content-Type` and `Accept` (the lease is in the body), returns only the status, body and `Content-Type`, and adds no backend credential.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.4:** The relay shall evaluate a request in this order and stop at the first failure: path (404), method (405), upstream configured (404, AC-002.6), request body over 64 KiB (413), body read failure (400); nothing is forwarded for these. It shall then forward, and answer 504 when the upstream has not completed within 15 seconds in total, and 502 when the upstream is unreachable, its response exceeds 1 MiB, or reading it fails (nothing partial is returned). Error bodies are generic JSON that omit the upstream URL.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.5:** The relay shall forward each request once: no retry, replay or caching, and cancel upstream when the caller disconnects.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.6:** An instance created without an upstream broker URL answers an allowed method on the relay paths with 404 (AC-002.4 order) and leaves process-visible broker state untouched.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.7:** Lease semantics, validation, and backend broker routes are unchanged.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.8:** For a remote executor, the broker URL remains the operator-configured URL, still validated by `validateGitHubCredentialBrokerURL` and still HTTPS. No relay is used and no upstream field is sent.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.9:** The launch-time broker readiness probe shall run against the URL the agent will use. For a same-host executor its limits shall exceed the relay's 15 seconds (design) so a relay 504 is observable; remote executors keep today's limits. An unreachable relay or upstream fails the launch as an unreachable broker does today.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.10:** For a same-host executor, no broker or Git credential environment variable, Git configuration entry, helper configuration, or CLI shim given to an agent or tool process shall contain the backend host or port. An operator public base URL is the relay's upstream, not visible to the agent.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.11:** The relay neither requires nor forwards the session MCP token and is served without the control credential; authorization is the lease in the body, validated upstream.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.12:** Each instance shall advertise on `/identity` a one-way fingerprint of its upstream broker URL (empty when none). The backend shall not adopt a retained instance whose fingerprint differs from that of the upstream URL it would send in a create request now (empty when none, and always empty for a remote executor); that instance is handled as in AC-001.11.

### REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-003: Default backend bind address

**Intent:** The default bind decision is recorded with its reason, and no
default changes without a gated rollout.

#### Acceptance criteria

- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.1:** The default of `server.host` shall remain `0.0.0.0`.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.2:** The design shall record why the default is unchanged and the scope of a gated loopback follow-up. This capability files no follow-up work item and changes no bind default.

### REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-004: Runtime flag rollout

**Intent:** The capability reaches no installation until an operator turns it on, and an installation with it off behaves as it does without this capability.

#### Acceptance criteria

- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-004.1:** The flag `features.sessionMcpToken` shall be registered in the runtime flag registry with the environment variable `KANDEV_FEATURES_SESSION_MCP_TOKEN`, restart required, and shall default to off in every profile (`prod`, `dev` and `e2e`). Effective values follow the existing order: explicit environment variable, stored override, profile default.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-004.2:** While the flag is off, the backend shall do no work for REQ-001 and REQ-002 on any launch, recovery or startup path: it shall create or read no session key, mint no nonce, send no token and no upstream broker URL in a create request, write no `mcp_token_nonce`, run no identity verifier, no revoke and no fingerprint-preconditioned delete, add no token or placeholder to any MCP entry, file or environment, not require `session_mcp_token` at control-server adoption, apply no `mcp_token_*` launch failure (an agent that AC-001.6 would refuse launches as it does today), and give agents the broker URL it gives them today. The create request body, the persisted execution record (for a launch whose record carries no `mcp_token_nonce` from earlier flag-on use; a carried nonce is preserved, not created) and the agent's MCP configuration shall be identical to those produced without this capability.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-004.3:** agentctl shall hold no flag. It shall enforce only on an instance whose create request carries a token, or that has since received a revoke (AC-001.4 defines the legacy and revoked instances) and relay only on an instance whose create request carries an upstream broker URL. The managed-policy recognition of the token header (AC-001.14) and the redaction of `Authorization` values in the ACP frame debug log shall apply unconditionally, because neither changes behaviour for an entry that carries no token. The Codex version probe, the Codex token environment variable, the revoke route and the fingerprint-precondition header act only on request content as the design's trigger table states, and the `/git-credentials/` exemption from control authentication applies only to an instance holding an upstream broker URL, so a legacy instance's authorization outcomes are today's.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-004.4:** Across an upgrade or downgrade, a backend and an agentctl that disagree on this capability shall behave per the design's compatibility matrix: flag on with an agentctl lacking `session_mcp_token` fails the launch with `mcp_token_unsupported` and a retained local, Sprites, remote Docker or plugin one is replaced and a retained Kubernetes or SSH one is refused per AC-001.11; flag off with any agentctl produces the backend behaviour of AC-004.2, except that a retained enforcing instance from earlier flag-on use keeps enforcing and is unreachable by token-less entries the flag-off backend writes (Claude and OpenCode passthrough files) until it is replaced, while entries agentctl composes keep working (design, per carrier class). Two further retained-instance outcomes hold after flag on to off, pinned by tests: a retained instance already revoked (a Kubernetes or SSH instance a flag-on backend deferred whose revoke route answered success, or any instance left in an unknown outcome whose revoke succeeded; an instance whose revoke was not accepted, for example by an agentctl without the route, stays as it was and is handled as a legacy instance) is adopted by the flag-off backend, which runs no verifier, and answers every MCP route for every carrier with 503 `mcp_token_unconfigured` until the session is stopped and the instance replaced; and a retained instance holding an upstream broker URL keeps relaying and keeps composing the relay URL for agents it starts, until it is replaced. Codex passthrough is in the token-less group (its `env_http_headers` argument is built by the backend).
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-004.5:** The public executors documentation shall state the flag name, how to enable it, that a restart is required, the upgrade order (backend before operator-pinned images, then enable), the effects at the first start with the flag on (retained sessions relaunch, refused agents and executors, operator-pinned images updated first) and the effect of turning it off, including the revoked-instance and retained-relay outcomes of AC-004.4 and that sessions on Kubernetes or SSH should be stopped before the flag is turned off.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-004.6:** The flag is off in the `dev` and `e2e` profiles, so no existing e2e spec and no `make dev` run exercises the capability unless it sets `KANDEV_FEATURES_SESSION_MCP_TOKEN=true`. The mock agents' MCP clients shall forward the `Authorization` header they are configured with, so that a spec which enables the flag can launch a mock agent: the ACP mock reads it from the `Authorization` header of its `session/new` `mcpServers` entry and the mock TUI agent from the `headers` map of the `--mcp-config` file its Claude-class carrier writes (design). Each is observed by a recorded request against a stub MCP server. An e2e spec that enables the flag uses only agents whose carrier is available (AC-001.6).
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-004.7:** The registry entry shall carry the label, description, stability, risk level and risk description the design gives, be mutable and available on every platform, and be mirrored in the frontend feature types.
