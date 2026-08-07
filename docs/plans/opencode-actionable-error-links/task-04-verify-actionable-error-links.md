---
id: "04-verify-actionable-error-links"
title: "Verify actionable error links"
status: done
wave: 4
depends_on: ["02-persist-recovery-link", "03-render-recovery-link"]
plan: "plan.md"
spec: "../../specs/agent-stall-recovery/spec.md"
---

# Task 04: Verify Actionable Error Links

## Acceptance

- Backend tests prove the observed OpenCode route is preserved only as
  `remediation_url`, and all rejected URL classes are omitted.
- Seeded desktop E2E proves the recovery link, short sanitized message, and
  existing recovery actions; the no-link short-ACP fallback is covered.
- Seeded `mobile-chrome` E2E proves keyboard/touch reachability, 44px controls,
  bounded details, and no horizontal overflow.
- Tests never invoke a real quota failure, read a developer's OpenCode logs, or
  add a production-only test hook.

## Verification

```text
cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/api ./internal/agent/runtime/lifecycle ./internal/orchestrator -run 'Test(OpenCode|ProviderError|RecoverableFailure|RecoveryStatus)' -count=1
cd apps/web && pnpm e2e:run --no-build tests/session/provider-remediation-link.spec.ts
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/session/mobile-provider-remediation-link.spec.ts
cd apps/web && pnpm e2e:run --no-build tests/office/provider-remediation-link.spec.ts
```

Inspect the phone viewport in addition to assertions and record exact results
in the plan's Verification Results section.

## Files likely touched

- `apps/web/e2e/tests/session/provider-remediation-link.spec.ts`
- `apps/web/e2e/tests/session/mobile-provider-remediation-link.spec.ts`
- existing E2E page/fixture helpers only if a reusable seeded metadata path is
  required
- backend lifecycle/orchestrator regression tests

## Dependencies and risks

Tasks 02 and 03. Seed the safe contract with a non-sensitive test workspace
identifier; assert no invalid URL reaches the DOM. A seeded UI test proves
rendering, not OpenCode upstream emission, so preserve the explicit external
dependency in the final report.

## Results

Implemented 2026-08-07.

- Desktop `tests/session/provider-remediation-link.spec.ts` (3 tests): link
  href/target/rel + keyboard focus, sanitized message and technical details
  stay URL/identifier-free, existing recovery actions intact, no horizontal
  overflow; short-error fallback renders no link; persistent
  last-agent-error notice renders the link.
- Mobile `tests/session/mobile-provider-remediation-link.spec.ts`
  (mobile-chrome): 44px touch target, focus reachability, URL-free details,
  in-viewport actions, no horizontal overflow.
- Office `tests/office/provider-remediation-link.spec.ts` (2 tests):
  RunErrorEntry renders the link from seeded `last_agent_error` metadata;
  invalid or absent metadata renders no link.
- All seeds inject only safe `remediation_url` metadata; no real provider,
  quota exhaustion, or OpenCode log access.

Verification: `cd apps/backend && go test ... -run 'Test(OpenCode|ProviderError|RecoverableFailure|RecoveryStatus)' -count=1` — passed; desktop + mobile-chrome + office E2E — all passed (run via `pnpm e2e:run --no-build`).

External dependency preserved: OpenCode must emit `action_url` for the live
1.18.5 failure to show its TUI URL; Kandev shows the short error until then.
