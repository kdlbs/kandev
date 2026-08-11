---
id: app-status-bar-appearance-03
title: Retire App status bar runtime flag
status: todo
wave: 3
depends_on: [app-status-bar-appearance-02]
plan: docs/plans/app-status-bar-appearance-setting/plan.md
spec: docs/specs/ui/app-status-bar.md
decision: docs/decisions/2026-08-11-user-owned-status-bar-visibility.md
---

# Retire App status bar runtime flag

## Inputs

Task 02's complete user-setting gates, the runtime flag graduation checklist,
the append-only retired identity registry, and the backend/frontend feature-key
contract test.

## TDD sequence

1. Replace the registry inclusion test with failing assertions that
   `features.appStatusBar` is absent from active definitions and its exact key
   and env variable are retired.
2. Update feature/profile contract tests to expect no App status bar field or
   profile key.
3. Remove the active configuration and frontend feature fields.
4. Run focused Go and frontend contract tests before lint/typecheck.

## Implementation

- In `apps/backend/internal/runtimeflags/registry.go`:
  - remove the active App status bar registration;
  - append
    `{key: "features.appStatusBar", envVar: "KANDEV_FEATURES_APP_STATUS_BAR"}`
    to `retiredRuntimeFlagIdentities`;
  - preserve the generic no-reuse validation.
- In `apps/backend/internal/runtimeflags/registry_test.go`, replace metadata
  inclusion coverage with explicit active-exclusion and retired-identity
  coverage.
- Remove App status bar cases from
  `apps/backend/internal/runtimeflags/config_test.go`.
- Remove `FeaturesConfig.AppStatusBar` from
  `apps/backend/internal/common/config/config.go` and its env/default tests from
  `config_test.go`.
- Remove `KANDEV_FEATURES_APP_STATUS_BAR` from
  `apps/backend/internal/profiles/profiles.yaml` and all profile tests that
  require, apply, or derive `app_status_bar`.
- Remove `appStatusBar` from
  `apps/web/lib/state/slices/features/types.ts`.
- Update `apps/web/app/actions/features.test.ts` and any remaining feature
  fixtures. Keep
  `apps/web/lib/state/slices/features/features-contract.test.ts` green against
  the reduced backend JSON shape.
- Search active source and public runtime configuration for stale identities.
  Public docs are changed in Task 04; historical plans and the new retirement
  ADR may retain the names.

Do not delete or rewrite SQLite runtime override rows. Unknown keys are already
ignored, and retaining the retired identity prevents reinterpretation.

## Files likely touched

- `apps/backend/internal/runtimeflags/registry.go`
- `apps/backend/internal/runtimeflags/registry_test.go`
- `apps/backend/internal/runtimeflags/config_test.go`
- `apps/backend/internal/common/config/config.go`
- `apps/backend/internal/common/config/config_test.go`
- `apps/backend/internal/profiles/profiles.yaml`
- `apps/backend/internal/profiles/profiles_test.go`
- `apps/web/lib/state/slices/features/types.ts`
- `apps/web/lib/state/slices/features/features-contract.test.ts`
- `apps/web/app/actions/features.test.ts`

## Acceptance

1. `/api/v1/features` and frontend `FeatureName` have no `appStatusBar`.
2. Runtime definitions and Feature Toggles have no App status bar entry.
3. No shipped profile sets `KANDEV_FEATURES_APP_STATUS_BAR`.
4. The exact former key and environment variable exist in the append-only
   retired identity list and fail generic active-reuse checks.
5. Setting the retired environment variable or retaining an old override cannot
   affect config or user visibility.
6. Status surfaces remain controlled by the default-true user setting from
   Tasks 01 and 02.

## Verification

```sh
(cd apps/backend && go test ./internal/runtimeflags ./internal/common/config ./internal/profiles)
(cd apps && pnpm --filter @kandev/web exec vitest run lib/state/slices/features/features-contract.test.ts app/actions/features.test.ts)
make -C apps/backend lint
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
git diff --check
```

Also inspect:

```sh
rg -n "appStatusBar|features\.appStatusBar|KANDEV_FEATURES_APP_STATUS_BAR" apps/backend/internal apps/web profiles.yaml
```

Expected matches are limited to retired-identity assertions or unrelated
user-setting/order names. Product consumers, active config, and profiles must
have none.

## Dependencies

Task 02. Removing the flag earlier would make live surfaces lose their gate
before the portable preference is wired.

## Risks

- Deleting the identity instead of retiring it permits future stale override
  reinterpretation.
- Removing only the registry entry leaves the config field in
  `/api/v1/features`, which keeps frontend contracts and mental models stale.
- Editing the root `profiles.yaml` symlink and its canonical target as separate
  files can duplicate work. Change the canonical profile content once.
- Broad search results include historical plans and ADRs. Do not rewrite
  implementation history to make the search empty.

## Output contract

Report the retired identity proof, reduced API/frontend feature shape, profile
removal, search results, exact commands, and blockers. Mark this task done only
when no active runtime path can observe the former flag.
