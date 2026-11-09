---
created: 2026-11-04
status: implemented
requirements:
  - REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-001
system_design:
  - ../../specs/platform/system-design/runtime-failure-attribution.md
  - ../../specs/platform/system-design/runtime-diagnostic-signal.md
legacy_specs: []
---

# Runtime stall diagnostics

## Overview

Identify database phases and preserve safe failure causes with two small changes.
Task 01 adds probe and durability-run diagnostics.
Task 02 adds classified agentctl context through the existing forwarding path.
Both work orders are complete. This package does not claim to fix the database stall.

Platform owns shared persistence and runtime diagnostic contracts.
Existing delivery, provider, and repository owners keep their authority.

## Evidence and confirmed scope

The user requested improvements after the 4 November 2026 log audit.
The user requested simple solutions and accepted the first diagnostic patch.
Authentication is intentional and excluded.
There is no material unresolved choice for this diagnostic increment.

At 20:16:03 UTC, the required-store probe failed at `writer_ping` after two seconds.
The single writer was occupied while reader connections were available.
Persistence recovered at 20:16:16 UTC after 15 API responses with HTTP 503.
Delivery acknowledgments and retention also timed out.
These facts do not identify the transaction that held the writer.

The current health call includes connection acquisition inside `PingContext`.
Its label therefore cannot distinguish acquisition failure from an acquired-connection failure.
The delivery metrics measure each caller's barrier wait, not the actual shared run's phases.
Those are confirmed diagnostic gaps, not proof that the barrier caused the stall.

The same audit found `failed to configure agent`, Git status errors, and an unsuccessful probe.
Their generic records omitted the cause after forwarding.
Those producers emit raw structured errors that the established allowlist correctly excludes.
The repair supplies typed, bounded context at the producers.

## Existing work

The completed [writer investigation](../database-writer-contention/plan.md)
and [message-update optimization](../message-update-writer-occupancy/plan.md)
remain historical evidence with unresolved incident attribution.
Reuse their controls without repeating their load experiments or changing their completed status.
The existing [safe child diagnostics](../startup-shutdown-diagnostic-integrity/task-02-safe-child-diagnostics.md)
remain the forwarding mechanism.
The agent-owned [probe package](../agentctl-probe-log-visibility/plan.md)
continues to own sanitized stderr and existing probe codes.

## Scope

### In scope

- Explicit connection-acquisition and acquired-ping stages under the current probe deadline.
- One bounded warning for each failed or slow durability run.
- Safe operation, error class, and available trusted identity on the observed agentctl failures.
- Regression tests for timing boundaries, cancellation, forwarding, and unchanged responses.

### Out of scope

- New drivers, transaction frameworks, schedulers, queues, metric registries, and settings.
- Additional writer connections, changed durability, probe grace periods, or readiness policy.
- Changes to retention budgets, payload storage, backups, or checkout repair.
- Plugin changes, authentication changes, UI work, and live load experiments.
- Claims that every writer owner is observable after this increment.

## Technical approach

`Health.pingObserved` acquires and releases each existing connection explicitly.
It keeps the shared context and records the stage that actually fails.
No successful probe adds a new failure-stage warning.

`DurabilityBarrier.run` owns a local diagnostic record for the shared operation.
Its existing transaction helper records begin, write, and commit durations.
`NewDurabilityBarrier` receives the repository logger through one added constructor argument.
The signature update affects the constructor, its repository caller, and its direct SQLite test.
The record emits after release and outside the barrier mutex.
The one-second slow threshold is fixed for production and exposed only as a package-private test seam.
Existing per-caller metrics, cancellation ownership, and connection-discard behavior remain unchanged.

Agentctl handlers add fields at their current failure sites.
Configuration gets a typed refusal with unchanged text.
Git reuses the existing filesystem error classifier.
Probe summaries map only existing typed codes.
The launcher extends its current closed allowlists by two operations and five classes.
No general error-classification service is necessary.

| Boundary | Coverage | Fallback and preservation |
| --- | --- | --- |
| SQLite required-store probe | Acquisition, acquired ping, failed stage | Existing failure and maintenance policy |
| SQLite durability run | Actual run phases after coalescing | Other writer owners remain unknown |
| PostgreSQL | Same bounded probe decisions and barrier no-op | No SQLite durability work or admission-policy change |
| Managed agentctl JSON | Valid operation, class, and trusted UUIDs | Unknown cause stays `unknown` |
| One-shot probe without session identity | Operation and existing typed code | No invented session or instance |
| Unsupported child fields | Existing typed validation | Drop the field and retain severity |

No ADR is required because these changes extend the existing diagnostic boundary.
They do not select a new persistence or recovery policy.
The shared diagnostic-signal documents cover other work and retain their existing lifecycle status.

## Tests

| Criterion | Evidence |
| --- | --- |
| AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.1 | Held writer/reader acquisition and acquired-ping failure controls |
| AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.8 | Shared-run timing, omitted unreached stages, coalescing, and canceled-caller tests |
| AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.9 | Real configuration, Git, and probe producer tests with unchanged responses |
| AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.4 and .5 | Typed forwarding, info threshold, invalid-value, and privacy controls |

The work orders name exact test commands.
No browser interaction changes, so this package needs no browser test.

## Work orders

- [x] [Task 01: Attribute database phases](task-01-attribute-database-phases.md)
- [x] [Task 02: Classify agentctl failures](task-02-classify-agentctl-failures.md)

The tasks executed sequentially in one implementation session after the design-package handoff.
No subagents were used.

## Implementation results

Task 01 separates probe acquisition from an acquired ping and adds one bounded
durability-run record. Task 02 adds bounded operation, class, and trusted
identity context to the existing agentctl failure records.

Verification on 4 November 2026:

- `go test -race ./internal/db ./internal/persistence/requiredstores` passed.
- `go test -race ./internal/task/repository/sqlite -run 'Durability|Barrier'` passed; the full package passed afterwards.
- `go test -race ./internal/agentctl/server/api` and `./internal/agentctl/server/utility` passed.
- `go test -race ./internal/agent/runtime/agentctl/launcher` passed.
- `go build ./...` passed for the backend.
- `bash scripts/lint-go-changed` reported 0 issues for every changed Go file.
- `node --test scripts/validate-public-docs.test.mjs` passed 63 tests, and `node scripts/validate-public-docs.mjs` validated 55 pages.
- `git diff --check` passed.

No database policy, readiness, retry, durability, response, or provider
redaction behavior changed.

Review remediation on 4 November 2026:

- The barrier now delivers the actual run result to the initiating caller
  before it emits the run warning. A blocked log sink can no longer replace the
  run outcome with the caller's deadline.
- The coalescing test holds every caller in the registration hook until all
  have registered, so no leader can take its covered snapshot early.
- The cancellation test forces the canceled caller to own the run, coordinates
  release with channels, and waits for both actual-run records.
- Tests that read run warnings wait for the owning goroutine to emit them.
- `go test -race ./internal/db -count=1` and repeated runs of the ordering
  tests passed. The new result-before-log regression fails against the previous
  ordering.

Review remediation for PR #5054 on 4 November 2026:

- The SQL guard exemption moved from `commit` to `commitAcquired` after the
  barrier phase split, so `go run ./cmd/sqlguard ./internal` passes again.
- Both Git status paths now share one failure record. A changing-evidence
  failure keeps its warning and adds `operation=git_inspection`,
  `error_class=evidence_changed`, and trusted identity; the multi-repository
  fan-out logs the generic record it previously omitted.
- Probe failure records include the instance's trusted identity when the API
  server supplies it, and the handler executor fields are injectable so the
  execution-error branch has handler-level coverage.
- The barrier exposes its slow-run threshold as a package-private test seam, so
  the slow-success and coalescing tests no longer sleep. Test gates release
  through `sync.Once` cleanup.
- Plan and operations wording now distinguishes the current stage from a failed
  stage and the failure-stage warning from the existing state-transition log.
- Verification passed: backend build; race tests for `internal/db`,
  `internal/persistence/requiredstores`, `internal/agentctl/server/{api,process,utility}`,
  `internal/agent/runtime/agentctl/launcher`, and the sqlite durability scope;
  `go run ./cmd/sqlguard ./internal`; `bash scripts/lint-go-changed`;
  `node --test scripts/validate-public-docs.test.mjs`; `node scripts/validate-public-docs.mjs`;
  `python3 scripts/lint-spec-files.py --all`; `python3 scripts/list-docs.py validate`;
  and `git diff --check`.

## Verification results

Design validation passed on 4 November 2026:

- `python3 scripts/list-docs.py validate`: 431 decisions and 1,915 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- PR-documentation preflight: the actual documentation diff is exempt.
  Both work orders and their linked contracts pass coverage with the proposed runtime paths.
- `git diff --check -- docs/specs docs/plans/runtime-stall-diagnostics`: passed.
- The added prose passed the simple-English sentence and vocabulary review.

Implementation checks passed in the results above.

## Risks

- Acquisition timing includes connection initialization and cannot prove pure lock or queue time.
- A canceled caller must not finish or cancel a shared durability run.
- Logging under a writer lease or barrier mutex can prolong the failure.
- Missing identity and unsupported errors must remain explicit unknown evidence.
- A changing-evidence Git status failure keeps its own warning class and does not become a generic status failure.

## Public documentation

Task 01 updated the existing diagnostics paragraph in `docs/public/operations.md`.
That reference distinguishes phase timings, unchanged policy, and incomplete writer attribution.
