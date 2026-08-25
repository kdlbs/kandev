---
id: "01-authenticate-and-isolate-preview"
title: "Authenticate and isolate docs preview publication"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/published-docs-preview-reliability.md"
---

# Task 01: Authenticate and Isolate Docs Preview Publication

## Acceptance

- The landing build receives `${{ secrets.GITHUB_TOKEN }}` while its job has
  only `contents: read`.
- Preview deployment URLs flow to a separate comment-publication job with issue
  and pull-request write permissions, and that job never checks out or builds
  pull request content.
- A checked-in regression test enforces both permission boundaries and runs in
  the public-doc validation job.

## Verification

```bash
node --test scripts/notify-docs-workflow.test.mjs
node --test scripts/validate-public-docs.test.mjs scripts/notify-docs-workflow.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `.github/workflows/notify-docs.yml`
- `scripts/notify-docs-workflow.test.mjs`

## Dependencies

None.

## Parallelism

`sequential`; the test and workflow share one security contract.

## Inputs

- Repair spec scenarios and permission boundary.
- Failing job `91236751922`, which reported
  `[github-auth] configured=false` and rejected unavailable community data.
- Passing job `91230452438`, which used the same unauthenticated path with more
  starting quota.
- Landing `scripts/build-pages.mjs`, `lib/github-community.ts`, and
  `packages/site/src/github-api.ts` contracts on `kdlbs/landing` `main`.

## Output contract

Report the RED workflow-contract failure, minimal workflow repair, files
changed, exact verification results, residual risks, and update this task plus
`plan.md` status in the same conversation.

## Result

- RED coverage failed against the original workflow because the preview job
  still had issue/pull-request write permissions, the build had no
  `GITHUB_TOKEN`, and no isolated publication job existed.
- The preview job now uses `contents: read`, passes `GITHUB_TOKEN` only to the
  landing build, and exports deployment URLs to a separate write-capable
  publication job.
- Verified with `node --test scripts/notify-docs-workflow.test.mjs` (2 tests),
  `node --test scripts/validate-public-docs.test.mjs scripts/notify-docs-workflow.test.mjs`
  (60 tests), `node scripts/validate-public-docs.mjs` (41 pages),
  `git diff --check`, and a PyYAML parse of the workflow.
