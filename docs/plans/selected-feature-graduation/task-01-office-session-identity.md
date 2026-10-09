---
id: selected-feature-graduation-01
title: Retire Office session identity flag
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-OFFICE-IDENTITY-GRADUATION-001
  - REQ-OFFICE-IDENTITY-GRADUATION-002
  - REQ-OFFICE-IDENTITY-GRADUATION-003
  - REQ-OFFICE-IDENTITY-GRADUATION-004
acceptance_criteria:
  - AC-OFFICE-IDENTITY-GRADUATION-004.5
  - AC-OFFICE-IDENTITY-GRADUATION-004.6
  - AC-OFFICE-IDENTITY-GRADUATION-004.7
  - AC-OFFICE-IDENTITY-GRADUATION-004.8
  - AC-OFFICE-IDENTITY-GRADUATION-004.9
  - AC-OFFICE-IDENTITY-GRADUATION-004.10
system_design:
  - ../../specs/office/system-design/session-identity-graduation.md
---

# Retire Office session identity flag

## Summary

Make the already-default-on participant-session behavior unconditional and
remove its live runtime flag. The default-on release was v0.94.0, so this is
the later retirement release required by Office's existing specification.

## In scope

- Remove `OfficeSessionIdentity` branching from orchestrator session binding
  and Office decision re-evaluation. Keep the participant, missing identity,
  non-Office, and duplicate-row behaviors in permanent tests.
- Remove its profile key, typed config and feature response field, registry
  registration, frontend default, runtime UI entry, and live catalog entry.
  Append the exact retired key and environment variable; keep old overrides.
- Update `docs/public/configuration.md` and any operator-facing status text.

## Out of scope

- Changing `features.office`, adding a unique index, migrating or deduplicating
  session rows, or changing the Tasks-owned fallback decision rule.

## Acceptance

1. Office reviewer and approver runs bind to their participant sessions and
   their decisions re-evaluate against the validated calling session without
   consulting the former flag.
2. A stale `features.officeSessionIdentity=false` row and explicit
   `KANDEV_FEATURES_OFFICE_SESSION_IDENTITY=false` are inert; active registry,
   profile, Feature Toggles, and `/api/v1/features` omit the identity.
3. Permanent tests cover duplicate live rows, no participant identity,
   non-Office tasks, and the supported transaction-guard behavior.

## Verification

```sh
cd apps/backend && go test ./internal/orchestrator ./internal/office/dashboard ./internal/runtimeflags ./internal/common/config ./internal/profiles
cd apps && pnpm --filter @kandev/web exec vitest run lib/state/slices/features/features-contract.test.ts
make -C apps/backend lint
cd apps/web && pnpm run typecheck
git diff --check
```

After rebuilding the backend and web E2E bundle, run:

```sh
cd apps/web && pnpm e2e:run --project=chromium tests/office/workflow-quorum-transitions.spec.ts
```

## Files likely touched

- `apps/backend/internal/orchestrator/task_operations.go`, service config and
  backend composition; `apps/backend/internal/office/dashboard/agent_decisions.go`.
- `apps/backend/internal/runtimeflags/registry.go`, `apps/backend/internal/common/config/config.go`,
  `apps/backend/internal/common/config/catalog.go`,
  `apps/backend/internal/profiles/profiles.yaml`.
- `apps/web/lib/state/slices/features/types.ts` and contract tests;
  `docs/public/configuration.md`.

## Dependencies and risks

No new code dependency. The old off path can deadlock multi-agent review; remove
it without weakening Office's session guard. Existing false overrides cease to
be rollback controls, so the release note must state that behavior.

## Results

Removed the live flag from backend composition, typed config, profiles,
runtime definitions, the feature response, frontend defaults, and the startup
catalog. The exact key and environment variable are now retired. Office
participant session binding and decision re-evaluation are unconditional.

Verification passed:

- `go test ./internal/orchestrator ./internal/office/dashboard ./internal/runtimeflags ./internal/profiles`
- `env -u KANDEV_INTERNAL_CONFIG_FILE -u KANDEV_SERVER_PORT go test ./internal/common/config -count=1`
- `pnpm --filter @kandev/web exec vitest run lib/state/slices/features/features-contract.test.ts`
- `make -C apps/backend lint`
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm e2e:run --project=chromium tests/office/workflow-quorum-transitions.spec.ts`
- `node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs`
- `git diff --check`
