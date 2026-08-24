---
status: current
system: launcher
requirements:
  - REQ-LAUNCHER-STARTUP-001
  - REQ-LAUNCHER-STARTUP-002
  - REQ-LAUNCHER-STARTUP-003
---

# Startup Recovery System Design

## Purpose and boundaries

The native launcher derives readiness and access endpoints from the same
effective bind configuration that it passes to the backend. It records probe
facts at the launcher boundary and uses those facts for a concise startup
failure report. The launcher does not inspect ACP logs or infer trusted
networks.

The design applies to `dev`, `start`, and `run`. A managed service reaches the
same behavior when its service command invokes `run` in headless mode.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-LAUNCHER-STARTUP-001` | [Endpoint resolution](#endpoint-resolution), [Readiness flow](#readiness-flow) |
| `REQ-LAUNCHER-STARTUP-002` | [Probe evidence](#probe-evidence), [Failure presentation](#failure-presentation) |
| `REQ-LAUNCHER-STARTUP-003` | [Proxy guidance](#proxy-guidance), [Security](#security) |

## Components and responsibilities

- `internal/common/config.ServerConfig.ResolvedBinds` remains the authority for
  validated and de-duplicated bind addresses.
- `internal/launcher` resolves backend endpoints after it selects the backend
  port and before it starts the child process.
- `waitForHealth` probes the resolved health targets and returns typed evidence.
- `runManagedApp` and `runDev` present one failure summary and then use the
  existing supervisor shutdown path.
- `internal/auth/httpmw` continues to enforce forwarded-header trust. Public
  documentation explains exact-IP and CIDR choices.

## Endpoint resolution

The launcher creates one immutable endpoint set for each launch:

```text
backendEndpointSet
  bindHosts     effective values from ResolvedBinds
  healthTargets ordered local HTTP probe URLs
  accessURLs    browser URLs corresponding to each health target
  accessURL     preferred URL for output and browser opening
```

The resolver applies these rules:

1. Map `0.0.0.0` to `127.0.0.1` for the health target and retain `localhost`
   as the default browser/access URL.
2. Map `::` to `::1` and format it with brackets for both probing and access.
3. Keep a specific IP or hostname as a target.
4. Put loopback targets before non-loopback targets.
5. Remove duplicate URLs after wildcard mapping and hostname normalization.
6. Probe every target concurrently, but select a healthy target in priority
   order. A lower-priority success cannot win until every higher-priority
   target has completed without the launch token.
7. Use the access URL corresponding to the selected health target.

The endpoint set is separate from `portConfig`. Port selection remains a port
concern. Bind resolution remains a configuration concern.

## Probe evidence

Each health target has one latest observation:

```text
healthObservation
  URL
  outcome       healthy | connection_error | http_status | foreign_process
  statusCode    present only for an HTTP response
  safeDetail    bounded network error class without a token or response body
```

`backendHealthError` contains the failure class, deadline, child exit state,
and final observations. The type supports `errors.As`. Human text is formatted
only at the launcher presentation boundary.

The launcher never records or prints the expected token. It drains and closes
responses as it does now.

## Readiness flow

For each polling interval, the launcher probes all unresolved targets with the
shared deadline and the existing per-request timeout. Probes in one interval
run concurrently so an unreachable address cannot delay a healthy sibling.
The interval completes when all probes return or their request contexts end.
Selection still follows target priority: a LAN response that arrives before a
loopback response is held until the loopback probe completes, so a healthy
loopback target remains preferred.

A `2xx` response with the matching token makes the backend ready. Other target
results update their observations. A child exit ends polling immediately. The
existing overall deadline and cancellation behavior remain in force.

The launcher opens or prints `accessURL` only after an owned backend responds.
If a non-loopback-only bind is selected, the launcher uses that address instead
of printing an unreachable localhost URL.

## Failure presentation

The launcher prints the captured backend output once. It then prints one
summary with these fields:

- failure class and plain-language result
- whether the backend exited or the launcher stopped a live backend
- effective bind addresses and value source
- each health target and its last observation
- selected configuration-file path, or `defaults and environment`
- effective source for `server.host`, including `KANDEV_SERVER_HOST` when an
  environment override wins over the selected file
- expected backend-log path
- one next action plus the public troubleshooting URL.

The next action follows the failure class. A foreign-process result tells the
operator to free or change the port. An early exit tells the operator to read
the named backend log. A connection-only timeout tells the operator to inspect
binds, firewall rules, and environment overrides. A non-success HTTP result
shows the status code.

The report uses short stable labels. Tests assert the facts and labels, not a
large terminal snapshot.

## Proxy guidance

The launcher failure summary does not present trusted-proxy warnings as a
readiness cause. The CLI troubleshooting section explains that these warnings
occur only after a reverse proxy reaches the backend.

The configuration guide provides two copyable examples:

- one stable reverse-proxy IP
- a narrow CIDR for a controlled proxy network with dynamic addresses.

The guide tells the operator to use the `peer` value from the warning. It also
explains that the CIDR is the proxy network, not the client network.

## Failure and recovery

- If bind resolution fails, startup stops before the backend starts and names
  the invalid setting through existing configuration validation.
- If one target is unavailable and another target is healthy, startup
  succeeds.
- If every target remains unavailable, the launcher stops the owned backend
  through the existing graceful supervisor path.
- If the child exits, the launcher reports its exit code and does not wait for
  the readiness deadline.
- If a different process answers, the launcher never accepts it as ready.

## Security

The per-launch token remains mandatory for owned-backend readiness. Probe
errors include no response body or token.

Kandev does not automatically trust RFC 1918, loopback, link-local, container,
or cluster ranges. An automatic rule can let another directly connected host
inside that range supply `X-Forwarded-For` and affect login attribution and rate
limits. Operators explicitly select exact proxy addresses or controlled proxy
CIDRs.

## Observability

Startup output records the resolved bind set, access URL, health targets, and
failure class. Existing backend file logs and bounded captured stdout remain
the detailed evidence. ACP probe lifecycle messages remain separate from
backend startup readiness.

## Related decisions

- [The Go launcher owns every entrypoint](../../../decisions/2026-08-08-go-launcher-owns-all-launch-modes.md)
- [Startup configuration uses one typed source model](../../../decisions/2026-08-20-startup-configuration-source-parity.md)
