# ADR-2026-08-29-github-provider-rate-coordination: Coordinate GitHub Rate State by Provider Principal

**Status:** accepted
**Date:** 2026-08-29
**Area:** backend, protocol, persistence

## Context

Six workspaces using GitHub user ID 79718216 failed in coordinated polling
bursts while `GET /rate_limit` reported full core and GraphQL primary quota.
Existing trackers are credential/workspace local and convert rate-limit prose
into synthetic primary exhaustion, so Kandev neither recognizes secondary
throttling nor protects interactive work from its own background traffic.

## Decision

Keep credentials, authorization, and caches under the workspace/principal
ownership defined by ADR 0047, but move GitHub rate observation and request
admission to one process-wide coordinator keyed by the upstream quota identity.
Human tokens for the same host/login share a key regardless of workspace or
credential generation. GitHub App traffic shares by registration and
installation.

The coordinator preserves primary bucket snapshots separately from an observed
secondary retry window. It gives interactive requests priority and reserves the
last ten percent of known primary quota from background work. A failed
Kandev-managed operation returns safe rate-limit details with that operation.
The details identify the rate kind, resource, retry boundary, and retry source.

## Consequences

Background Kandev work can no longer independently spend the same upstream
identity's budget from multiple workspace trackers. Callers receive rate-limit
context from the failed operation, without a separate diagnostic request.
Successful operations do not carry quota details. External processes using the
same credential remain outside the coordinator.

Credential replacement for the same upstream identity does not discard an
active throttle. Process restart loses in-memory provider observations, while
Workflow Sync's persisted next-attempt time still prevents an immediate retry
storm.

## Alternatives Considered

- **Keep one tracker per workspace:** rejected because GitHub charges human
  primary and secondary budgets across tokens for the same user.
- **Poll `GET /rate_limit` before work:** rejected because it consumes
  secondary capacity and cannot report secondary throttling.
- **Use only a global subprocess semaphore:** rejected because direct HTTP
  clients bypass it and concurrency bounds do not reserve quota or honor
  provider retry windows.
- **Treat every rate-limit 403 as primary exhaustion:** rejected by the field
  fixture where primary quota remained 5000/5000 and the throttle cleared
  before the advertised primary reset.
