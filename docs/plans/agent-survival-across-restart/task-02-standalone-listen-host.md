---
id: "02-standalone-listen-host"
title: "Keep locally launched agentctl listeners on the standalone host"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-CONTROL-OWNERSHIP-001
acceptance_criteria:
  - AC-EXECUTORS-CONTROL-OWNERSHIP-001.12
system_design:
  - ../../specs/executors/system-design/agent-survival-across-restart-03.md
---

# Task 02: Keep locally launched agentctl listeners on the standalone host

## Summary

The standalone launcher starts agentctl with a bootstrap nonce, so agentctl
holds an auth token and binds its control server and every instance server to
all interfaces. The backend dials them only at `agent.standaloneHost`
(`127.0.0.1` by default). Pass that host to the child as
`AGENTCTL_LISTEN_HOST`. Keep injected Kandev MCP endpoints reachable from each
local agent, and make preferred-port checks and fallback-port probes follow the
addresses where the child will listen.

## In scope

- Add `AGENTCTL_LISTEN_HOST=<agent.standaloneHost>` to the launcher-owned
  child environment, replacing an inherited value.
- Advertise agent-facing Kandev MCP URLs at the restricted listener host, or
  at loopback when the listener accepts all interfaces. Apply this to ACP,
  passthrough, Cursor, and exact injected-permission recognition paths.
- Probe the effective listener addresses for the preferred port and fallback
  port. Resolve host names, expand wildcard hosts to local interface addresses,
  use bounded connect and bind checks, and never open a temporary wildcard
  listener.
- Update the `agent.standaloneHost` configuration row and the Windows
  firewall note in public docs.

## Out of scope

- The backend HTTP listener default (`server.host`, `0.0.0.0`).
- agentctl launched by the Docker, remote Docker, Sprites, SSH, and
  Kubernetes executors, which set their own environment.
- `Config.ListenHost` itself, the auth-disabled loopback fallback, and the
  websocket port tunnel bind.
- The shared task-process port allocator (`portutil.AllocatePort`).
- Listeners started inside agentctl for editors and previews.

## Acceptance

- `AC-EXECUTORS-CONTROL-OWNERSHIP-001.12` describes the behavior covered by
  this work order.

## Verification

```bash
cd apps/backend && go test -race ./internal/common/netprobe ./internal/agent/runtime/agentctl/launcher ./internal/agentctl/server/config ./internal/agentctl/server/api ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle -count=1
cd apps/backend && golangci-lint run ./internal/common/netprobe/... ./internal/agent/runtime/agentctl/launcher/... ./internal/agentctl/server/config/... ./internal/agentctl/server/api/... ./internal/agentctl/server/process/... ./internal/agent/runtime/lifecycle/... --new-from-rev=<base-sha> --timeout=10m
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Results

- The child environment carries exactly one `AGENTCTL_LISTEN_HOST` entry
  equal to the configured host for `127.0.0.1`, an empty host (`localhost`),
  `[::1]`, and a non-loopback address, including when the backend inherited a
  different value.
- Before the change the child environment had no entry, or kept the
  inherited `0.0.0.0`.
- MCP HTTP and SSE endpoints use the effective agentctl listener host. The
  listener reachability test connects to both injected endpoints on a
  non-loopback interface.
- The preferred-port check detects a listener bound only to a configured
  non-loopback address. Fallback probes resolve host names and use concrete
  interface addresses, with bounded retries and no wildcard probe listener.
- The permission policy recognizes only the trusted endpoint for the current
  host and port. Passthrough and Cursor configs use the standalone host, while
  non-standalone agents keep their execution-local loopback endpoint.
