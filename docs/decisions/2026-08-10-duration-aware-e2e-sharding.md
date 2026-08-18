# Duration-aware E2E sharding uses rolling `main` timings

**Status:** accepted
**Date:** 2026-08-10
**Area:** infra, workflow

## Context

The E2E workflow currently uses ordinal Playwright sharding. Test counts are
balanced, but test durations are not. The investigated fourteen-shard normal
run ranged from 3m52 to 17m57, and the six-shard container run ranged from
about 3m to 9m59. New tests and changed test behavior would make committed
shard assignments increasingly stale.

The planner needs recent timings without creating a new service or requiring a
developer to edit CI whenever the suite changes. It also needs a safe answer
when the timing artifact is missing, a test is new, or a file changed after
its timing sample was recorded.

## Decision

Use a rolling timing profile from successful `main` E2E runs as the source of
truth for duration-aware planning. Store it as a bounded workflow artifact
containing project/file/title keys, recent first-attempt passing samples, file
hashes, p50/p75 values, and source metadata. On each successful `main` run,
merge new eligible samples into the previous profile and compact the history.
Pull request runs may publish timing diagnostics but cannot update the
baseline.

Generate normal and container manifests per run. Manifests are ephemeral,
validated artifacts and contain explicit project/file selections plus predicted
costs. The initial planner uses deterministic longest-processing-time bin
packing at the project/file level, with stable tie-breaking. Playwright keeps
one worker per shard. A later test-level split is allowed only for measured
file outliers.

The planner uses p75 for unchanged entries, applies a conservative multiplier
to entries whose current source-file hash differs, and falls back to a
project/cohort estimate and then a deterministic count-based plan for unknown
or unavailable data. Every fallback is reported. Invalid or incomplete
manifests fail the planning job rather than silently mixing duration-aware and
ordinal assignments.

## Consequences

- Adding tests requires no shard-file maintenance; discovery and the next
  `main` profile update include them automatically.
- Duration changes converge through the bounded sample history instead of
  depending on a manual timing refresh.
- The first run after a source change is intentionally conservative and
  reports warm entries, which can temporarily overestimate a shard.
- A missing artifact cannot block CI because the count-based fallback remains
  deterministic and visible.
- Timing data is retained in GitHub Actions artifacts, so its retention and
  access permissions must be maintained with the workflow.
- File-level planning is simple and stable, but a single very large file can
  still limit balance; test-level splitting remains a measured extension.

## Implementation status

Accepted and implemented in `.github/workflows/e2e-tests.yml`, the E2E timing,
planner, runner, and retry scripts, and the scoped E2E/backend fixtures. The
initial worker default and matrix layout remain unchanged pending the measured
rollout checks described in the plan.

## Alternatives considered

### Commit shard assignments

Rejected because new tests and timing changes would require manual edits and
would make stale assignments look authoritative.

### Use the latest pull request timings as the baseline

Rejected because a pull request may contain partial coverage, intentional
stress tests, or transient failures. Successful `main` is the comparable,
stable population.

### Keep Playwright's ordinal shard selection

Rejected because it ignores the observed duration skew and cannot express the
profile/fallback information needed to keep assignments current.

### Store timings in an external service

Deferred. An artifact is sufficient for this workflow, avoids new credentials
and availability dependencies, and can be replaced later behind the profile
contract if the suite grows beyond artifact needs.
