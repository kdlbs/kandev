---
created: 2026-09-26
updated: 2026-09-26
status: done
requirements:
  - REQ-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001
system_design:
  - ../../specs/platform/system-design/system-info-query-cache.md
  - ../../specs/platform/system-design/backend-restart-page-recovery.md
legacy_specs: []
---

# Delivery Plan: SystemInfo Query Cache Pilot

## Outcome

Move the System > About `SystemInfo` snapshot and request state to one identity-scoped TanStack Query cache. Preserve the existing About presentation, authenticated endpoint, page boot ID, backend restart guard, and every other System state owner. This refactor adds no product behavior or boot-payload fields.

## Contracts

- The [SystemInfo query design](../../specs/platform/system-design/system-info-query-cache.md) defines cache ownership and identity. The [backend restart design](../../specs/platform/system-design/backend-restart-page-recovery.md) remains authoritative for process-generation detection.
- The existing restart requirement's ACs `.1` and `.2` remain regression invariants: the page uses its existing boot ID and the reconnect guard keeps its separate no-store probe.
- The frontend query is lazy, keyed by canonical full API base URL, page boot ID, auth mode, authenticated state, and user ID. Workspace does not scope this process-wide resource.
- Build and process identity fields are immutable for one backend process. Explicit refetch remains available. Other System resources and the restart, restart-flow, and self-update probes retain their existing owners.

## Work orders

- [x] [Task 01: Move SystemInfo to the scoped Query cache](task-01-system-info-cache.md) — done

## Verification results

- Focused SystemInfo, System slice, restart guard, restart flow, self-update, and System API tests pass (66 tests across 6 files).
- Web typecheck and full lint pass. `pnpm install --frozen-lockfile` passes from `apps/`.
- Architecture lint, spec validation, harness validation, the i18n ratchet, and `git diff --check` pass.
- No mobile E2E was needed: this data-ownership refactor leaves About layout, copy, controls, and responsive behavior unchanged.
