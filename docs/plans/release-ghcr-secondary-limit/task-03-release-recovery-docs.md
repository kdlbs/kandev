---
id: "03-release-recovery-docs"
title: "Document release recovery"
status: done
wave: 3
depends_on: ["02-workflow-wiring"]
plan: "plan.md"
spec: "../../specs/release/requirements/release-ghcr-secondary-limit.md"
---

# Task 03: Document release recovery

## Acceptance

- The public release guide explains how to handle a GHCR secondary-rate-limit failure.
- The guide tells maintainers to rerun failed jobs after the throttle clears and not to start a new normal version bump after the signed tag exists.
- The guide points maintainers to `backfill_tag` for a recoverable partial release and retains the existing partial-release verification requirements.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/public/release-process.md`

## Dependencies

Task 02.

## Parallelism

sequential

## Inputs

- `docs/specs/release/requirements/release-ghcr-secondary-limit.md`, especially Why, What, Failure modes, and Out of scope.
- `docs/plans/release-ghcr-secondary-limit/plan.md`, Documentation section.
- Existing partial-release recovery guidance in `docs/public/release-process.md`.

## Output contract

Report the documentation change, exact validation results, docs-impact classification, and the updated task/plan status in the primary session.

## Results

Added GHCR secondary-rate-limit recovery guidance to `docs/public/release-process.md`, including bounded workflow retries, rerunning the failed job after the throttle clears, avoiding a new version bump after the signed tag exists, and using `backfill_tag` for a recoverable partial release.

- `node --test scripts/validate-public-docs.test.mjs` — passed, 60 tests.
- `node scripts/validate-public-docs.mjs` — passed, 41 published docs pages.
