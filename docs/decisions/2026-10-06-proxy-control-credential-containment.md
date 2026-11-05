# ADR-2026-10-06-proxy-control-credential-containment: Consume control credentials before application forwarding

**Status:** accepted
**Date:** 2026-10-06
**Area:** backend, protocol, security

## Context

The gateway supplies agentctl Authorization to authenticate executor forwarding.
Agentctl currently leaves that header on port and editor proxy requests. A nested
Kandev test instance sees a bearer credential even though its browser sent none,
so its settings interlock rejects every write. Arbitrary executor apps also
receive a credential intended for executor control.

## Decision

Successful agentctl authentication marks consumption in trusted request state.
The shared proxied-request projection removes the consumed Authorization from
an outbound clone. Authentication, request-generation fencing, and gateway
session ownership continue to run before forwarding. Disabled authentication
keeps existing application-header behavior. Destination application credentials
are not inferred from or replaced by control credentials.

## Consequences

Protected executor applications can distinguish their own browser requests from
machine credentials. The credential stays at its intended boundary across HTTP
and WebSocket proxy paths. No application-side interlock relaxation is needed.
Live forwarding requires deployment of the agentctl that owns that hop; a nested
application restart alone does not update it.

## Alternatives Considered

- Strip Authorization in the gateway: prevents agentctl authentication.
- Strip every Authorization regardless of consumption: changes unauthenticated
  executor application behavior and discards application credentials.
- Accept the control credential in the nested application: confuses authority
  and weakens an independent settings protection.
- Add a test-only sanitizing relay: hides the shipping proxy defect and cannot
  establish that production forwarding contains credentials.

## Related contracts

- [Requirements](../specs/executors/requirements/proxy-control-credential-containment.md)
- [System design](../specs/executors/system-design/proxy-control-credential-containment.md)
