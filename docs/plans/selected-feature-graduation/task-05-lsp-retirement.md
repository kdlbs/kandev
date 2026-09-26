---
id: selected-feature-graduation-05
title: Retire LSP continuity flag
status: pending
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

Requires Task 02 to have shipped in a stable release and its default-on
resource evidence to be recorded. Removing the former browser-owned path also
removes its emergency rollback. A retained lease can pin a task host until
its configured release condition, so expiry and capacity proof is required.

## Results

Pending.
