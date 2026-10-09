# ADR-2026-10-08-capacity-aware-ci-verification: Reduce work before increasing CI fan-out

**Status:** accepted
**Date:** 2026-10-08
**Area:** infra, workflow

## Context

The maintainer reports approximately 60 concurrent GitHub jobs and a queue exceeding 200 jobs.
Sampled application E2E runs consume about 550 runner-minutes each.
Frontend tests repeatedly initialize browser and multilingual state that many test files do not need.
Additional shards can reduce one run's execution path while increasing setup work and shared queue pressure.

The maintainer delegated optimization design choices. The existing design checkpoint still precedes implementation.

## Decision

Optimize total runner-minutes and feedback time together within existing capacity.
Preserve test assertions, isolation, race checks, desktop/mobile coverage, required-check names, and workflow trust boundaries.
Remove unnecessary setup and scheduling stages before increasing frontend fan-out.
Keep the normal E2E matrix at 14 shards and the container matrix at six.

Frontend shard adoption requires three comparable samples, at least 30% lower execution critical path, and at most 10% additional runner-minutes.
The baseline uses the optimized unsharded setup. Loaded feedback must not regress.
This replaces the previous 25% runner-minute allowance in the CI performance package.

A narrow PR-only exemption can omit application E2E when changed unit-test inputs cannot affect the application build or E2E execution.
The first exemption covers verified regular Go `_test.go` files, with existing documentation exclusions.
Unknown, mixed, embedded, renamed production, or unavailable comparison inputs retain full verification.
The unit and static checks remain required. Main, merge-group, and manual selection policies remain unchanged.

## Consequences

Measurements report execution, runner occupancy, scheduling, failures, and cancellations separately.
Test configuration changes require identity-level coverage comparisons and preserve the conservative setup for unknown files.
The exemption requires a dependency audit and contract tests, not only filename matching.
Some frontend test-only changes still pay for E2E until their build independence is proved.
An external runner pilot remains available under its existing separate trust and operator contract.

## Alternatives considered

- **Add more E2E shards:** Deferred because the existing fleet is saturated and the normal shards already have reasonable balance.
- **Enable paid capacity first:** Not required to obtain setup and workload reductions. It changes capacity rather than efficiency.
- **Run only directly changed tests:** Rejected because indirect runtime dependencies can invalidate apparently unrelated tests.
- **Remove main or merge-group runs:** Rejected because they verify distinct integration revisions and maintain the trusted timing baseline.
- **Disable isolation or delete broad test groups:** Rejected because speed alone does not prove equivalent regression protection.
- **Hold a runner while waiting for capacity:** Rejected because the waiting job consumes the capacity it attempts to protect.

## References

- [CI performance requirements](../specs/platform/requirements/ci-performance.md)
- [CI performance design](../specs/platform/system-design/ci-performance.md)
- [October evidence](../plans/ci-performance/evidence-2026-10-08.md)
- [External runner decision](2026-09-06-opt-in-external-e2e-runners.md)
