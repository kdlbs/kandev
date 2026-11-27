---
created: 2026-10-05
status: implemented
requirements:
  - REQ-RELEASE-CONTRIBUTOR-NOTIFICATIONS-001
  - REQ-RELEASE-CONTRIBUTOR-NOTIFICATIONS-002
  - REQ-RELEASE-CONTRIBUTOR-NOTIFICATIONS-003
system_design:
  - ../../specs/release/system-design/contributor-notifications.md
legacy_specs: []
---

# Implementation Plan: Release Contributor Notifications

## Overview

Implement the tested notification helper first. Then add the manual workflow,
release checkbox, CI checks, and maintainer documentation in one integration work order.
The release system owns this package because it owns publication and maintainer release operations.

## Scope

The package covers optional tag selection, latest-release fallback, eligible external contributors,
duplicate prevention, partial failure recovery, and opt-in notices after Stable publication.
It excludes scheduled notices, email, PRs absent from notes, and publication changes.
The user confirmed manual dispatch and the release checkbox. The box defaults to unchecked.
The comment text and external-contributor scope continue the preceding approved notification task.

## Technical approach

Task 01 adds the Python helper and mocked API/Git tests.
It reads the maintainer declaration in `cliff.toml`, resolves release-note candidates,
and compares merge commits with the bound tag before posting.
Conversation comments provide the completion record.

Task 02 creates a workflow with `workflow_dispatch` and `workflow_call` inputs.
It uses the helper from the workflow revision, a shared posting lock, and the built-in job token.
The Release workflow calls it with its exact tag after all four publication channels succeed.
Wire helper and workflow tests into `.github/workflows/lint-action-pinning.yml` and the root `make test-scripts` target.
Update the release guide, root engineering guide, and release skill in the same implementation.

## ASCII UI preview

UI-01: GitHub Actions, Notify release contributors, Run workflow.

```text
Release tag (optional)
[                              ]
Leave empty for the latest published Stable release.
[ ] Preview without posting comments
[ Run workflow ]
```

UI-02: GitHub Actions, Release, Run workflow. Existing inputs remain in their current order.

```text
... existing release inputs ...
[ ] Notify external contributors after a successful Stable release
[ Run workflow ]
```

These previews specify GitHub-native controls, not Kandev frontend changes.
GitHub owns their desktop and phone rendering. No Kandev UI or localization catalogs change.
UI-01 maps to requirement 001; UI-02 maps to requirement 003.

## Tests and verification

| Coverage | Evidence |
| --- | --- |
| AC-001.1 through 001.5 | Helper tests for latest versus explicit tag, invalid release, and preview; workflow input contract tests |
| AC-002.1 through 002.3 | Fixture tests for maintainer and bot exclusion, duplicates, wrong repository, non-PR links, and tag ancestry |
| AC-002.4 through 002.5 | Tests for markers, legacy visible notices, later comment pages, and common concurrency lock |
| AC-002.6 through 002.7 | Tests for uncertain POST results, partial failure, rate limits, and summaries |
| AC-003.1 through 003.6 | Workflow tests for all success gates, exact tag, unchecked default, backfill, and excluded modes |

Use mocked API responses for implementation checks. Do not post live test comments.
After deployment, a manual preview for v0.97.0 is read-only evidence for the real selection and existing notices.
The first real notification run is an operator action outside local implementation verification.

## Work orders

- [x] [Task 01: Implement the notification helper](task-01-notification-helper.md)
- [x] [Task 02: Add workflows and maintainer guidance](task-02-workflows-and-guidance.md)

Execute sequentially. Task 02 depends on Task 01. No subagents are authorized.
Each work order contains its exact commands and acceptance conditions.

## Verification results

Both work orders are complete. Verification passed on 2026-10-05:

- Notification helper: 23 tests passed; notification workflow contract: 4 passed; release workflow contract: 49 passed; action-pinning tests: 9 passed.
- Action-pinning audit: all 26 workflow files passed.
- `make test-scripts`: passed with the web and desktop preview bundles built first (`make build-web` and `pnpm --filter @kandev/desktop build:vite`).
- Public docs validation: 47 pages passed; public docs validator tests: 62 passed.
- Harness tests: 19 passed; harness lint: all 201 files passed.
- `python3 scripts/list-docs.py validate`: 348 decisions and 1343 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Mutation check: the release workflow contract test rejected changing a required `&&` success gate to `||`.
- `git diff --check`: passed.
- `zizmor .github/workflows` exits 14 on existing repository findings. The Release workflow has 27 findings at both `HEAD` and the implementation revision, with zero route-level changes; the new notification workflow has zero findings.

## Risks

- Release notes can contain links that do not identify a PR. Selection must use PR metadata and Git ancestry.
- GitHub can throttle comment creation. Serial writes and repeat-run detection protect recovery.
- Cancellation during posting can leave a partial result; trusted comment identity and repeat-run detection make the manual retry safe.
- A helper older than this feature cannot serve a backfill. Execute the helper from the workflow revision.
- Existing API automation can produce notices without the new marker. Match the approved legacy text as well.
