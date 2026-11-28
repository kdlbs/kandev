---
status: current
system: launcher
requirements:
  - REQ-LAUNCHER-LOCAL-ONLY-001
---

# Local-only CLI startup system design

## Purpose and boundaries

The launcher, backend, and restart supervisor must agree on one effective
listener configuration. The shared typed configuration owns bind selection.
The launcher owns the readiness probe and access URL.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-LAUNCHER-LOCAL-ONLY-001` | [Bind selection](#bind-selection), [Launch and restart](#launch-and-restart), [Upgrade and documentation](#upgrade-and-documentation) |

## Components and responsibilities

- `internal/common/config` resolves configured binds and selects the fallback.
- `internal/launcher` derives readiness and browser targets from resolved binds.
- `Dockerfile` identifies the official container runtime so it can retain its
  existing bind fallback. The Kandev-managed service generators already set
  `KANDEV_RUNNING_AS_SERVICE=true`.
- The backend binds its HTTP and WebSocket server to the resolved addresses.

## Bind selection

Keep the raw host default empty so a selected YAML `server.hosts` list works.
Keep the existing precedence of `KANDEV_SERVER_HOST`, YAML `server.host`, and
YAML `server.hosts`. Validate, normalize, and deduplicate the selected values
with `ServerConfig.ResolvedBinds`. Apply the deployment fallback only when this
process has no usable configured entries. Never use the fallback after an
explicit bind fails validation or a socket fails to open.

The standalone fallback is the literal IPv4 loopback address `127.0.0.1`.
The official image sets `KANDEV_INTERNAL_CONTAINER_RUNTIME=1`. That marker and the
existing managed-service marker select `0.0.0.0` as the compatibility fallback.
Neither marker populates `KANDEV_SERVER_HOST` or overrides a YAML value. A
custom deployment that used an implicit wildcard should configure its bind
explicitly. Neither `--headless` nor browser suppression identifies a deployment.

## Launch and restart

Resolve the same deployment fallback in the launcher and backend through the
shared configuration loader. Carry the context through backend and supervisor
restarts using the existing child environment contract. Keep the configuration
catalog and diagnostics accurate about the default and its source.

For the implicit standalone bind, probe `127.0.0.1` and print or open
`http://localhost:<port>`. This preserves browser state tied to the URL used by
previous versions. Keep the existing endpoint mapping for explicit addresses.
Keep the existing launch token and readiness behavior.

## Upgrade and documentation

Apply the fallback on the next start. The change needs no database migration
or configuration write. An existing standalone deployment that relies on
implicit network access must set an address before upgrading. Retain the
first-task prompt and catalog steps from #5518. Update the CLI, security,
configuration, service, and container guidance to state the fallback for each
entry point and explain how to configure protected remote access.

## Security

A failed loopback bind must never select a wildcard. Authentication and proxy
trust retain their existing behavior. A wildcard inside a container does not
set the host-side publication address.
