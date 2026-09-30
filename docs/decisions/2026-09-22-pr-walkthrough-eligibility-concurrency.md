# ADR-2026-09-22-pr-walkthrough-eligibility-concurrency: Isolate ineligible PR walkthrough triggers

**Status:** accepted
**Date:** 2026-09-22
**Area:** workflow

## Context

The PR walkthrough workflow receives every supported pull request event, but
its generation decision runs in the job's `if` expression. GitHub applies the
workflow concurrency group before that job condition. Any event that fails the
generation gate can therefore cancel an eligible run for the same pull
request, including an unauthorized fork update, a draft event, or an event
received while generation is disabled.

PR #3708 demonstrated this failure. The `safe-to-review` event started an
eligible generation run. A simultaneous `safe-to-test` event entered the same
per-pull-request group, canceled the eligible run, and then skipped its own
generation job because `safe-to-test` is not an authorization label.

## Decision

Keep one canceling concurrency group for eligible walkthrough triggers, and
route every event rejected by the full generation gate to a separate
per-pull-request group before the job-level authorization gate runs.

The workflow-level expression mirrors the generation gate's toggle, event,
draft, repository, action, label, and allowlist conditions. Eligible
same-repository `generate-pr-walkthrough` events and contributor
`safe-to-review` or allowlisted events remain in
`pr-walkthrough-<number>`. Every other event uses an `-ineligible` suffix. The
job-level authorization expression remains the source of permission and
continues to fail closed.

## Consequences

- A newer eligible trigger still cancels an older eligible pipeline for the
  same pull request.
- An event rejected by the generation gate can no longer cancel generation,
  publication, or linking for an eligible trigger.
- Rejected events still create skipped workflow runs, which preserve the
  existing audit trail and do not grant any capability.
- The concurrency expression must remain aligned with the job-level trigger
  eligibility rules, and the workflow contract test must protect both paths.

## Alternatives Considered

- Set `cancel-in-progress: false` for every walkthrough run. Rejected because
  it changes the existing latest-eligible-run behavior and can spend model
  capacity on stale eligible heads.
- Remove the `labeled` event trigger. Rejected because approved contributor
  pull requests would lose the required `safe-to-review` activation path.
- Give every label its own concurrency group. Rejected because eligible
  labels would no longer serialize with synchronize and other eligible events
  for the same pull request.
