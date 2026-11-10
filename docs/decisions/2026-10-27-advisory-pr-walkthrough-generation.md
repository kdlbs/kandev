# ADR-2026-10-27-advisory-pr-walkthrough-generation: Treat PR walkthrough generation as best effort

**Status:** accepted
**Date:** 2026-10-27
**Area:** workflow, infra

## Context

PR walkthrough generation depends on an OpenCode free-tier model with no
availability guarantee. On 2026-10-27 the free model stopped returning tokens
in GitHub Actions. Every walkthrough run reached the runner's 600-second
deadline with an empty draft, and the generation job failed on unrelated pull
requests for nearly three hours. A previous free model disappeared from the
catalog in October 2026, so the dependency can break without a repository
change.

The walkthrough is supplementary review material. It publishes an explanation
and adds a callout to the pull request description. Its absence does not affect
the code, tests, or review evidence that a pull request carries.

## Decision

Walkthrough generation is advisory in CI. A generation attempt that does not
produce a verified output pair reports a workflow warning annotation and a job
summary entry, and the generation job stays successful.

The generation job exports `generated=true` only when the supervised runner
returns a verified output pair. The publication and description-link jobs run
only when that output is `true`, so an unverified attempt never publishes or
rewrites the pull request description.

Diagnostics still upload for every outcome. Staging, upload, and missing-artifact
failures are advisory when generation produced no verified pair, so capture
problems cannot turn a skipped walkthrough into a red check; they stay strict
once a verified pair must reach publication.

Publication and description linking remain strict. Once generation produced a
verified pair, an R2 upload, public-URL validation, or race-safe body update
failure still fails its own job.

Runner supervision, the renderer completion receipt, output verification, and
the bounded generation deadline are unchanged. Only the CI failure
presentation and the downstream gating change.

## Consequences

- A free-tier stall, provider outage, or retired model no longer blocks pull
  requests.
- A generation regression is visible through the warning annotation, the job
  summary, and the uploaded artifact instead of a failed check.
- Failed attempts skip publication and linking. An existing callout for an
  older head remains until a later successful run replaces it.
- Operators must watch walkthrough warnings and artifacts to detect a systemic
  model problem rather than relying on a red check.
- Publishing and description-integrity failures keep their existing severity.

## Alternatives Considered

- **Keep failing the generation check:** This couples every pull request to an
  unsupported third-party free model and blocks unrelated work.
- **Retry the attempt more times:** The deadline is already bounded and an
  outage still consumes the budget, so retries add delay without changing the
  outcome.
- **Disable the workflow:** This removes a useful review aid and loses the
  diagnostics that identify model problems.
- **Move to a paid model without changing failure semantics:** This reduces but
  does not remove the outage class, and it can be combined with this decision
  later.
- **Make publication and linking best effort too:** Those failures indicate
  real hosting or description-integrity problems that should stay visible once
  a verified walkthrough exists.
