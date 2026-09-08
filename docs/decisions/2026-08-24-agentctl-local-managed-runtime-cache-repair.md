# ADR-2026-08-24-agentctl-local-managed-runtime-cache-repair: Run cache repair where npm runs

**Status:** accepted
**Date:** 2026-08-24
**Area:** backend, agentctl, protocol, security

## Context

Kandev launches managed npm runtimes on the host, in local Docker, and through
remote SSH. Each location has a separate npm environment and cache.

The first stale-metadata recovery ran cache repair in the Kandev backend. It
therefore supported only host-local standalone sessions. Docker and SSH
failures reached users as npm `ETARGET` errors.

Executor-specific shell repair would duplicate cache discovery, authentication,
quoting, and cleanup logic. Automatic rollback would also violate exact
version selection.

## Decision

The lifecycle manager owns recovery policy and trusted command reconstruction.
The session-scoped `agentctl` instance owns npm cache discovery and exact cache
repair.

The Kandev backend sends only the trusted exact package specification through
an authenticated agentctl request. Agentctl resolves the npm cache with the
configured agent environment. It removes only the deterministic `_npx` tree
for that specification.

The same contract applies to local PC, local Docker, and remote SSH runtimes.
Kandev preserves the configured registry and retries the same package version
once with online-preferred metadata.

## Consequences

Cache repair runs in the environment that owns the failed npm process. The
backend does not need Docker or SSH shell implementations for this repair.

Agentctl gains one authenticated maintenance action. Its process manager must
resolve npm configuration with the agent environment and retain process-tree
cleanup guarantees.

Future executors can adopt automatic recovery only when they use this
authenticated colocated contract and have executor-specific evidence.

## Alternatives considered

- Backend-host repair was rejected because it cannot reach Docker or SSH caches.
- Executor-specific shell commands were rejected because they duplicate security and environment logic.
- Global npm cache cleanup was rejected because it removes unrelated user data.
- Automatic version rollback was rejected because it changes the selected runtime without operator consent.
- Registry replacement was rejected because Kandev must preserve operator network policy.
