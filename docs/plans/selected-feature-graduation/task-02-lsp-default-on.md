---
id: selected-feature-graduation-02
title: Promote LSP browser continuity
status: done
wave: 2
depends_on: []
plan: plan.md
requirements:
  - REQ-PLATFORM-LSP-CONTINUITY-GRADUATION-001
acceptance_criteria:
  - AC-PLATFORM-LSP-CONTINUITY-GRADUATION-001.1
  - AC-PLATFORM-LSP-CONTINUITY-GRADUATION-001.3
  - AC-PLATFORM-LSP-CONTINUITY-GRADUATION-001.4
system_design:
  - ../../specs/platform/system-design/lsp-continuity-graduation.md
---

# Promote LSP browser continuity

## Summary

Ship the existing lease path on by default while preserving its working
restart-required kill switch for one stable release.

## In scope

- Set `KANDEV_FEATURES_LSP_BROWSER_CONTINUITY` to true in the canonical
  production, development, and E2E profiles. Retain the registry, backend and
  frontend disabled paths, and explicit override precedence.
- Prove reconnect, Stop, capacity eviction, one-hour expiry, idle release,
  task-host/backend shutdown, and a stale lease against a real task host.
- Update public feature status, developer tools, and WebSocket wording for the
  new default while the flag remains available.

## Out of scope

- Removing the flag, changing LSP auto-start/auto-install, or adding phone LSP.

## Acceptance

1. A normal production-profile launch uses lease continuity without an override;
   an explicit false override still selects the former browser-owned path.
2. Resource bounds and failure states behave as the existing LSP contract
   requires on Local PC and Local Docker.
3. Desktop/tablet reconnect and phone no-attachment outcomes pass their focused
   browser flows.

## ASCII UI preview

`UI-03` from [the plan](plan.md#ascii-ui-preview) preserves existing surfaces:

```text
Desktop toolbar: Go [~] Reconnecting ... [Stop] -> [Ready]
Tablet drawer:  Language server | Reconnecting ... | [Stop]
Phone viewer:   < Back | main.go | file content (no LSP control)
```

The structural states are required; copy and spacing are illustrative. The
tablet drawer keeps its touch action and internal scroll. The phone viewer
remains a separate, focused surface.

## Verification

```sh
cd apps/backend && go test ./internal/profiles ./internal/runtimeflags ./internal/gateway/websocket ./internal/orchestrator
cd apps && pnpm --filter @kandev/web exec vitest run lib/state/slices/features/features-contract.test.ts
make -C apps/backend lint
cd apps/web && pnpm run typecheck
git diff --check
```

After rebuilding backend and web E2E artifacts:

```sh
cd apps/web && pnpm e2e:run --project=chromium tests/lsp/lsp-file-intelligence.spec.ts
cd apps/web && pnpm e2e:run --project=mobile-chrome tests/lsp/mobile-lsp-file-intelligence.spec.ts
cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project=containers tests/docker/lsp-file-intelligence.spec.ts
```

## Files likely touched

- `apps/backend/internal/profiles/profiles.yaml` and profile/registry tests.
- `docs/public/feature-status.md`, `docs/public/developer-tools.md`,
  `docs/public/websocket-api.md`.

## Dependencies and risks

The [base implementation plan](../lsp-browser-continuity/plan.md) is marked
implemented. Promotion may be the first stable release containing this feature.
Default-on leases retain task-host resources after browser detachment; the
capacity and expiry checks are release gates. The flag remains operable here.

## Results

Promoted LSP browser continuity to the default-on path in the production,
development, and E2E profiles. The active runtime flag remains registered as a
restart-required kill switch, and explicit false continues to select the
browser-owned path.

The enabled-path verification exposed and fixed two crash regressions. The
agentctl bridge now observes direct process exit before process-group cleanup
and reports server exit separately from transport failure. The frontend stops
reacquiring a terminally failed lease until the user retries. Closing the last
editor also clears cached diagnostics for that document.

Verification passed: Go profile, runtime flag, gateway, orchestrator, agentctl
process, and agentctl API tests; backend lint and build; frontend typecheck,
Vite build, focused ESLint, and 32 focused Vitest tests; 62 public-doc tests and
validation of all 47 published pages; and `git diff --check`. Browser coverage
passed with 19 desktop tests, 4 phone/tablet tests, and 3 Local Docker tests.
