---
id: "02-document-local-git-operations"
title: "Document local Git operations"
status: done
wave: 2
depends_on: ["01-resolve-local-base-targets"]
plan: "plan.md"
spec: "../../specs/workspaces/local-repositories.md"
---

# Task 02: Document local Git operations

## Intent

Update the public Git reference for local-only Merge and Rebase behavior. Keep remote precedence and failure behavior explicit.

## Inputs

- Spec: `docs/specs/workspaces/local-repositories.md`.
- Plan: `docs/plans/local-only-merge-rebase/plan.md`, Public Documentation.
- Existing reference page: `docs/public/git-operations.md`, Everyday operations and Troubleshooting.
- Primary Diátaxis type: reference.

## Acceptance

- The Rebase and Merge table rows describe both remote and local-only targets.
- Troubleshooting distinguishes a missing local branch from an `origin` fetch or authentication error.
- The page states that an existing `origin` keeps remote precedence and never falls back after a fetch error.

## Files likely touched

- `docs/public/git-operations.md`

## Verification

```shell
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Dependencies

Task 01.

## Parallelism

`sequential`. Document the implemented behavior after Task 01 establishes the final error text.

## Risks

- Do not imply that Pull, Push, or change-request creation work without a remote.
- Do not describe a local fallback after remote authentication or network failure.

## Output contract

Report the reference sections changed and both documentation command results. Update this task and `plan.md` in the same conversation.

## Results

Updated `docs/public/git-operations.md` to document the `origin` precedence
rule, local `refs/heads/<base>` targets, and the missing-local-base error. The
page also distinguishes remote fetch failures from local-only branch errors and
does not imply a local fallback after a failed `origin` fetch.

- `node --test scripts/validate-public-docs.test.mjs`: 61 passed.
- `node scripts/validate-public-docs.mjs`: 41 published docs pages validated.
