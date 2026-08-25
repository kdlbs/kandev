---
status: active
system: release
created: 2026-08-17
owners:
  - Kandev maintainers
---
# Stable release PR queue bypass Requirements

## Overview

Stable releases create a mechanical version commit and must continue after that commit reaches `main`. The merge queue now blocks this release commit on CI that maintainers do not require for generated release changes.

## Requirements

### REQ-RELEASE-RELEASE-PR-QUEUE-BYPASS-001: Stable release PR queue bypass

**Intent:** Stable releases create a mechanical version commit and must continue after that commit reaches `main`. The merge queue now blocks this release commit on CI that maintainers do not require for generated release changes.

#### Acceptance criteria

- **AC-RELEASE-RELEASE-PR-QUEUE-BYPASS-001.1:** A normal Stable release creates a release branch and a pull request before it changes `main`.
- **AC-RELEASE-RELEASE-PR-QUEUE-BYPASS-001.2:** The release pull request merges immediately without waiting for pull-request checks or the merge queue.
- **AC-RELEASE-RELEASE-PR-QUEUE-BYPASS-001.3:** A fine-grained personal access token from an organization administrator performs only the privileged pull-request merge.
- **AC-RELEASE-RELEASE-PR-QUEUE-BYPASS-001.4:** The normal `GITHUB_TOKEN` remains responsible for the release branch and pull-request creation.
- **AC-RELEASE-RELEASE-PR-QUEUE-BYPASS-001.5:** The merge operation matches the expected release head commit and uses a squash merge.
- **AC-RELEASE-RELEASE-PR-QUEUE-BYPASS-001.6:** The release pull-request title satisfies the repository title contract.
- **AC-RELEASE-RELEASE-PR-QUEUE-BYPASS-001.7:** The workflow creates the signed tag only after GitHub reports that the pull request merged.
- **AC-RELEASE-RELEASE-PR-QUEUE-BYPASS-001.8:** The signed tag targets the reported release merge commit, even when another commit reaches `main` immediately afterward.

## Migrated source detail

## Why

Stable releases create a mechanical version commit and must continue after that commit reaches `main`. The merge queue now blocks this release commit on CI that maintainers do not require for generated release changes.

## What

- A normal Stable release creates a release branch and a pull request before it changes `main`.
- The release pull request merges immediately without waiting for pull-request checks or the merge queue.
- A fine-grained personal access token from an organization administrator performs only the privileged pull-request merge.
- The normal `GITHUB_TOKEN` remains responsible for the release branch and pull-request creation.
- The merge operation matches the expected release head commit and uses a squash merge.
- The release pull-request title satisfies the repository title contract.
- The workflow creates the signed tag only after GitHub reports that the pull request merged.
- The signed tag targets the reported release merge commit, even when another commit reaches `main` immediately afterward.
- Dry runs, Desktop validation, Nightly releases, and `backfill_tag` runs do not request the bypass token.

## Permissions

- The token selects only the `kdlbs/kandev` repository and has `contents: write` permission.
- The token owner remains an organization administrator while the token is active.
- The existing organization-administrator entry in the `main` ruleset provides the bypass.
- The protected `release` environment owns the `RELEASE_PR_BYPASS_TOKEN` secret.
- The workflow exposes the token only to the privileged merge step.

## Failure modes

- A missing, expired, revoked, or invalid token stops the release before the pull request merges.
- A token owner without organization-administrator bypass access causes the merge step to fail closed.
- A changed pull-request head causes the matched-head merge to fail closed.
- A closed or unmerged pull request stops the release before tag signing.
- A merge commit that is absent from `origin/main` stops the release before tag signing.

## Scenarios

- **GIVEN** a valid administrator token, **WHEN** a normal Stable release creates its pull request, **THEN** it merges that exact head without CI or queue enrollment.
- **GIVEN** the release pull request merges, **WHEN** another commit reaches `main`, **THEN** the signed release tag still targets the reported release merge commit.
- **GIVEN** the token owner lacks bypass access, **WHEN** the workflow requests the privileged merge, **THEN** the workflow stops before tag creation and publication.
- **GIVEN** the release branch changes after pull-request creation, **WHEN** the workflow requests the privileged merge, **THEN** the head-match guard rejects the merge.
- **GIVEN** an excluded release mode, **WHEN** the prepare job runs, **THEN** it does not use the administrator token.

## Out of scope

- Skipping CI for ordinary pull requests.
- Granting the release workflow direct-push access to `main`.
- Changing merge-queue checks or their workflow triggers.
- Automating personal access token creation or rotation.
- Recovering a release pull request after its originating workflow has stopped.
