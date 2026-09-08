---
id: "01-protect-references"
title: "Protect secret references"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-REPOSITORY-SECRETS-001
acceptance_criteria:
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.9
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.10
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.11
system_design:
  - ../../specs/workspaces/system-design/repository-secrets.md
---

# Task 01: Protect secret references

## Summary

Reject unsafe secret deletion and explain how to repair an existing missing reference.

## In scope

- Service guards, HTTP/WS conflict and force contracts, reference discovery, runtime errors, and public documentation.

## Out of scope

- Automatic rebinding, scope transfers, concurrent save serialization, and worktree recovery.

## Acceptance

- Existing references block ordinary deletion. Force requires existing secret authorization.
- Lookup errors fail closed. Responses disclose no secret values or unauthorized repository details.
- Runtime errors identify the key and source and give a repair instruction.

## Verification

From `apps/backend`:

```bash
go test ./internal/secrets ./internal/agent/runtime/environment
go test ./internal/backendapp -run TestSecretReference -count=1
go test ./internal/agent/runtime/lifecycle -run 'TestSecretRecovery|TestResolveStrict' -count=1
```

From the repository root:

```bash
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
git diff --check
```

From `apps/web`:

```bash
pnpm exec vitest run components/settings/secrets-settings.test.ts components/settings/secret-delete-error.test.ts
pnpm exec eslint components/settings/secrets-settings.tsx components/settings/secret-delete-error.ts
pnpm run typecheck
pnpm run i18n:check
CAPTURE_PR_ASSETS=true pnpm e2e:run --host --project chromium e2e/tests/settings/secrets-delete.spec.ts
CAPTURE_PR_ASSETS=true pnpm e2e:run --host --project mobile-chrome e2e/tests/settings/mobile-secrets-delete.spec.ts
```

## Files likely touched

- `apps/backend/internal/secrets/service.go`, `handlers.go`, and new deletion tests/helpers.
- `apps/backend/internal/backendapp/main.go` and new reference checker/tests.
- `apps/backend/internal/agent/runtime/environment/environment.go` and tests.
- `apps/backend/internal/agent/runtime/lifecycle/environment_resolution.go` and tests.
- `docs/public/agents-and-profiles.md` and `docs/public/websocket-api.md`.
- `apps/web/components/settings/secrets-settings.tsx`, `secret-delete-error.ts`, their tests, and the settings locale catalogs.

## Dependencies

None.

## Risks

Cross-owner reference reads must fail closed. Internal store deletion remains available for lifecycle cleanup.

## Parallelism

`sequential`

## Inputs

- Issue #3500 and the linked requirements and system design.
- Existing scoped secret handlers and dynamic-profile conflict responses.

## Results

- `go test ./internal/secrets ./internal/agent/runtime/environment`: passed, 154 tests. Eight PostgreSQL tests were skipped because no test DSN was configured.
- `go test ./internal/backendapp -run TestSecretReference -count=1`: passed, nine tests. The integration test uses real encrypted SQLite storage and all three reference owners.
- `go test ./internal/agent/runtime/lifecycle -run 'TestSecretRecovery|TestResolveStrict' -count=1`: passed, three tests.
- The listed Vitest command passed all 13 tests. TypeScript, ESLint, and `i18n:check` passed.
- The managed Chromium and Pixel 5 E2E runs passed two tests each and produced validated desktop/mobile conflict-toast captures.
- Targeted `golangci-lint` over the four affected backend packages with `--new-from-rev=HEAD --timeout=5m` passed.
- Specification lint, public-documentation validation, and `git diff --check` passed.
- The existing desktop and mobile secret-deletion E2E specs now cover the structured conflict toast and retain the row after the failed delete. A fresh capture run will provide the PR screenshots.
- No live instance was changed or started. Temporary databases use test cleanup. Unrelated generated translation changes were removed.
- No agents were delegated. The issue assignment was verified. The implementation is committed in PR #3503; review fixup is in progress.
