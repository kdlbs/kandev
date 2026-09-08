# ADR-2026-08-23-pr-walkthrough-workflow-provenance: Use the workflow SHA for trusted PR walkthrough inputs

**Status:** accepted
**Date:** 2026-08-23
**Area:** workflow, infra, security

## Context

The PR walkthrough uses `pull_request_target` because the generation job needs
a model secret. GitHub loads this workflow from the trusted default branch.
However, the workflow checked out `github.event.pull_request.base.sha` for its
skill, setup action, context helper, and PR-description helper.

The event base SHA can remain older than the workflow commit after a pull
request receives default-branch changes. This state caused one pipeline to use
new publication code and an old PR-description helper. The publisher created a
12-character URL, but the old helper required a full-SHA URL.

A secret-bearing job must not load executable content from the pull request.
It also must not mix trusted components from different commits.

## Decision

Use `github.workflow_sha` as the single trusted commit for the complete PR
walkthrough pipeline. GitHub defines this value as the commit SHA for the
workflow file.

The generation job checks out this commit and makes sure that the checked-out
`HEAD` has the same value. It uses this commit for these inputs:

- repository guidance
- the complete `pr-walkthrough` skill
- the OpenCode setup action
- the comparison base and merge-base resolution
- the context helper and renderer.

The PR-link job also checks out this commit. It uses the PR-description helper
from the same commit. The workflow does not use the event base SHA to select
instructions, actions, scripts, or comparison content.

The exact event head SHA remains the untrusted data boundary. The workflow
fetches it as an immutable Git object and never checks it out in the
secret-bearing worktree.

This decision refines
`2026-08-22-pr-walkthrough-filesystem-runner`. In that decision, references to
the exact base commit now mean the trusted workflow commit.

## Consequences

- One commit defines the workflow and every trusted helper that it invokes.
- A stale event base SHA cannot combine old helpers with new workflow steps.
- A workflow rerun keeps one coherent trusted version for the complete run.
- The comparison uses the trusted workflow commit and the exact event head.
  Git resolves their merge base before it prepares the PR change set.
- The workflow still keeps pull request code away from model secrets and
  GitHub write credentials.

## Alternatives Considered

- **Use the event base SHA:** The value is trusted, but it can be older than
  the workflow commit. This choice caused the mixed-version error.
- **Fetch the current default branch at runtime:** This value can change after
  the workflow starts. It does not give one immutable provenance value.
- **Use the pull request head SHA:** This value is contributor-controlled. It
  cannot supply executable code to a secret-bearing job.
