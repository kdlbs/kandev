---
status: done
---

# Task 01: Persist plugin failure diagnostics

## Objective

Carry failure causes through the supervised runtime and persist them on plugin
records so operators can recover an errored plugin with a useful explanation.

## Scope

- `apps/backend/internal/plugins/store/`
- `apps/backend/internal/plugins/registry.go`
- `apps/backend/internal/plugins/service.go`
- `apps/backend/internal/plugins/service_sync.go`
- `apps/backend/internal/plugins/runtime/`
- Related backend tests.

## Requirements

- Add nullable `last_error` and `last_error_at` record fields with backward-
  compatible load/save behavior.
- Normalize stored diagnostics to one line and at most 2048 bytes; redact PATs,
  bearer tokens, labeled passwords/tokens/secrets/API keys, and the host home
  path before persistence. Do not store arbitrary plugin stdout or configuration.
- Propagate the triggering error through unhealthy runtime callbacks.
- Persist diagnostics for activation, boot/config restart, health failure,
  restart exhaustion, and missing install paths.
- Clear diagnostics on successful activation/recovery and retain them across an
  explicit disable.
- Preserve the existing `error -> active` transition and rollback guarantees.
- Never hold `Manager.mu` while waiting for `process.stop()`, so restart
  exhaustion and concurrent manual Enable cannot wait on each other.

## Test-first acceptance

- Store round-trips fields and loads records written before the fields existed.
- Newline and overlong errors are normalized deterministically.
- PAT, bearer, labeled-secret, and home-path values are absent from persisted
  diagnostics while safe host error context remains.
- Service tests cover failed enable, successful retry, failed retry replacement,
  health failure, recovery, and missing install path.
- Runtime tests prove ping/crash causes reach the manager callback and healthy
  recovery carries no error; a barrier-based concurrent Enable/restart-exhaustion
  test proves both operations complete.

## Dependencies

None. This is the API contract consumed by task 03.
