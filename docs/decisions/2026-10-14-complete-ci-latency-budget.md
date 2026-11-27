# ADR-2026-10-14-complete-ci-latency-budget: Measure the complete CI path

**Status:** accepted
**Date:** 2026-10-14
**Area:** infra, workflow

## Context

PR #4457 reduced the longest normal E2E step by 51.46%, but a complete subsequent
workflow set still took 30m03s. Independent Windows, PostgreSQL and frontend paths
also exceeded fifteen minutes. The maintainer now asks for full CI to finish in
fifteen minutes and explicitly authorizes continuous work through completion.

## Decision

Judge the new target by complete backend, frontend and E2E workflow sets,
including waiting, setup and reporting. Preserve all relevant platform and test
coverage and required failure gates. Continue to measure runner occupancy and
loaded feedback independently of elapsed time.

Supersede only the fixed fourteen-normal/six-container E2E experiment restriction
in ADR-2026-10-08-capacity-aware-ci-verification for this continuation. Measured
hosted partitions are permitted; no additional paid fleet is activated. The
frontend 10% occupancy adoption gate remains. A fan-out increase is a scheduling
tradeoff and must not be presented as reduced compute. First remove unnecessary
work and serial dependencies, then test bounded fan-out where needed.

## Consequences

More simultaneous jobs can shorten one execution path but worsen shared queues.
Record complete attempts, coverage and occupancy before adoption. Three complete
successful samples within the budget establish delivery evidence, not a queue
percentile or permanent ceiling. External-provider and protected-job trust
boundaries remain unchanged.

## Alternatives considered

- Optimize E2E alone: cannot remove independent backend/frontend bottlenecks.
- Count only test-step time: omits setup and scheduling the maintainer waits for.
- Delete coverage or increase retries: conceals regressions or adds work.
- Activate paid capacity immediately: needs an explicit operator decision and
  cannot replace elimination of serial work.
