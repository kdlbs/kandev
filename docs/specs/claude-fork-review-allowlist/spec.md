---
status: building
created: 2026-07-30
owner: kdlbs
---

# Claude Fork Review Allowlist

Decision: ADR-2026-07-31-isolate-manual-pr-review-content; ADR-2026-08-07-claude-allowlist-label-bridge

## Why

Trusted fork contributors listed in the repository's Claude review allowlist should receive an automatic review when they open a pull request, without requiring a maintainer to label it. Further review rounds should be explicit so pushes do not repeatedly consume Claude tokens. The workflow must pass the already-authorized contributor to Claude's separate non-write-user permission gate.

The same trusted contributors should also receive the repository's existing review and preview approval markers, so maintainers do not need to maintain a second identity list or apply two labels manually.

## What

- A fork pull request actor listed in `CLAUDE_REVIEW_ALLOWLIST` receives one automatic review when they open a pull request and passes both the workflow job gate and the Claude action's non-write-user gate.
- `CLAUDE_REVIEW_ALLOWLIST` remains a JSON array for safe use in GitHub Actions expressions.
- The Claude action receives the already job-authorized pull request author through its `allowed_non_write_users` input.
- When an allowlisted fork pull request opens, the base-controlled workflow adds both existing labels, `safe-to-review` and `safe-to-test`, to the pull request. The label operation is idempotent and applies only to fork pull requests whose opening author matches the allowlist.
- The `CLAUDE_REVIEW_ALLOWLIST` gate remains a direct authorization path for the fork review job, so adding labels with the repository `GITHUB_TOKEN` does not create a second review run through the `labeled` event.
- The preview workflow treats an author in `CLAUDE_REVIEW_ALLOWLIST` as trusted for fork preview deployment on every non-closed pull-request-target event it already handles (`opened`, `synchronize`, `reopened`, and `labeled`). This keeps preview deployment automatic even though the existing per-commit `safe-to-test` cleanup removes that label after a fork push.
- A maintainer may apply `safe-to-review` to request the initial review of an untrusted fork pull request.
- Pushes, ready-for-review transitions, and reopenings do not automatically start another Claude review. A maintainer can request a later review by commenting `@claude review` on the pull request. That requested review reads the current pull request head, including files newly added by the pull request.
- Manual pull request reviews keep the trusted default branch at the workflow root and do not check out pull request content. Claude may use read-only local tools for trusted surrounding code, reads the current diff through constrained GitHub commands, and reads complete current-head PR files only through a path-validated, size-limited GET helper bound to the event's PR number. Its only write capability is posting review comments.
- Other Claude mentions keep the generic workflow behavior and trusted default-branch checkout.
- The same-repository review path remains unchanged except for the open-only trigger policy.
- Empty, malformed, or non-matching allowlists continue to fail closed at the workflow job gate.

## Permissions

- Only the base-controlled `pull_request_target` workflow may add the two labels. It does not check out or execute pull-request content for the labeling step.
- A matching `CLAUDE_REVIEW_ALLOWLIST` entry authorizes the existing fork review path and the preview workflow's fork deployment path, which has access to the preview deployment credentials. Repository maintainers are responsible for keeping this variable restricted to trusted contributors.
- A non-allowlisted fork author still needs the existing maintainer-applied labels; the new automation does not broaden the manual label paths.

## Failure modes

- An empty, malformed, or non-matching `CLAUDE_REVIEW_ALLOWLIST` produces no automatic labels and does not authorize the review or preview jobs.
- If the label API call fails or either repository label does not exist, the labeling job fails visibly. The direct review and preview gates still evaluate the allowlist independently; a label-write failure does not turn an untrusted author into a trusted one.
- Labels added with `GITHUB_TOKEN` do not themselves launch a new workflow run. The review and preview workflows therefore must retain their direct allowlist gates rather than depending only on the resulting `labeled` event.

## Scenarios

- **GIVEN** `CLAUDE_REVIEW_ALLOWLIST` is `["ClemDNL"]`, **WHEN** `ClemDNL` opens a fork pull request, **THEN** the fork review job supplies `ClemDNL` through `allowed_non_write_users` and Claude does not reject the run solely because the actor has repository permission `read`.
- **GIVEN** `CLAUDE_REVIEW_ALLOWLIST` is `["ClemDNL"]`, **WHEN** `ClemDNL` opens a fork pull request, **THEN** the pull request receives both `safe-to-review` and `safe-to-test` labels without a maintainer action.
- **GIVEN** an allowlisted fork pull request has a new commit pushed, **WHEN** the preview workflow handles the `synchronize` event, **THEN** the preview deploy job remains eligible through `CLAUDE_REVIEW_ALLOWLIST` even if the existing cleanup removes `safe-to-test`.
- **GIVEN** a fork pull request has received its initial review, **WHEN** its contributor pushes another commit, marks it ready for review, or reopens it, **THEN** the Claude review workflow does not run again.
- **GIVEN** a fork contributor is absent from `CLAUDE_REVIEW_ALLOWLIST`, **WHEN** they open or update their pull request without `safe-to-review`, **THEN** the fork review job does not run.
- **GIVEN** a fork contributor is absent from `CLAUDE_REVIEW_ALLOWLIST`, **WHEN** they open a pull request, **THEN** the automatic labeling job adds neither approval label and the preview workflow does not deploy unless its existing `PREVIEW_ENV_ALLOWLIST` or `safe-to-test` path authorizes it.
- **GIVEN** a maintainer applies `safe-to-review` to a fork pull request, **WHEN** the labeled event runs, **THEN** the existing maintainer-approved review path remains available.
- **GIVEN** a maintainer wants another review round, **WHEN** they comment `@claude review` on the pull request, **THEN** the existing Claude mention workflow recognizes the `@claude` mention and starts the requested review.
- **GIVEN** a pull request adds a file after its initial review, **WHEN** a maintainer comments `@claude review`, **THEN** Claude can read and review that added file rather than ending without a review because the file is absent from the default-branch checkout.
- **GIVEN** a changed file depends on code outside its diff hunks, **WHEN** Claude performs a manual review, **THEN** it can explore the trusted default-branch codebase and request the complete UTF-8 version of a specific file from the current PR head for semantic context.
- **GIVEN** a user mentions `@claude` on an issue that is not a pull request, **WHEN** the generic Claude mention workflow runs, **THEN** it continues to use the default-branch checkout.
- **GIVEN** a pull request changes Claude project settings or repository instructions, **WHEN** a maintainer comments `@claude review`, **THEN** the trusted default branch remains the agent workspace and pull request content is read only through constrained GitHub commands or the bound GET-only file helper.

## Out of scope

- Changing the pinned Claude Code Action version.
- Changing the Claude OAuth token, GitHub token, or OIDC strategy.
- Creating a second preview-specific allowlist or removing `PREVIEW_ENV_ALLOWLIST`.
- Re-adding approval labels after every fork push or using a new personal access token/GitHub App solely to make label events recurse into other workflows.
- Changing the OpenCode review workflow.
- Changing the automatic review behavior for same-repository pull requests.
