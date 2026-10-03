# ADR-2026-10-02-direct-go-cache-reclamation: Direct Go-cache deletion with a busy policy

**Status:** accepted; implemented
**Date:** 2026-10-02
**Area:** backend

## Context

Continuous task activity can prevent global idle maintenance indefinitely.
Quarantine retains reproducible build data while replacement data grows.
The user prefers occasional failed builds over disk exhaustion and explicitly
requests a setting that permits Go-cache cleanup during active tasks.

## Decision

Keep one shared cache path and use consistent repository `-trimpath` defaults.
Delete eligible build-cache data directly without quarantine.
Add `go_cache.allow_cleanup_while_busy`, default false.
When enabled, Go cleanup bypasses activity and quiet-period admission.
The existing schedule, threshold, ownership, and permission checks still apply.
The option does not permit busy cleanup of other resources.

Busy deletion can fail active builds. Kandev does not stop tasks or promise
automatic retries. Agents or users can retry against the same cache path.
The operator accepts this risk by saving the visibly explained setting.

The [design](../specs/system-page/system-design/go-cache-reclamation.md)
contains no generations, consumer leases, or execution-lifetime tracking.
This replaces the earlier unimplemented generation proposal in this package.

This policy supersedes ADR 0045 only for new Go-cache quarantine and opted-in
busy admission. Preserve workspace quarantine and historical retention.
Preserve the [optional-cache fallback](2026-10-01-optional-managed-go-cache.md).

## Consequences

Cleanup can make progress on an always-busy installation without tracking consumers.
Some builds can fail and subsequent compilation can be slower.
Concurrent writers can refill the cache before cleanup finishes.
The threshold remains a trigger, not a hard quota or disk-full guarantee.
No new cleanup operation creates restorable Go-cache data.

## Alternatives considered

- Global idle only cannot satisfy the user's continuously busy installation.
- Generations and leases add lifecycle and recovery complexity and can retain data indefinitely.
- Unconditional busy cleanup imposes build failures on users who did not select that tradeoff.
- Quarantine delays actual disk recovery for reproducible data.
- Hard quotas can fail every writer at the limit without reclaiming data.
- External cache services add deployment and protocol dependencies outside this scope.
