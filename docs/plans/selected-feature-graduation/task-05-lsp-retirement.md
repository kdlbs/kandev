---
id: selected-feature-graduation-05
title: Retire LSP continuity flag
status: done
wave: 5
depends_on:
  - selected-feature-graduation-02
plan: plan.md
requirements:
  - REQ-PLATFORM-LSP-CONTINUITY-GRADUATION-001
acceptance_criteria:
  - AC-PLATFORM-LSP-CONTINUITY-GRADUATION-001.2
  - AC-PLATFORM-LSP-CONTINUITY-GRADUATION-001.3
  - AC-PLATFORM-LSP-CONTINUITY-GRADUATION-001.4
  - AC-PLATFORM-LSP-CONTINUITY-GRADUATION-001.5
system_design:
  - ../../specs/platform/system-design/lsp-continuity-graduation.md
---

# Retire LSP continuity flag

## Summary

After a stable default-on release, make the bounded LSP lease path the only
browser LSP lifecycle and retire the old release-toggle identity.

## In scope

- Remove the flag argument and browser-owned branch from gateway composition,
  LSP WebSocket handling, and frontend LSP client behavior. Keep auto-start,
  executor, authorization, capacity, and phone-viewer boundaries.
- Remove the live profile/config/registry/frontend identity and append the
  exact key/environment pair to the retired registry; leave stored overrides.
- Update developer-tools, WebSocket, configuration, and feature-status pages
  to describe unconditional lease continuity when LSP is started.

## Out of scope

- Changing the one-hour lease deadline, LSP resource limit, or phone editor.

## Acceptance

1. A supported language server always uses the lease path, including after an
   old false environment value or SQLite override is supplied.
2. The former flag is absent from active metadata, `/api/v1/features`, profiles,
   and frontend feature names; its exact identity is permanently retired.
3. Desktop, tablet, and phone behavior retains the pre-retirement lifecycle and
   resource bounds.

## ASCII UI preview

`UI-03` from [the plan](plan.md#ascii-ui-preview):

```text
Desktop toolbar: Go [~] Reconnecting ... [Stop] -> [Ready]
Tablet drawer:  Language server | Reconnecting ... | [Stop]
Phone viewer:   < Back | main.go | file content (no LSP control)
```

The surfaces do not change shape. The focused rendered check verifies that a
normal production-profile launch reaches them with no feature override.

## Verification

```sh
cd apps/backend && go test ./internal/gateway/websocket ./internal/orchestrator ./internal/runtimeflags ./internal/common/config ./internal/profiles
cd apps && pnpm --filter @kandev/web exec vitest run lib/state/slices/features/features-contract.test.ts lib/lsp
make -C apps/backend lint
cd apps/web && pnpm run typecheck && pnpm run lint
git diff --check
```

After rebuilding backend and web E2E artifacts:

```sh
cd apps/web && pnpm e2e:run --project=chromium tests/lsp/lsp-file-intelligence.spec.ts
cd apps/web && pnpm e2e:run --project=mobile-chrome tests/lsp/mobile-lsp-file-intelligence.spec.ts
cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project=containers tests/docker/lsp-file-intelligence.spec.ts
```

## Files likely touched

- `apps/backend/internal/backendapp/main.go`,
  `apps/backend/internal/gateway/websocket/lsp_handler.go` and its lease tests.
- `apps/web/hooks/use-lsp.ts`, LSP client/manager tests, feature slice/types.
- `apps/backend/internal/runtimeflags/registry.go`,
  `apps/backend/internal/common/config/config.go`, profile YAML and contracts.
- `docs/public/developer-tools.md`, `docs/public/websocket-api.md`,
  `docs/public/configuration.md`, `docs/public/feature-status.md`.

## Dependencies and risks

Task 02 shipped in Stable v0.97.0. Nine Stable releases used the default-on
lease path through v0.103.1. The release and capacity checks below cover the
resource bounds that previously had an operator off switch.

## Results

The gateway and browser always use the bounded lease lifecycle for supported
LSP connections. The old flag identity is retired. The runtime flags API,
feature API, profile defaults, and frontend flag map omit it. Existing false
override rows remain stored and have no effect.

- Registry tests used a saved SQLite false override and an explicit false
  environment value. The desktop E2E test reloaded with the retired
  environment value and reattached to the same task-host process.
- Gateway tests passed for detached expiry, capacity, and an unavailable
  agentctl client. Frontend tests covered Stop during editor initialization and
  confirmed lease release on editor idle.
- Playwright passed 18 desktop, 4 phone and tablet, and 3 Local Docker
  scenarios with retries disabled. Desktop coverage exercised browser close,
  idle reaping, capacity eviction, and backend restart.
- Go registry, config, profile, gateway, backend composition, and orchestrator
  suites passed. The shared frontend run passed 160 tests. Changed Go lint,
  frontend typecheck and lint, and documentation checks passed.
