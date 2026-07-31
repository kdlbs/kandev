---
status: building
created: 2026-07-30
owner: kdlbs
---

# Claude Fork Review Allowlist

## Why

Trusted fork contributors listed in the repository's Claude review allowlist should receive automatic reviews without requiring a maintainer to label every commit. The workflow currently starts for those contributors but the Claude action rejects them because its separate non-write-user permission gate receives no allowlist.

## What

- A fork pull request actor listed in `CLAUDE_REVIEW_ALLOWLIST` passes both the workflow job gate and the Claude action's non-write-user gate.
- `CLAUDE_REVIEW_ALLOWLIST` remains a JSON array for safe use in GitHub Actions expressions.
- The Claude action receives the already job-authorized pull request author through its `allowed_non_write_users` input.
- The existing `safe-to-review` label path and same-repository review path remain unchanged.
- Empty, malformed, or non-matching allowlists continue to fail closed at the workflow job gate.

## Scenarios

- **GIVEN** `CLAUDE_REVIEW_ALLOWLIST` is `["ClemDNL"]`, **WHEN** `ClemDNL` synchronizes a fork pull request, **THEN** the fork review job supplies `ClemDNL` through `allowed_non_write_users` and Claude does not reject the run solely because the actor has repository permission `read`.
- **GIVEN** a fork contributor is absent from `CLAUDE_REVIEW_ALLOWLIST`, **WHEN** they synchronize their pull request without a current `safe-to-review` approval, **THEN** the fork review job does not run.
- **GIVEN** a maintainer applies `safe-to-review` to a fork pull request, **WHEN** the labeled event runs, **THEN** the existing maintainer-approved review path remains available.

## Out of scope

- Changing the pinned Claude Code Action version.
- Changing the Claude OAuth token, GitHub token, or OIDC strategy.
- Changing the OpenCode review workflow.
- Changing review behavior for same-repository pull requests.
