# ADR-2026-10-25-automatic-review-input-isolation: Isolate automatic review inputs

**Status:** accepted
**Date:** 2026-10-25
**Area:** workflow, infra, security

## Context

Automatic OpenCode review checks out the PR head before loading the CLI with a
provider credential. Removing `.opencode` leaves root `opencode.json` and
`opencode.jsonc` available. The pinned CLI loads their plugins before model tool
permissions can constrain them. The Claude fork workflow also checks out PR
content and explicitly selects a PR-owned review skill. Its pinned action
restores sensitive Claude configuration but does not restore that skill.

The current [OpenCode evidence ADR](2026-07-23-opencode-review-evidence-trust.md)
explicitly requires a head checkout. The amendment replaces only that
checkout requirement. Exact-head evidence, dedicated producer identity, protected
environment, full-PR review, and advisory fork results remain required.

## Decision

Use the immutable workflow revision as the execution checkout for automatic
Claude fork review and both OpenCode review paths. Fetch PR commits as Git
objects. Provide bounded text context and an immutable diff. Agent instructions,
configuration, setup actions, and posting helpers come from trusted source.

Use neutral blob filenames for prepared review and walkthrough context. Keep
original paths only in a manifest mapping. Preserving `AGENTS.md`, `CLAUDE.md`,
or `CONTEXT.md` beside prepared source lets the pinned OpenCode reader load PR
text as instructions. One shared layout prevents drift between agent workflows.

Contributor approval authorizes a review. It does not authorize PR configuration
to execute in the credential-bearing reviewer process. Reuse the manual Claude
and walkthrough isolation patterns instead of introducing a new review service.

The [requirements](../specs/ci/requirements/privileged-workflow-inputs.md) and
[design](../specs/ci/system-design/privileged-workflow-inputs.md) define the change.
The existing ADR retains the producer policy. Its checkout clause now follows
this decision, with implementation and regression evidence in the linked plan.

## Consequences

Reviewers retain exact PR data without placing it in automatic configuration
discovery locations. Tests must cover configuration files, symlinks, moving
heads, and failed preparation. Complete-file context remains bounded. Trusted
surrounding code and the diff provide context when a file is omitted.

The implementation updates tests that currently require a head checkout.
It must preserve their assertions about event head identity and review provenance.
No model, provider, review trigger, approval label, or publication identity changes.

## Alternatives considered

- Keep the PR checkout and remove known config files: rejected because additional
  discovery paths, project tools, and instructions would remain under PR control.
- Rely on model tool denials: rejected because startup plugin loading precedes them.
- Preserve source paths beneath a data directory: rejected because nested
  instruction discovery still treats reserved filenames as agent policy.
- Remove target triggers: rejected because authorized fork automation needs the
  trusted workflow and existing credential/permission routing.
- Treat contributor approval as arbitrary-code approval: rejected because it
  conflicts with `.github/AGENTS.md` and the existing manual-review isolation rule.
