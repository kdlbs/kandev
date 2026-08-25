---
spec: docs/specs/ui/requirements/published-docs-preview-reliability.md
created: 2026-07-31
status: implemented
---

# Implementation Plan: Published Docs Preview Reliability

## Overview

The cited preview failed because `.github/workflows/notify-docs.yml` launches
landing without `GITHUB_TOKEN`; the renderer therefore consumes the hosted
runner IP's 60-request unauthenticated quota and rejects any generated fallback
community data. The repair first adds a static regression test for the workflow
permission boundary, then authenticates the build with a read-only job token
and moves preview-comment publication to a separate write-capable job.

## Root Cause

- The preview build step exports `KANDEV_DOCS_SOURCE_PATH` but not
  `GITHUB_TOKEN`, confirmed by the failing log's
  `[github-auth] configured=false` diagnostic.
- Landing fetches repository metrics and contribution activity during static
  generation. A non-success response produces fallback markup that
  `build-pages.mjs` rejects.
- GitHub's unauthenticated REST quota is IP-scoped and limited to 60 requests
  per hour, so runner starting quota and concurrent consumption make the result
  intermittent.
- The existing preview job also owns issue and pull-request write permissions
  for its final comment step. Passing that job token directly to the build
  would make the preview reliable but violate least privilege.

## Workflow

### Static permission contract

- Add `scripts/notify-docs-workflow.test.mjs` to inspect the checked-in
  workflow and prove the preview build has an explicit token, the preview job
  is read-only, and preview-link publication is isolated in a write-capable job
  that does not build pull request content.
- Add the new test to the workflow's pull-request path filter and validator
  test command so future workflow changes keep exercising the contract.

### Authenticated preview build

- Restrict `jobs.preview.permissions` in
  `.github/workflows/notify-docs.yml` to `contents: read`.
- Export `GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}` only from the
  `Build preview from pull request docs` step. Landing already recognizes this
  environment variable through its existing `githubApiHeaders()` contract.
- Export the Cloudflare-enabled flag, deployment URL, and alias URL as preview
  job outputs.

### Isolated preview comment

- Move `Publish docs preview link` into a `publish-preview-link` job that needs
  `preview`, runs only when preview deployment is enabled, and owns
  `issues: write` plus `pull-requests: write`.
- Pass deployment outputs through `needs.preview.outputs` and retain the
  existing idempotent marker-based create-or-update script.

## Tests

- **What:** the landing build receives an explicit authenticated token while
  its job cannot write issues or pull requests.
  **File:** `scripts/notify-docs-workflow.test.mjs`.
  **How:** extract the checked-in preview job and named build step from the YAML
  text, then assert the token mapping and permission exclusions.
- **What:** preview comment publication retains write permissions without
  executing the landing build.
  **File:** `scripts/notify-docs-workflow.test.mjs`.
  **How:** extract the publication job and assert its dependency, output inputs,
  permissions, and absence of checkout/build steps.
- **What:** public-doc validation and the workflow contract remain green under
  the exact command used by CI.
  **File:** `.github/workflows/notify-docs.yml`.
  **How:** run both Node test files together, then run the public-doc validator.

## Implementation Waves And Parallel Candidates

Execution is sequential because the regression test and workflow repair share
the same permission and job-output contract.

- [x] [Task 01: Authenticate and isolate docs preview publication](task-01-authenticate-and-isolate-preview.md) — done

## Documentation Impact

The concise repair spec records CI behavior and permissions. No public CLI,
configuration, API, deployment, or UI documentation changes are required.

## Risks

- Job outputs must preserve the exact Cloudflare action output values across
  the new job boundary.
- The publication job must remain skipped when Cloudflare is unconfigured or
  preview fails.
- Exposing the token at workflow or job scope would unnecessarily widen its
  availability; the implementation must keep it scoped to the build step.
