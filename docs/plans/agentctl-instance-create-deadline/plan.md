---
created: 2026-10-12
status: complete
requirements:
  - REQ-EXECUTORS-AGENTCTL-CREATE-DEADLINE-001
  - REQ-EXECUTORS-AGENTCTL-CREATE-DEADLINE-002
system_design:
  - ../../specs/executors/system-design/agentctl-instance-create-deadline.md
legacy_specs: []
---

# Implementation plan: Agentctl instance creation deadline and concurrency

## Overview

Concurrent session starts can fail while the agentctl controller is still
creating instances. Two sequential work orders address the two halves of the
cause:

1. Give `ControlClient.CreateInstance` its own 120-second client deadline. All
   other control calls keep the existing 30-second deadline.
2. Run each creation's startup outside `instance.Manager.mu`, so concurrent
   creations overlap instead of queueing. Ownership, abandonment cleanup, and
   shutdown keep their guarantees.

Each work order has its own commit, red check, and verification. No existing
specification defines control-call deadlines or creation concurrency. The new
capability pair records both contracts.

## Evidence and root cause

`NewControlClient` in `internal/agent/runtime/agentctl/control.go` sets one
30-second `http.Client.Timeout`, and `CreateInstance` uses that client. Callers
pass longer launch contexts, so the 30-second client timeout is the effective
bound.

The controller answers `POST /api/v1/instances` only after it starts the
instance's process manager and workspace trackers.
`instance.Manager.CreateInstance` holds `mu` across that startup, so
concurrent creations serialize. Its existing comment records creation latency
that climbed from milliseconds past 30 seconds under load. Lookup, listing,
and stop of other instances wait on the same lock.

## Scope

### In scope

- A dedicated 120-second deadline for `ControlClient.CreateInstance`.
- Admission, startup, and publication phases in `Manager.CreateInstance`, with
  startup outside `mu`.
- Shutdown coordination for creations that passed admission.
- Tests next to each changed package, and durable requirements and design.

### Out of scope

- Making a single creation's startup faster.
- Retry, backoff, or failure classification for instance creation.
- Launch-phase deadlines, executor-specific creation logic, and task
  environment preparation.
- A configurable deadline or concurrency limit.

## Technical approach

### Task 01: client deadline

Add `createInstanceHTTPTimeout = 120 * time.Second` in `control.go`. In
`CreateInstance`, copy `*c.httpClient`, set the copy's `Timeout`, and send the
request through the copy. The copy shares the configured transport, so
authentication and runtime binding are unchanged.

### Task 02: creation outside the manager lock

Split `Manager.CreateInstance` into admission, startup, and publication as the
[creation phases](../../specs/executors/system-design/agentctl-instance-create-deadline.md#creation-phases)
describe. Keep the provisional bundle as the ID and port owner during startup.
Start the HTTP server inside the publication critical section, so a refused
creation never starts one.

Add `activeCreations`, incremented under `mu` at admission. `Shutdown` waits on
it before draining `abandonWG`, as the
[shutdown design](../../specs/executors/system-design/agentctl-instance-create-deadline.md#shutdown)
describes. Store the process manager on the bundle under `cleanupMu`.

Move the instance override construction into a helper so `CreateInstance`
stays within the Go function-length limits. Carry every current
`CreateRequest` field into the overrides unchanged. Update the comments that
describe creation as serialized under `mu` or bounded by a 30-second client.

The [system design](../../specs/executors/system-design/agentctl-instance-create-deadline.md#known-limits)
records what these changes do not resolve.

## Tests

| Acceptance | Test evidence |
| --- | --- |
| 001.1 | New `TestCreateInstance_UsesLongerHTTPTimeout`: a stubbed transport observes a request deadline between 119 and 120 seconds |
| 001.2 | Existing `TestNewControlClient_BuildsBaseURLFromHostAndPort`, and the shared-timeout check in `TestCreateInstance_UsesLongerHTTPTimeout` |
| 001.3 | New `TestCreateInstance_CallerDeadlineStillApplies`: a short caller deadline ends creation with a wrapped `context.DeadlineExceeded` |
| 001.4 | New `TestCreateInstance_SendsControlAuthToken`: a token-authenticated client's creation request carries the bearer credential; new `TestCreateInstance_CopiesClientUnderRotation` under `-race`: concurrent credential rotation does not race the client copy |
| 002.1 | New `TestConcurrentCreationsOverlapStartup`: four creations reach `afterTrackerStart` together |
| 002.2 | New `TestCreateInstanceRejectsDuplicateIDDuringStartup`: a same-ID creation fails promptly while the first is held, and the first then succeeds |
| 002.3 | Existing `TestCreateInstanceAbandonsWhenCallerGaveUp`, `TestCreateInstanceAbandonsAfterTrackerStartup`, `TestAbandonRetainsLeaseOnCleanupFailure`, and `TestShutdownRetriesFailedAbandon`; new `TestCreateInstanceNotPublishedWhenCancelledBeforePublication` |
| 002.4 | New `TestShutdownWaitsForInProgressCreation`; existing `TestCreateInstanceRefusedAfterShutdown` |
| 002.5 | New `TestOtherInstanceOperationsDoNotWaitForStartup`: lookup, listing, and stop of a published instance complete while another creation is held |

Required red checks: `TestCreateInstance_UsesLongerHTTPTimeout` for task 01,
and `TestConcurrentCreationsOverlapStartup` for task 02. On the unchanged
manager, only one creation can reach the startup barrier. The duplicate-ID and
other-instance tests also fail on the unchanged manager, because they wait on
`mu`. The shutdown test is a preservation check and passes before and after.

## E2E tests

No browser flow changes, so no Playwright test applies. The client tests use
stubbed transports and `httptest` servers. The manager tests use the existing
in-package seam, temporary workspaces, and ephemeral ports. They do not start
an agent.

## Work orders

- [x] [Task 01: Give instance creation its own deadline](task-01-instance-create-deadline.md)
- [x] [Task 02: Create instances without holding the manager lock across startup](task-02-create-outside-manager-lock.md)

## Verification results

Task 01 implementation checks on 2026-10-12:

- The required red check, `TestCreateInstance_UsesLongerHTTPTimeout`, failed
  before the production change with `deadline had 30s remaining, want close to
  2m0s`. The caller-deadline and credential tests passed before and after the
  change.
- `go test ./internal/agent/runtime/agentctl/... -count=1`: passed.
- `go build` and `go vet` for `./internal/agent/runtime/agentctl/...`: passed.
- `python3 scripts/list-docs.py validate` and
  `python3 scripts/lint-spec-files.py --all`: passed.
- Local `validateCoverage` preflight on the changed files: `covered`.

Task 02 implementation checks on 2026-10-12:

- Before the production change, `TestConcurrentCreationsOverlapStartup`,
  `TestCreateInstanceRejectsDuplicateIDDuringStartup`, and
  `TestOtherInstanceOperationsDoNotWaitForStartup` failed because each waited
  on `mu`. `TestShutdownWaitsForInProgressCreation` passed before and after.
- After the change, the four new tests passed. `go test -race` over the
  creation, abandonment, shutdown, and stop tests passed with no race reports.
- `go test ./internal/agentctl/server/instance -count=1` passed except
  `TestBuildMcpServerConfigs_DropsMissingStdioCommand`, which needs a POSIX
  shell script to be executable and fails on Windows hosts independently of
  this change.
- `go build ./internal/agentctl/...`, `go build ./cmd/agentctl`, and
  `go vet ./internal/agentctl/server/instance`: passed.
- Local `validateCoverage` preflight on the changed files: `covered`.

PR review follow-up on 2026-10-12:

- `ControlClient.CreateInstance` copies the shared client under `c.mu`.
  `TestCreateInstance_CopiesClientUnderRotation` reported a data race under
  `-race` without the lock and passes with it.
- `publishInstance` re-checks the caller's context under `mu`.
  `TestCreateInstanceNotPublishedWhenCancelledBeforePublication` failed before
  the fix with a published instance and a nil error, and passes after it.
- The concurrency tests bound every completion wait and their cleanup
  shutdown, and their creations use the test context.
- The design lists environment validation under admission, and the
  requirement and design documents use their accepted statuses.
- `go test ./internal/agent/runtime/agentctl/... -count=1`,
  `go test ./internal/agentctl/server/instance -count=1` (apart from the
  Windows-only failure noted above), the race-enabled creation, abandonment,
  shutdown, and stop tests, `go vet` for both packages, and the
  documentation preflight passed.

The work orders for [task 01](task-01-instance-create-deadline.md#results) and
[task 02](task-02-create-outside-manager-lock.md#results) record the details.

## Risks

- A per-call client that replaces the transport would drop authentication.
  Copy the client so the configured transport is shared.
- Raising the shared timeout would lengthen failure detection for every control
  call. Change only `CreateInstance`.
- A creation published after `Shutdown` snapshots the maps would never stop.
  Admission registers on `activeCreations` under `mu`, and `Shutdown` waits on
  it first.
- A server started before a refused publication would keep serving on an
  unpublished instance. Start the server only during publication.
- Overlapping startups run more tracker and git work at once. Startup work per
  creation is unchanged, and `agentctl_create_ready_ms` still reports full
  creation time.

## Documentation impact

No user or operator setting changes. Public documentation is not required. The
`internal/agentctl/AGENTS.md` port-lease section remains accurate.
