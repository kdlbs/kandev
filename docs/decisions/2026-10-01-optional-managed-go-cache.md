# ADR-2026-10-01-optional-managed-go-cache: Optional launch cache and strict maintenance safety

**Status:** accepted and implemented
**Date:** 2026-10-01
**Area:** backend

## Context

The managed Go cache is an install-wide opt-in performance feature under ADR 0045.
Its provider rejects symlink paths because maintenance can move and delete cache data.
Lifecycle startup currently treats the same rejection as fatal, including during Start fresh.
An adopted cache can become unsafe after adoption, without any task or conversation change.

## Decision

Keep the provider's path and ownership validation strict.
At the lifecycle boundary, omit the managed override after cache-specific preparation failures.
Preserve ordinary environment resolution and emit a fixed backend warning.
Cancellation and deadline errors remain startup errors.

Select the cache once for each new execution.
Promotion retains an existing execution's environment snapshot.
Recovered executions discard old managed-cache authority and select from current settings.
Keep request environment input separate from the managed override until final composition.

A launch fallback grants no maintenance ownership and changes no saved setting.
Adoption, cleanup, quarantine, restore, and deletion retain their existing safety checks.
The owning contract is [Storage maintenance](../specs/system-page/requirements/storage-maintenance.md), requirement `REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-006`.
The [design](../specs/system-page/system-design/storage-maintenance-01.md#managed-cache-launch-integration) records integration boundaries.

## Consequences

Otherwise valid tasks can start despite an unusable optional cache.
Fallback can reduce cache reuse and leave an above-threshold cache untouched until an operator repairs it.
An independently configured or inherited cache can still cause a later Go command to fail.
A fixed warning omits the detailed provider cause. Existing storage diagnostics retain their separate role.
Live executions do not change cache paths when settings or filesystem state change.

## Alternatives Considered

- **Keep launch failure:** an optional performance feature blocks otherwise valid work and repeated fresh starts.
- **Resolve symlinks in the provider:** this expands destructive maintenance authority beyond the explicitly adopted path.
- **Repair the host cache:** this removes one incident condition but leaves the product failure boundary unchanged.
- **Disable the saved option automatically:** this changes operator intent and hides the maintenance problem.
- **Select a second managed cache:** this adds ownership and cleanup policy without a requested contract.

This decision extends [ADR 0045](0045-install-wide-storage-maintenance.md) without replacing its maintenance rules.
