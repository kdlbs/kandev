# ADR-2026-08-24-unified-fork-approval-label: Use One Maintainer Approval Label for Contributor PR Automation

**Status:** superseded by 2026-09-07-isolate-fork-preview-builds
**Date:** 2026-08-24
**Area:** infra, workflow, security

## Context

Contributor pull requests currently use two visible labels with different
meanings: `safe-to-review` enables fork AI review and `safe-to-test` enables
fork preview deployment. The PR walkthrough workflow has an additional
same-repository guard, so a contributor pull request can receive review and
preview approval while its walkthrough job is skipped.

PR #2974 demonstrated the walkthrough failure. It was a cross-repository pull
request and its run was skipped by the explicit
`head.repo.full_name == github.repository` condition. The pull request already
had the existing approval labels when the skipped run was observed. The
problem was the workflow boundary, not a missing walkthrough label.

Maintainers want one trust decision for contributors they intentionally trust.
The repository also has allowlisted contributors. The label job writes with
`GITHUB_TOKEN`, so a label it adds does not reliably create a new downstream
`labeled` workflow run.

## Decision

Use `safe-to-review` as the only maintainer approval label for contributor pull
request AI review, preview deployment, and walkthrough generation. Remove
`safe-to-test` from active authorization expressions and from the automatic
allowlist label bridge.

Keep the existing direct allowlists as independent authorization paths. The
allowlist label bridge adds only `safe-to-review`. Workflows that need to start
on an opening or update event retain a direct allowlist expression instead of
depending on a recursive label event.

Extend the walkthrough workflow to authorized contributor pull requests while
preserving its trust boundary. The generation job keeps the trusted workflow
checkout, fetches the exact contributor head as Git data, prepares bounded
context, and does not check out or execute contributor code in its
secret-bearing worktree. Publication and pull request linking keep their
existing separate jobs and minimum permissions.

At the time of this decision, reuse the privileged fork preview path for
`safe-to-review`. That path checked out the contributor head and ran
`go run ./cmd/preview deploy` with `SPRITES_API_TOKEN` and `GITHUB_TOKEN`.
The later isolation decision in
`2026-09-07-isolate-fork-preview-builds.md` supersedes this preview boundary.

Keep approval labels durable until a maintainer removes them. Keep
`generate-pr-walkthrough` as a same-repository operational rerun label, not as
a contributor trust label.

## Consequences

- Maintainers use one label to request the three trusted contributor workflows.
- Allowlisted contributors keep automatic access through the existing direct
  allowlist gates and receive one visible approval label.
- A contributor push keeps the approval active until a maintainer removes it.
- At the time, a labeled contributor pull request could run privileged preview
  code. The repository allowlist and maintainer label process had to remain
  restricted to contributors the maintainers trusted.
- PR walkthrough generation gains fork support without allowing contributor
  files to replace its executable workflow assets.
- Existing `safe-to-test` labels become inert after rollout and require a
  one-time repository cleanup.

## Alternatives considered

- Keep separate `safe-to-review` and `safe-to-test` labels. Rejected because a
  maintainer trust decision then has two labels and can enable only part of the
  intended workflow set.
- Let `safe-to-review` enable review and walkthrough but keep a separate
  preview label. Rejected because preview is part of the requested trusted
  contributor experience.
- Use only the label written by the allowlist bridge. Rejected because
  `GITHUB_TOKEN` label writes do not provide a reliable downstream workflow
  trigger.
- Redesign preview isolation before enabling the unified label. Deferred at the
  time because the user selected the existing privileged preview path. The
  later isolation decision is recorded in
  `2026-09-07-isolate-fork-preview-builds.md`.
- Keep fork walkthroughs disabled. Rejected because the walkthrough is the
  requested explanation of contributor changes and can preserve a stronger
  trusted-input boundary than the existing privileged preview job.
