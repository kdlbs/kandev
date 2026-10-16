---
status: draft
system: costs
requirements:
  - REQ-COSTS-SUBSCRIPTION-USAGE-001
---

# Subscription usage cache system design

## Purpose and boundaries

The costs system owns how subscription utilization readings are fetched, cached,
paced, and degraded. This design covers the `internal/agent/usage` provider
clients and the shared `UsageCache` in front of them.

Adjacent contracts this design uses but does not own:

- Agent profile identity and credential paths belong to the
  [agent system](../../agents/README.md).
- Office cost display and scheduler deferral consume a reading but own their own
  policy; the reading's freshness contract is defined here.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-COSTS-SUBSCRIPTION-USAGE-001` | Data and contracts, control flow, failure and recovery |

## Components and responsibilities

- `ClaudeUsageClient` and `CodexUsageClient` fetch one provider reading. On a
  non-OK response they return a typed status failure carrying the HTTP status
  and the provider's `Retry-After` hint rather than an opaque message string.
- `UsageCache` coalesces concurrent fetches per key, keeps the last success and
  the last failure independently, and applies the backoff and stale policy.

## Data and contracts

- `ProviderUsage.Stale` marks a preserved success served while a provider
  refuses a refresh; `FetchedAt` still records when the reading was taken.
- A provider status failure exposes `StatusCode()` and `RetryAfter()`, so the
  cache follows the provider's own pacing without parsing message strings.
- A `Retry-After` hint is accepted as delta-seconds or an HTTP-date and clamped
  to 15 minutes. Positive decimal seconds above the integer parser's range also
  reach this cap; malformed and negative values remain unreadable hints.
- A nil reading from a fetch function is stored as a negative-cache entry and
  reused while it is within the caller's age bound, but it is never cloned as a
  stale success.
- While a key is in backoff, a real (non-nil) preserved success younger than 30
  minutes is served as `Stale` even beyond the caller's age bound; otherwise
  `GetOrFetchWithin` serves a cached value only within the caller's age bound.
  The backoff failure policy is checked before ordinary freshness, so a longer
  caller age bound cannot bypass the stale limit or hide a cached failure.

## Control flow

1. A caller asks the cache for a key within a caller-chosen age bound.
2. A fresh success or an in-window negative entry is returned immediately.
3. Otherwise a per-key lock serializes one fetch; concurrent misses coalesce.
4. On a non-OK response the client returns the typed status failure.
5. The cache records the failure beside any preserved success and holds the key
   in backoff: a provider hint wins, otherwise an exponential 429 window
   (15-second base, 15-minute cap), otherwise the 15-second base window.

## Failure and recovery

- A preserved success younger than 30 minutes is served as `Stale`, so a caller
  keeps a real reading instead of an unactionable error.
- A preserved success at least 30 minutes old, or no usable (non-nil) preserved
  success, reports the provider error.
- A failed refresh returns the provider error and never a nil success, except
  when a real preserved success younger than 30 minutes is available: then that
  reading is served as `Stale`, including when the entry's only other success was
  an expired nil negative-cache entry.
- A caller cancellation does not poison the cache; that failure is not recorded.

## Persistence

The cache is in-memory only; a backend restart causes one fresh fetch per key.
No usage reading is persisted to SQLite.

## Security

Usage requests carry the agent's own subscription credential, read from the
profile-resolved credentials file. The cache stores only the reading, never the
credential or token.

## Observability

Backoff, stale-service, and reported-error outcomes are visible from the cache
entry state. No new metric or log label carries a credential, task, or session
identifier.

## Related decisions

None.
