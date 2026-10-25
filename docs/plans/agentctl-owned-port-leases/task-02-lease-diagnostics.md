---
id: "02-lease-diagnostics"
title: "Carry lease diagnostics safely"
status: done
wave: 2
depends_on:
  - "01-owned-reservations"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AGENTCTL-INSTANCE-STOP-001
acceptance_criteria:
  - AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.9
  - AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.10
system_design:
  - ../../specs/platform/system-design/agentctl-instance-stop.md
---

# Task 02: Carry lease diagnostics safely

## Summary

Expose lease generation and listener activity through existing instance routes
and the control client. Prove that recovery treats these fields as diagnostics
and preserves live instances in mixed inventories.

## In scope

- Add `lease_generation` and `listener_active` to both `InstanceInfo` types.
- Track listener activity atomically from startup through `Serve` exit.
- Verify list and detail responses through the actual control HTTP boundary.
- Verify older response decoding and existing lifecycle identity guards.
- Run the package's final targeted tests, backend lint, and build.

## Out of scope

- New port-pool endpoints, aggregate counters, or frontend presentation.
- Database reclamation, runtime recovery policy changes, and new migrations.
- Changes to remote executor lifecycles or session retention.

## Acceptance

1. List and detail responses carry the current lease generation and listener
   state through the control client without dropping existing instance metadata.
2. Older responses remain usable. Zero generation, false activity, incomplete
   probes, and failed reads cannot independently authorize deletion or release.
3. A mixed inventory preserves every live non-terminal and waiting instance,
   even when terminal or stale records share its stored numeric port.

## TDD sequence

Add `TestInstanceLeaseDiagnostics` in the existing control instance list tests.
Exercise list and detail handlers with a real manager and HTTP listener.
Add `TestControlClientInstanceLeaseDiagnostics` for the client boundary.
Assert generation, listener activity, task/session IDs, workspace roots, and
provider session identity. Include older JSON responses without the new fields.
The new diagnostic assertions must fail before production changes.

Add `TestRecoveryLeaseDiagnosticsAreNonAuthoritative` to lifecycle recovery tests.
Build a mixed fixture with live, waiting, terminal, and stale records that share
a stored port. Vary generation and activity, then inject inventory/probe errors.
Assert no cleanup call targets the protected live instance. These safety cases
may already pass and serve as compatibility evidence, not manufactured red tests.

Exercise listener shutdown and unexpected `Serve` exit. Assert that the activity
field becomes false without independently releasing the lease. Preserve normal
teardown as the only path that returns this registered instance's reservation.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race -tags fts5 ./internal/agentctl/server/instance ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/task/repository/sqlite -count=1)
make -C apps/backend lint
make -C apps/backend build
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The six-package command preserves the issue's accepted compatibility coverage.
Record actual results on the implementation head. Do not reuse fork results.

## Files likely touched

- `apps/backend/internal/agentctl/server/instance/instance.go`
- `apps/backend/internal/agentctl/server/instance/manager.go`
- `apps/backend/internal/agentctl/server/instance/manager_ports_test.go`
- `apps/backend/internal/agentctl/server/api/control_instance_list_test.go`
- `apps/backend/internal/agent/runtime/agentctl/control.go`
- `apps/backend/internal/agent/runtime/agentctl/control_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_recovery_inventory_unknown_test.go`

The implementing session owns these files and this work order's result record.
The existing handlers already serialize `Instance.Info()` and need no new route.
Synchronize the parent plan after completion.

## Dependencies

Task 01 supplies stored leases and cleanup safety. Execute sequentially because
both tasks change instance representation and the manager.

## Risks

- Old servers encode missing activity as the client zero value.
- Generation identity is local to an allocator process, not durable across restarts.
- Listener inactivity can precede process cleanup and cannot prove capacity.
- Existing recovery tests must preserve their actual identity assumptions.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/agentctl-instance-stop.md), criteria .9 and .10.
- [System design](../../specs/platform/system-design/agentctl-instance-stop.md), diagnostic transport.
- [Plan](plan.md), compatibility matrix and exclusions.
- Existing control API, control-client, and lifecycle recovery fixtures.

## Results

Red/green evidence:

- `TestInstanceLeaseDiagnostics` first observed generation zero and an inactive listener because the fields were absent. It now verifies list and detail through a real manager, control HTTP server, and `ControlClient`.
- `TestControlClientInstanceLeaseDiagnostics` verifies field decoding alongside task/session IDs, workspace roots, and provider session identity. `TestGetInstance_DecodesEveryField` verifies older responses retain zero-value compatibility.
- `TestInstanceListenerActivityTracksServeLifetime` verifies unexpected `Serve` exit clears activity while preserving the lease until normal teardown.
- `TestRecoveryLeaseDiagnosticsAreNonAuthoritative` covers live, waiting, terminal, and stale records sharing a port, varied diagnostic values, and a failed control-server inventory probe. Existing unreadable database inventory coverage also passes.

Verification passed:

- `(cd apps/backend && go test -race -tags fts5 ./internal/agentctl/server/instance ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/task/repository/sqlite -count=1)`
- `make -C apps/backend lint`: 0 issues.
- `make -C apps/backend build`.
- `python3 scripts/list-docs.py validate`.
- `python3 scripts/lint-spec-files.py --all`.
- `git diff --check`.
