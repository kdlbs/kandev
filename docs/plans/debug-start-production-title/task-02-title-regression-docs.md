---
id: "02-title-regression-docs"
title: "Prove debug title behavior"
status: done
wave: 2
depends_on: ["01-profile-selector-contract"]
plan: "plan.md"
spec: "../../specs/platform/requirements/dev-preview-title-prefixes.md"
---

# Task 02: Prove debug title behavior

## Acceptance

- The title-prefix E2E test asserts `Debug Kandev` for a pprof/debug restart
  without development or e2e profile selectors.
- The existing `Dev Kandev` and `Preview Kandev` assertions continue to pass.
- Public and internal configuration docs explain the different `make dev` and
  `make start-debug` title behavior.
- The viewport-independent metadata behavior has an explicit mobile-parity
  justification. No touch or layout surface changes.

## Verification

- `cd apps/web && pnpm e2e:run --project chromium tests/system/title-prefix.spec.ts`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- `git diff --check`

If the fresh worktree has no frontend dependencies, first run
`cd apps && pnpm install --frozen-lockfile`.

## Files likely touched

- `apps/web/e2e/tests/system/title-prefix.spec.ts`
- `docs/public/configuration.md`
- `docs/public/cli.md`
- `docs/configuration.md`

## Dependencies

Task 01 must pass before this task starts.

## Parallelism

Sequential. The browser assertion depends on the profile-selector behavior.

## Inputs

- The title scenarios in `docs/specs/platform/requirements/dev-preview-title-prefixes.md`.
- The mobile-parity conclusion in `plan.md`.
- Existing fixture restart behavior in `apps/web/e2e/fixtures/backend.ts`.

## Output contract

Report the changed files, exact E2E and documentation checks, cleanup status,
and the updated task and plan statuses in the same conversation.

## Results

- `cd apps/web && pnpm e2e:run --project chromium tests/system/title-prefix.spec.ts`
  — passed (3 tests: Debug, Dev, and Preview titles).
- `node --test scripts/validate-public-docs.test.mjs` — passed (58 tests).
- `node scripts/validate-public-docs.mjs` — passed (41 published docs pages).
- `cd apps && pnpm exec prettier --check web/e2e/tests/system/title-prefix.spec.ts`
  — passed.
- `git diff --check` — passed.
