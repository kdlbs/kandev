# ADR-2026-09-07-host-utility-managed-runtime-recovery: Recover host capability probes before publishing failure

**Status:** accepted
**Date:** 2026-09-07
**Area:** backend, agentctl, protocol, security

## Context

Kandev already repairs strict npm `ETARGET` failures for managed runtime task
sessions. Host capability probes use the same exact package commands and the
same local npm cache, but they publish failure immediately. After a shipped
default changes, stale offline-preferred metadata can therefore mark every
profile for an otherwise valid agent as failed until another path refreshes the
cache.

The probe process captures the npm diagnostic inside agentctl. The host utility
manager receives only the generic ACP disconnect error, so it cannot safely
distinguish a recoverable top-level package miss from authentication, provider,
transitive dependency, or unrelated initialization failures.

## Decision

Host capability probes use the existing executor-local managed-runtime recovery
contract before publishing a failed capability status.

Agentctl classifies the bounded probe stderr against the exact top-level package
specification in the trusted probe command. It returns a stable failure code,
not raw stderr or executable command data. The host utility manager accepts that
code only for a registered managed npm agent, asks the same warm agentctl
instance to remove the exact deterministic execution tree, and retries the same
effective version once with online-preferred metadata. Repair uses the failed
probe's environment overrides and strip list. The warm instance excludes other
probe and prompt processes while it repairs and retries.

The host utility manager publishes only the final probe result. It does not
change persisted profile models, fallback models, modes, enabled state, or the
active runtime version during recovery.

## Consequences

New reviewed defaults can recover from stale local npm metadata during startup,
before task creation and without user action. The capability UI receives the
recovered catalogue instead of a transient failure when the registry can serve
the trusted exact version.

The probe response gains a bounded stable failure code. Agentctl must keep raw
stderr private and must require an exact match to the trusted top-level package.
Recovery adds at most one online probe and one narrowly scoped cache repair.

A registry outage, removed package, failed online retry, authentication error,
or unrelated ACP failure remains visible. Kandev does not silently roll back,
change registries, select another version, or publish capabilities from a
different runtime generation.

## Alternatives Considered

- Suppressing capability warnings was rejected because the runtime can remain unable to start.
- Keeping the last process-local catalogue after every failed probe was rejected because a changed runtime generation can make it incompatible.
- Returning raw probe stderr to the backend was rejected because it can contain paths, URLs, identifiers, or provider diagnostics.
- Running every normal probe with online-preferred metadata was rejected because healthy cached exact versions should not require registry access.
- Automatically rolling back to the prior version was rejected because it changes the reviewed effective runtime without operator consent.
