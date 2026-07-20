---
id: "04-cross-platform-regression"
title: "Cross-platform repository regression coverage"
status: done
wave: 4
depends_on: ["01-explicit-path-contract", "02-identity-bound-git-operations", "03-frontend-validation-contract"]
plan: "plan.md"
spec: "../../specs/workspaces/local-repositories.md"
---

# Task 04: Cross-Platform Repository Regression Coverage

## Acceptance

- A Windows-native backend test covers drive-letter casing, separators, and a repository outside
  configured discovery roots, and the existing Windows CI job runs it.
- Desktop and mobile Playwright flows add and save a real repository outside automatic discovery
  roots.
- Public configuration documentation clearly limits discovery roots to automatic scanning.

## Verification

From `apps/backend`:

```bash
rtk go test -v ./internal/task/service -run 'Test.*ExplicitLocalRepository.*Windows|TestValidateLocalRepositoryPath'
```

From `apps/web` after rebuilding the production web/backend assets required by E2E:

```bash
rtk pnpm e2e -- e2e/tests/settings/repository-add-local.spec.ts --project=chromium
rtk pnpm e2e -- e2e/tests/settings/mobile-repository-add-local.spec.ts --project=mobile-chrome
```

## Files Likely Touched

- `apps/backend/internal/task/service/repository_discovery_windows_test.go`
- `.github/workflows/backend-tests.yml`
- `apps/web/e2e/tests/settings/repository-add-local.spec.ts`
- `apps/web/e2e/tests/settings/mobile-repository-add-local.spec.ts`
- `docs/public/configuration.md`
- `docs/configuration.md`
- `docs/specs/INDEX.md`
- `docs/decisions/INDEX.md`

## Dependencies

- Tasks 01-03 provide the integrated behavior under test.

## Inputs

- Spec scenarios for Windows native paths, discovery isolation, persistence, and invalid paths.
- Existing E2E backend HOME isolation and settings fixtures.
- Existing Windows backend workflow in `.github/workflows/backend-tests.yml`.

## Output Contract

Report cross-platform and browser evidence, docs changed, exact commands, skipped checks with reasons,
blockers, and update this task plus `plan.md` to done.
