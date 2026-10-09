---
created: 2026-09-27
status: done
requirements:
  - REQ-PLATFORM-AGENTCTL-INSTANCE-STOP-001
system_design:
  - ../../specs/platform/system-design/agentctl-instance-stop.md
legacy_specs: []
---

# Implementation Plan: Owned agentctl port leases

## Overview

Make standalone instance reservations safe across cancellation and port reuse.
Implement ownership and cleanup first, then diagnostic transport and recovery
compatibility. Execute both work orders sequentially.

Source issue: [#3961](https://github.com/kdlbs/kandev/issues/3961).
The authenticated user, `carlosflorencio`, received the assignment on 2026-09-27.
Implementation is complete in this checkout. Both work orders are done and
their required race tests, backend lint/build, and documentation checks passed.

## Evidence and root cause

Investigation used repository head `f24a785cd3012bbf14c06134ed480e82fd7613a3`.
`PortAllocator.Allocate` searches by free port without consulting an owner index.
`Release` deletes by number. `MarkUnavailable` blocks and deletes by number.
These operations cannot distinguish a successor reservation from stale cleanup.

A temporary test drove the current allocator with one-port and two-port pools:

| Reproduction | Observed result |
| --- | --- |
| Allocate twice for one owner | Returned 41001, then 41002. |
| Release A, allocate B, release A again | A third owner obtained B's reserved port. |
| Release A, allocate B, mark A unavailable | B's reservation disappeared. |

All three desired-behavior assertions failed on the current implementation.
The temporary test was removed after execution. At that investigation stage,
production source had not changed.
The allocator reproduction establishes the missing safety boundary. It does not
claim an observed live-manager callback interleaving.

The current cancellation path already passes its existing regression tests.
`abandonPartialInstance` closes the listener and releases the port before calling
`StopForTeardown`. It logs teardown failure but discards the provisional owner.
That order contradicts the requested retention rule. An admission-drain failure
can return before trackers stop, so releasing first is not safe.

## Preserved implementation

The remote fork branch still points to
`7577be0e8943af4e8d6ef0846496cc30eb75491b`. Its implementation commit is
`b2aa655e330821df14fc0f6809843fafe83856b6`.
Both commits were fetched and inspected without applying them.

Reuse the lease model, owner index, generation comparisons, deferred cleanup,
and duplicate-owner regression as implementation references. Preserve the
contributor attribution in any later delivery.

Do not apply the fork unchanged. Its abandon helper still releases before
process teardown. This package records the required correction: retain failed
provisional bundles and release only after successful cleanup.
The fork also adds a pool diagnostic endpoint. That endpoint is unnecessary for
this repair and is excluded. The issue's reported full verification is upstream
evidence, not verification of this checkout or this proposed correction.

## Requirement conformance and ownership

Platform owns the existing instance-manager lifecycle contract. Criteria .1
through .4 already cover registered stop behavior. The added criteria .5 through
.10 cover reservation identity, provisional cleanup, and diagnostic transport.
Executor specifications retain port probing and runtime adoption ownership.

The completed `startup-observability-cleanup` Task 03 remains unchanged. Its
criteria .1 through .4 and recorded results remain historical delivery evidence.
This package extends that contract and reruns its relevant regressions.
No new ADR is needed: the paired design preserves the local ownership rationale.

## Scope

### In scope

- Immutable leases and atomic port/owner indexes.
- Same-owner allocation, generation fencing, and occupied-port retry.
- Deferred provisional cleanup, failure retention, and shutdown retry.
- Registered teardown using its exact lease.
- Additive instance diagnostics through existing control routes and client.
- Mixed-inventory and older-server compatibility coverage.

### Out of scope

- Increasing the 41001–41100 range or changing parked-session retention.
- Database-driven reclamation, schema changes, and new recovery scanners.
- Docker, SSH, Kubernetes, Sprites, or UI lifecycle changes.
- Worktree/commit recovery and published runtime-exit signals from issue #3962.
- The fork's pool endpoint and aggregate metrics.
- Publishing a PR or endorsing the fork's exact head without its required review.

## Technical approach

`port_allocator.go` returns value leases and compares complete identities.
Migrate all manager and test callers in the same work order so the package
compiles. Store leases on registered instances and provisional bundles.

`manager.go` arms cleanup immediately after bind. A private bundle map prevents
same-owner rebinding during asynchronous cleanup. Failed bundles remain reserved.
Shutdown drains active cleanup, retries retained bundles once, and returns errors
without discarding ownership. No retry runs while holding the manager mutex.

`instance.go` and `runtime/agentctl/control.go` carry `lease_generation` and
`listener_active`. Existing list/detail handlers serialize those fields.
Keep recovery policy unchanged and exercise its existing identity guards.

| Runtime/transport | Identity | Intended behavior | Evidence/fallback |
| --- | --- | --- | --- |
| Standalone allocator | Owner, port, generation | Exact lease mutations | Allocator and manager tests |
| Authenticated control HTTP | Instance ID plus lease diagnostics | Preserve fields through client decoding | Control API/client integration tests |
| Older control server | Missing generation/activity | Existing operations remain compatible | Missing fields never authorize release |
| Remote executors | Existing runtime identity | No lifecycle policy change | Existing focused lifecycle regressions |

## Tests

Paths below are relative to `apps/backend/internal/`.
New test names describe intended permanent regressions.

| Criteria | File and test |
| --- | --- |
| .5, .6 | `agentctl/server/instance/port_allocator_test.go`: `TestPortAllocatorReusesOwnerReservation`, `TestPortAllocatorStaleReleaseAfterReuse` |
| .5, .7 | Same file: `TestPortAllocatorStaleMarkUnavailable`, `TestPortAllocatorGenerationAndConcurrentExhaustion` |
| .8 | `agentctl/server/instance/manager_abandon_test.go`: existing `TestCreateInstanceAbandonsAfterTrackerStartup`; new `TestAbandonRetainsLeaseOnCleanupFailure`, `TestAbandonRetrySameOwnerDoesNotRebind` |
| .8 | Same file: `TestShutdownRetriesFailedAbandon`, `TestAbandonListenerCloseFailureRetainsLease` |
| .1–.4 | `agentctl/server/instance/manager_shutdown_test.go` and existing control stop tests: duplicate stop, pointer reuse, cleanup failure |
| .9 | `agentctl/server/api/control_instance_list_test.go`: `TestInstanceLeaseDiagnostics`; `agent/runtime/agentctl/control_test.go`: `TestControlClientInstanceLeaseDiagnostics` |
| .10 | `agent/runtime/lifecycle/manager_recovery_inventory_unknown_test.go`: `TestRecoveryLeaseDiagnosticsAreNonAuthoritative` |

Use deterministic barriers and teardown fakes. Test stale releases for both a
new owner and a new generation of the same owner. Test every mixed inventory:
live non-terminal, `WAITING_FOR_INPUT`, terminal, and stale records sharing a
stored port. Unknown reads and missing diagnostics must not trigger deletion.

## E2E tests

There is no rendered UI change. End-to-end evidence uses the actual control
handler and HTTP client for list/detail serialization, plus lifecycle recovery
fixtures. Playwright cannot improve evidence for this internal allocator boundary.

## Work orders

- [x] [Task 01: Own reservations through cleanup](task-01-owned-reservations.md)
- [x] [Task 02: Carry lease diagnostics safely](task-02-lease-diagnostics.md)

## Verification results

Investigation completed:

- Temporary allocator reproduction: three assertions failed for the expected reasons.
- `(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/instance -run 'Test(CreateInstanceAbandonsAfterTrackerStartup|CreateInstanceRefusedAfterShutdown|Abandon)' -count=1)` passed.
- Fork head and implementation inspected. Fork checks were not rerun.

Implementation verification passed:

- `(cd apps/backend && go test -race -tags fts5 ./internal/agentctl/server/instance ./internal/agentctl/server/api -count=1)`
- `(cd apps/backend && go test -race -tags fts5 ./internal/agentctl/server/instance ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/task/repository/sqlite -count=1)`
- `make -C apps/backend lint`: passed with 0 issues.
- `make -C apps/backend build`: passed.
- `python3 scripts/list-docs.py validate`: passed, 316 decisions and 1207 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.

Task 01's allocator regressions first failed on the original owner and
port-only behavior, then passed with exact lease matching. Task 02's API
diagnostics regression first observed zero generation and inactive listener
because the fields were absent, then passed through list and detail client
calls. The lifecycle test confirms diagnostic values do not change recovery
decisions when sessions share a numeric port or when inventory reads fail.
Documentation validation completed:

- `python3 scripts/list-docs.py validate`: passed, 316 decisions and 1207 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: passed for both work orders
  with the planned allocator source path included as a coverage trigger.
- Relative document links: passed.
- `git diff --check -- docs/specs/platform docs/plans/agentctl-owned-port-leases`: passed.
- `git status --short`: only the two specification edits and three new package files.
- GitHub assignment readback: issue remains open and assigned to `carlosflorencio`.

## Risks

- Duplicate-owner allocation must not let the manager bind or abandon a second
  bundle using the first bundle's lease.
- Failed cleanup deliberately retains capacity. Shutdown retry must retain the
  bundle until all resources stop, and must report persistent failure.
- Tracker stop methods have their own waits. A cleanup context does not make
  shutdown a strict wall-clock deadline.
- Generation zero and inactive listeners are diagnostic values, not proof of death.
- Fork code predates current changes. Adapt the specific logic instead of
  replacing current instance metadata or recovery behavior wholesale.

## Documentation impact

Internal requirements, system design, plan, work orders, and agentctl guidance
change. Public docs remain unchanged. No public command or setting changes.
