---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-PROXY-CREDENTIAL-001
---

# Proxy Control Credential Containment System Design

## Purpose and boundaries

The gateway's `port_proxy.go`, `port_tunnel.go`, and `vscode_proxy.go` authenticate
browser access and obtain the existing agentctl transport. That transport adds
its host-owned Authorization credential for agentctl. Agentctl must consume it
at its control boundary before forwarding into an executor application.
The gateway-to-agentctl authentication and browser ownership checks are unchanged.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-EXECUTORS-PROXY-CREDENTIAL-001` | [Consumed credential projection](#consumed-credential-projection) |

## Consumed credential projection

In `internal/agentctl/server/api`, successful authenticated branches of
`instanceAuth`, `controlCredentialAuth`, and the legacy `bearerTokenAuth` mark
control-credential consumption in trusted Gin request state. Do not infer it
from a nonempty Authorization header, an unverified token, or whether a rotating
credential source exists. Static authenticated requests and rotating requests
must agree; disabled-auth and exempt branches must not mark consumption.

Both `handlePortProxy` and `handleVSCodeProxy` already call
`credentialScopedRequest`. Extend that projection to clone a consumed request
and remove Authorization only from the outbound clone. Preserve the original
request for control handlers and middleware. Reuse the invalidation channel
captured during authentication; do not perform another credential lookup or
alter generation fencing. The header and cancellation projections compose in
that helper before the HTTP reverse proxy runs, including upgraded connections.

The gateway still sends a valid agentctl credential. Existing tests expecting
Bearer on the gateway-to-agentctl hop remain correct. A fake agentctl must mimic
consumption after authentication if it is used to test destination behavior;
never satisfy destination tests merely by stripping at the gateway and causing
agentctl admission to fail.

## Failure, compatibility, and evidence

Unauthenticated or invalid control requests never reach the application.
Disabled-auth requests retain original application Authorization. Existing
settings interlocks and Office-bearer rejection remain unchanged. No database
migration, new public header, token persistence, or credential logging is needed.
The authenticated path already reserves Authorization for agentctl; preserving
an additional original application credential across that path is a separate
contract, not part of this repair.

Use live HTTP integration with real agentctl auth and the real settings-interlock
middleware to compare direct and forwarded writes. Cover accepted static and
rotating credentials, rejected credentials, disabled authentication with app
headers, POST/PUT bodies, streaming, and WebSocket upgrades. Preserve the current
rotation-stream and gateway transport tests.

## Deployment boundary

The active :38383 tunnel belongs to the main backend and forwards through that
session's agentctl, rather than through the sandbox's own :49593 control server.
Updating only the sandbox app cannot fix this hop. Validation must use an owned
proxy built from the repaired code. Replacing or restarting the active main
runtime is outside this package; preserve :9998 and disclose that limitation.

## Related decisions

- [Control credential containment](../../../decisions/2026-10-06-proxy-control-credential-containment.md)
- [Opt-in authentication](../../../decisions/2026-07-24-opt-in-authentication.md)
- [Remote executor boundary](../../../decisions/2026-09-26-remote-executor-plugin-boundary.md)
