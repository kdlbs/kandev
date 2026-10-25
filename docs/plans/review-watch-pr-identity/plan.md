---
created: 2026-09-25
status: complete
requirements:
  - REQ-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001
system_design:
  - ../../specs/integrations/system-design/github-fork-review-start.md
legacy_specs: []
---

# Implementation Plan: Review-watch PR identity

## Overview

Restore configured auto-start for same-repository GitHub review-watch PRs by
preserving repository identity during search-result enrichment. One work order
covers the provider-to-orchestrator path and its regression tests.

## Evidence and root cause

`CheckReviewWatch` obtains lightweight PRs through GitHub search, then calls
`enrichPRDetails` before publishing `GitHubNewReviewPR`. The latter currently
copies branch names, SHAs, diff counts, and mergeability, but omits head
repository identity. `reviewPRHeadMatchesBase` requires that identity, so
`buildReviewTaskRequest` marks an internal PR as requiring manual start and
does not grant `auto_start_claimed`. The existing orchestrator unit test gives
the event a complete PR directly, so it does not exercise enrichment.

This violates
[`AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.1`](../../specs/integrations/requirements/github-fork-review-start.md).
The existing requirement and fork security decision remain correct. The
[system design](../../specs/integrations/system-design/github-fork-review-start.md)
now states the provider-data contract that was missing.

## Scope

### In scope

- Copy head and base repository metadata and PR detail fields from `GetPR`
  into newly found review-watch PRs.
- Fetch details when branch names exist but head repository identity is absent.
- Prove that a same-repository search hit gains its auto-start token after
  enrichment, while a fork or failed detail fetch remains manual-start only.

### Out of scope

- Changes to fork trust policy, automated launch gates, workflow admission, or
  manual PR-link behavior.
- Automatic repair of tasks already created with the manual-start marker or
  direct edits to existing SQLite task metadata.
- Rendered UI changes or new public API fields.

## Technical approach

In `apps/backend/internal/github/service_reviews.go`, update
`enrichPRDetails` to copy the detail response's head repository owner, name,
node ID, clone URL, base repository owner/name, base SHA, mergeability state,
and maintainer permission. Preserve all existing copied fields. Also carry
repository numeric IDs and base default branch if supplied by the same full
response, so the enriched `PR` stays coherent. Keep the detail fetch on missing
branch or head identity; on fetch error leave the identity incomplete.

The review event already carries the returned `PR` without a second mapping.
`buildReviewTaskRequest` remains the trust decision point. Do not infer a
head repository from the search-result target.

## Tests

| Criterion | Evidence |
| --- | --- |
| AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.1 | `service_reviews_test.go` verifies detail fields are copied; `TestReviewWatchSearchResultIdentityControlsAutoStart` runs a search-shaped result through `TriggerReviewWatch`, then passes that same returned event PR to `buildReviewTaskRequest` and verifies internal identity grants `auto_start_claimed=true` without a manual-start marker. |
| AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.2 | The same end-to-end test verifies a fork head and failed detail lookup retain the manual-start marker and omit the auto-start claim. |

The new provider regression must fail before the production change because
the head repository fields remain empty.

## E2E tests

The backend integration test connects the watch search result to the review
event and task request without a browser. Its mock client models distinct
search and detail responses. Existing
`apps/web/e2e/tests/pr/pr-watcher-dockview-layout.spec.ts` exercises a mock
watch's visible auto-start behavior; no rendered UI behavior changes here.

## Work orders

- [x] [Task 01: Preserve review PR identity through enrichment](task-01-preserve-review-pr-identity.md)

## Verification results

The full GitHub package tests and targeted orchestrator tests passed, including
the search-result enrichment event path and same-repository/fork/unknown
identity launch decisions. Focused race tests for both packages, the backend
build, and backend lint passed. The environment-isolated full backend suite did
not pass because two real-process probe assertions in
`internal/agentctl/server/process/probe` returned `live` where they expect
`settled`; an isolated rerun reproduced both failures.

## Risks

- A failed or incomplete `GetPR` response must continue to fail closed.
- Copying only owner/name would fix launch but leave downstream PR metadata
  incomplete; the field-completeness test guards the whole detail mapping.
