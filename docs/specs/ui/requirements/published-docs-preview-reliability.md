---
status: active
system: ui
created: 2026-07-31
owners:
  - kdlbs
---
# Published Docs Preview Reliability Requirements

## Overview

Pull request documentation previews intermittently fail when the landing build cannot fetch live public GitHub community data. Contributors need preview results to depend on the proposed documentation and publisher contract, not on the shared unauthenticated GitHub API quota of a hosted runner.

## Requirements

### REQ-UI-PUBLISHED-DOCS-PREVIEW-RELIABILITY-001: Published Docs Preview Reliability

**Intent:** Pull request documentation previews intermittently fail when the landing build cannot fetch live public GitHub community data. Contributors need preview results to depend on the proposed documentation and publisher contract, not on the shared unauthenticated GitHub API quota of a hosted runner.

#### Acceptance criteria

- **AC-UI-PUBLISHED-DOCS-PREVIEW-RELIABILITY-001.1:** Same-repository pull request previews authenticate landing's GitHub API reads with the workflow job's short-lived `GITHUB_TOKEN`.
- **AC-UI-PUBLISHED-DOCS-PREVIEW-RELIABILITY-001.2:** The landing build receives only a token whose job permissions are restricted to read-only repository contents.
- **AC-UI-PUBLISHED-DOCS-PREVIEW-RELIABILITY-001.3:** Publishing or updating the preview link remains available through a separate job with issue and pull-request write permissions.
- **AC-UI-PUBLISHED-DOCS-PREVIEW-RELIABILITY-001.4:** The workflow retains its existing same-repository pull request gate, Cloudflare deployment behavior, stable alias, and preview comment format.
- **AC-UI-PUBLISHED-DOCS-PREVIEW-RELIABILITY-001.5:** A repository test fails if the build loses authenticated GitHub API access or regains issue or pull-request write permissions.
- **AC-UI-PUBLISHED-DOCS-PREVIEW-RELIABILITY-001.6:** **GIVEN** a same-repository pull request changes public documentation, **WHEN** the landing preview build requests public Kandev repository data, **THEN** the request uses the job-scoped `GITHUB_TOKEN` under read-only contents permissions.
- **AC-UI-PUBLISHED-DOCS-PREVIEW-RELIABILITY-001.7:** **GIVEN** a successful configured Cloudflare preview deployment, **WHEN** the deployment exports its URL and stable alias, **THEN** a separate write-capable job creates or updates the pull request preview comment.
- **AC-UI-PUBLISHED-DOCS-PREVIEW-RELIABILITY-001.8:** **GIVEN** the preview build executes pull request documentation, **WHEN** that process inspects its job token, **THEN** the token has no issue or pull-request write permission.

## Migrated source detail

## Why

Pull request documentation previews intermittently fail when the landing build
cannot fetch live public GitHub community data. Contributors need preview
results to depend on the proposed documentation and publisher contract, not on
the shared unauthenticated GitHub API quota of a hosted runner.

## What

- Same-repository pull request previews authenticate landing's GitHub API
  reads with the workflow job's short-lived `GITHUB_TOKEN`.
- The landing build receives only a token whose job permissions are restricted
  to read-only repository contents.
- Publishing or updating the preview link remains available through a separate
  job with issue and pull-request write permissions.
- The workflow retains its existing same-repository pull request gate,
  Cloudflare deployment behavior, stable alias, and preview comment format.
- A repository test fails if the build loses authenticated GitHub API access or
  regains issue or pull-request write permissions.

## Permissions

- The preview build and Cloudflare deployment job has `contents: read` and no
  issue or pull-request write permission. Its `GITHUB_TOKEN` is exposed only to
  the landing build step for authenticated public GitHub API reads.
- The preview-link publication job has `contents: read`, `issues: write`, and
  `pull-requests: write`. It consumes only the deployment URLs exported by the
  successful preview job and does not check out or build pull request content.
- Fork pull requests remain excluded from the preview path.

## Failure modes

- A failed landing build or Cloudflare deployment fails the preview job and
  prevents preview-link publication.
- Missing Cloudflare configuration retains the existing warning-and-skip
  behavior and does not start the publication job.
- A missing deployment URL fails the publication job rather than publishing an
  incomplete comment.
- GitHub API failure after authenticated access remains a real build failure;
  the workflow does not publish stale or fallback community data as a valid
  preview.

## Scenarios

- **GIVEN** a same-repository pull request changes public documentation,
  **WHEN** the landing preview build requests public Kandev repository data,
  **THEN** the request uses the job-scoped `GITHUB_TOKEN` under read-only
  contents permissions.
- **GIVEN** a successful configured Cloudflare preview deployment, **WHEN** the
  deployment exports its URL and stable alias, **THEN** a separate write-capable
  job creates or updates the pull request preview comment.
- **GIVEN** the preview build executes pull request documentation, **WHEN** that
  process inspects its job token, **THEN** the token has no issue or pull-request
  write permission.
- **GIVEN** Cloudflare preview deployment is not configured or the preview job
  fails, **WHEN** downstream jobs are evaluated, **THEN** no preview-link
  publication job runs.

## Out of scope

- Changing landing's community-data fallback or artifact validation logic.
- Supporting preview deployments for fork pull requests.
- Changing Cloudflare credentials, project configuration, deployment aliases,
  or the production rebuild hook.
