---
status: building
created: 2026-07-30
owner: kdlbs
---

# Claude Fork Review Allowlist

Decision: ADR-2026-07-31-isolate-manual-pr-review-content

## Why

Trusted fork contributors listed in the repository's Claude review allowlist should receive an automatic review when they open a pull request, without requiring a maintainer to label it. Further review rounds should be explicit so pushes do not repeatedly consume Claude tokens. The workflow must pass the already-authorized contributor to Claude's separate non-write-user permission gate.

## What

- A fork pull request actor listed in `CLAUDE_REVIEW_ALLOWLIST` receives one automatic review when they open a pull request and passes both the workflow job gate and the Claude action's non-write-user gate.
- `CLAUDE_REVIEW_ALLOWLIST` remains a JSON array for safe use in GitHub Actions expressions.
- The Claude action receives the already job-authorized pull request author through its `allowed_non_write_users` input.
- A maintainer may apply `safe-to-review` to request the initial review of an untrusted fork pull request.
- Pushes, ready-for-review transitions, and reopenings do not automatically start another Claude review. A maintainer can request a later review by commenting `@claude review` on the pull request. That requested review reads the current pull request head, including files newly added by the pull request.
- Manual pull request review content is isolated under a read-only subtree while the trusted default branch remains at the workflow root. Checkout-provided credentials are not persisted, and Claude receives only review and comment tools.
- Other Claude mentions keep the generic workflow behavior and trusted default-branch checkout.
- The same-repository review path remains unchanged except for the open-only trigger policy.
- Empty, malformed, or non-matching allowlists continue to fail closed at the workflow job gate.

## Scenarios

- **GIVEN** `CLAUDE_REVIEW_ALLOWLIST` is `["ClemDNL"]`, **WHEN** `ClemDNL` opens a fork pull request, **THEN** the fork review job supplies `ClemDNL` through `allowed_non_write_users` and Claude does not reject the run solely because the actor has repository permission `read`.
- **GIVEN** a fork pull request has received its initial review, **WHEN** its contributor pushes another commit, marks it ready for review, or reopens it, **THEN** the Claude review workflow does not run again.
- **GIVEN** a fork contributor is absent from `CLAUDE_REVIEW_ALLOWLIST`, **WHEN** they open or update their pull request without `safe-to-review`, **THEN** the fork review job does not run.
- **GIVEN** a maintainer applies `safe-to-review` to a fork pull request, **WHEN** the labeled event runs, **THEN** the existing maintainer-approved review path remains available.
- **GIVEN** a maintainer wants another review round, **WHEN** they comment `@claude review` on the pull request, **THEN** the existing Claude mention workflow recognizes the `@claude` mention and starts the requested review.
- **GIVEN** a pull request adds a file after its initial review, **WHEN** a maintainer comments `@claude review`, **THEN** Claude can read and review that added file rather than ending without a review because the file is absent from the default-branch checkout.
- **GIVEN** a user mentions `@claude` on an issue that is not a pull request, **WHEN** the generic Claude mention workflow runs, **THEN** it continues to use the default-branch checkout.
- **GIVEN** a pull request changes Claude project settings or repository instructions, **WHEN** a maintainer comments `@claude review`, **THEN** the trusted default branch remains the agent workspace and the pull request content is treated only as review data.

## Out of scope

- Changing the pinned Claude Code Action version.
- Changing the Claude OAuth token, GitHub token, or OIDC strategy.
- Changing the OpenCode review workflow.
- Changing the automatic review behavior for same-repository pull requests.
