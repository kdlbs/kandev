---
status: current
system: system-page
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-006
created: 2026-10-02
updated: 2026-10-02
owners:
  - cfl
---

# Managed Go-Cache Launch Fallback System Design

## Purpose and boundaries

This design defines how an unavailable optional managed Go cache affects task launch and recovery.
Storage owns the persisted setting, provider, and maintenance safety contract. Lifecycle owns the
launch fallback. Task recovery uses the lifecycle behavior without adding separate cache policy.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-006` | [Cache decision and recovery](#cache-decision-and-recovery) |

## Components and responsibilities

- `gocache.Provider` validates managed paths and retains strict settings, ownership, cleanup, and quarantine rules.
- `lifecycle.Manager` decides whether a cache preparation failure omits the optional override or prevents startup.
- `Service.RecoverSession` and the lifecycle adapter use the same preparation boundary for Resume, Start fresh, and workspace recovery.

## Cache decision and recovery

For a new host-local execution, `launchInternal` looks up an existing session before selecting a cache.
If creation is required, `Manager.prepareManagedGoCacheEnvironment` clears stale managed-only state and asks the provider once.
It does not call the provider for remote or container executors.

An empty provider result means management is disabled. A successful absolute path becomes the execution's managed `GOCACHE` at final environment composition. `LaunchRequest.Env` remains input during selection, so an independently configured `GOCACHE` is preserved when preparation falls back.

Other provider errors and nonempty relative paths fall back without a managed override. Caller cancellation, caller deadline expiry, and wrapped provider cancellation/deadline errors remain startup errors. Non-cache launch failures retain their existing behavior.

Promotion and coalesced requests reuse the established execution decision without probing again. A recovered execution removes copied `managed_go_cache_path` metadata before applying the current successful result. A later execution evaluates current settings again. Recovery admission and conversation-token behavior remain unchanged.

## Persistence and maintenance safety

Fallback changes no saved setting and grants no maintenance ownership. New execution metadata contains a managed path only after current successful preparation. The provider and shared path validator continue rejecting unsafe paths during adoption, cleanup, rotation, quarantine, restore, and deletion.

## Observability

Each failed preparation decision emits one bounded warning with the exact message:

> managed Go cache preparation skipped; continuing without managed override

The structured `reason` is `preparation_failed` or `invalid_output`. When present, `task_id` and `session_id` fields identify the affected launch. The warning does not include cache paths, raw provider output, credentials, or environment values. Disabled management and cancellation emit no fallback warning.

## Verification

The lifecycle regression suite covers cache selection, environment composition, promotion, coalescing, recovery metadata, cancellation, warning fields, and provider call counts. Backend integration tests cover initial launch, Resume, Start fresh, non-cache failure, and cancellation through real recovery dispatch. Provider and quarantine tests retain sentinel and symlink safety controls. Exact commands and outcomes are recorded in the [implementation plan](../../../plans/managed-go-cache-launch-fallback/plan.md) and its work orders.

## Related decisions

- [Optional managed Go cache and strict maintenance safety](../../../decisions/2026-10-01-optional-managed-go-cache.md)
