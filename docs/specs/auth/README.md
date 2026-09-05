---
status: draft
system: auth
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# Authorization and identity system

## Purpose

The authorization and identity system owns authentication state, trust
boundaries, session credentials, and authorization checks shared by Kandev
surfaces.

## Ownership

This system owns authenticated identity, trusted proxy handling, session and
cookie boundaries, self-action guards, share-link access, and security checks
for user and service requests.

## Exclusions

- Provider credentials and external service connections belong to the
  [integration system](../integrations/README.md).
- Agent permission policy belongs to the [agent system](../agents/README.md).

## Specification map

### Requirements



- [Opt-in Authentication & Multi-User Segregation](requirements/auth.md)
- [Org Roles and Scopes](requirements/roles-and-scopes.md)
- [Isolate Kandev cookies between instances on one host](requirements/fix-multi-instance-cookie-isolation.md)
- [Public Share Links (v0)](requirements/public-share-links.md)
- [Repair secure-context browser fallbacks](requirements/secure-context-browser-fallbacks.md)
- [Self-Actions Guard in System Users](requirements/self-actions-guard.md)
- [Session IP Refresh](requirements/session-ip-refresh.md)
- [Trusted Proxies for X-Forwarded-For](requirements/trusted-proxies.md)

### System design



- [Isolate Kandev cookies between instances on one host](system-design/fix-multi-instance-cookie-isolation.md)
- [Public Share Links (v0)](system-design/public-share-links.md)

## Migration record

Migration remains in progress while legacy source detail is extracted from the
canonical requirement and system-design documents above.

## Related systems

- [Integrations](../integrations/README.md): owns external provider auth.
- [Platform](../platform/README.md): owns process-level security boundaries.
