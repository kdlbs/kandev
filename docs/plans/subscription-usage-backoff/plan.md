---
created: 2026-10-15
status: done
requirements:
  - REQ-COSTS-SUBSCRIPTION-USAGE-001
system_design:
  - ../../specs/costs/system-design/subscription-usage.md
legacy_specs: []
---

# Implementation Plan: Subscription usage 429 backoff and bounded stale reading

## Overview

Carry the residual upstream port of the subscription usage cache fix: honor a
provider `Retry-After` hint, apply an exponential backoff to a refused usage
read, and serve the last known success as explicitly stale for a bounded window
instead of returning a nil, error pair. One sequential work order covers the
provider clients, the shared cache, the `ProviderUsage.Stale` field, the TeamClaude no-capacity exception, and the
requirement/design pair that owns the contract.

## Scope

In scope: `apps/backend/internal/agent/usage/client_claude.go`,
`client_codex.go`, `cache.go`, the new `retry_after.go` status-error helper, and
the `ProviderUsage.Stale` marker in `types.go`, and the authoritative
no-capacity error in `client_teamclaude.go`.

Excluded: the fork-only `usage/errors.go`, Office overview rendering, locale
copies, web API/DTO plumbing, a new production time seam. The
upstream shared failure cache is reused, not replaced.

## Technical approach

The provider clients already coalesce fetches and keep a 15-second negative
window upstream. This change adds only the missing pieces: a typed status error
so the status and `Retry-After` survive to the cache, a backoff selector that
prefers the provider hint and otherwise grows the 429 window exponentially, and
a bounded stale branch in `lookup`/`GetOrFetchWithin` that serves a preserved
success marked `Stale` for at most 30 minutes.

## Work orders

- [x] [Task 01: Back off usage reads on 429 and serve a bounded stale reading](task-01-usage-429-backoff-stale.md)

## Changed-path coverage

The work order owns the `apps/backend/internal/agent/usage/` files above and
the permanent regression tests,
the extended `docs/specs/costs/requirements/subscription-usage.md` acceptance
criteria, and the new `docs/specs/costs/system-design/subscription-usage.md`
design. This manifest and its sole sibling order complete the coverage package.

## Verification

Permanent regression coverage lives beside the real usage modules in
`cache_internal_test.go`, `cache_concurrency_internal_test.go`,
`retry_after_internal_test.go`, `provider_backoff_internal_test.go`, and
`teamclaude_cache_internal_test.go`.
It covers parser boundaries, finite retry progression and resets, exact stale
limits, cancellation, credential-key sharing, per-key coalescing, HTTP status
propagation, and token-refresh compatibility. Time-sensitive cache cases use
`testing/synctest`; HTTP cases use only local servers and temporary credentials.

Hosted execution and full CI receipts are retained outside the source tree.
Deleted probes recorded in the original work order are historical author
reports and do not satisfy the permanent regression gate.

## Risks

- Serving a stale reading past its bound hides a real provider error; the
  30-minute cap keeps the reading actionable.
- Treating a nil negative-cache success as stale would panic or mask an error;
  only a real preserved reading is cloned.
- Returning a cached error as a nil success would silently clear a rate-limit
  signal; the error is surfaced whenever no bounded stale reading exists.
