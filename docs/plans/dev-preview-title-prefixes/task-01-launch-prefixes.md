---
id: "01-launch-prefixes"
title: "Set environment title prefixes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/dev-preview-title-prefixes/spec.md"
---

# Task 01: Set environment title prefixes

## Acceptance

- `make dev` selects a backend profile that produces `Dev Kandev` by default,
  while normal production/start and e2e profiles remain unprefixed unless an
  explicit environment override is supplied.
- PR preview startup produces `Preview Kandev`, and the CLI supervisor retains
  `KANDEV_WEB_TITLE_PREFIX` when it writes and reuses its restart manifest.
- Focused browser, Go, TypeScript, and public-doc checks pass for the new
  launcher behavior.

## Verification

Run the focused unit checks:

```bash
cd apps/backend && go test -tags fts5 ./internal/profiles ./cmd/preview
cd ../ && pnpm --filter kandev test -- --run src/dev.test.ts src/supervisor/manifest.test.ts
```

Run the browser contract against the managed production E2E build:

```bash
cd apps/web && pnpm e2e:run --project chromium tests/system/title-prefix.spec.ts
```

Validate public documentation:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

On a fresh worktree, install workspace dependencies first with
`cd apps && pnpm install --frozen-lockfile`.

## Files likely touched

- `apps/backend/internal/profiles/profiles.yaml`
- `apps/backend/internal/profiles/profiles_test.go`
- `apps/backend/cmd/preview/sprite_ops.go`
- `apps/backend/cmd/preview/sprite_ops_test.go`
- `apps/cli/src/supervisor/manifest.ts`
- `apps/cli/src/supervisor/manifest.test.ts`
- `apps/web/e2e/tests/system/title-prefix.spec.ts`
- `docs/configuration.md`
- `docs/public/configuration.md`

## Dependencies

None.

## Parallelism

Sequential. The profile, preview script, supervisor allowlist, and browser test
all implement one environment-to-title contract, and the tests share the same
behavioral acceptance criteria.

## Inputs

- `docs/specs/dev-preview-title-prefixes/spec.md`
- `docs/plans/dev-preview-title-prefixes/plan.md`
- The merged PR #2459 `KANDEV_WEB_TITLE_PREFIX` contract.
- Existing profile precedence: explicit shell/launcher environment overrides
  profile defaults.
- Existing backend and web E2E fixtures.

## Output contract

Report the changed files, exact test commands and results, any E2E cleanup, and
whether the task and plan statuses were updated. Record blockers and remaining
risks explicitly.

## Results

Implemented the profile, preview launcher, and supervisor environment
contracts. Added browser coverage for both environment defaults and updated the
public and internal configuration references.

Verification completed:

- `cd apps/backend && go test -tags fts5 ./internal/profiles ./cmd/preview` —
  passed, 30 tests in 2 packages.
- `cd apps && pnpm --filter kandev test -- --run src/dev.test.ts src/supervisor/manifest.test.ts` —
  passed, 2 files and 6 tests.
- `cd apps/web && pnpm e2e:run --project chromium tests/system/title-prefix.spec.ts` —
  passed, 2 Playwright tests.
- `node --test scripts/validate-public-docs.test.mjs` — passed, 58 tests.
- `node scripts/validate-public-docs.mjs` — passed, 41 published pages.
- `pnpm exec prettier --check ...` and `git diff --check` — passed.

The managed E2E runner restored the fixture backend after each test; no
generated files are tracked in the worktree.
