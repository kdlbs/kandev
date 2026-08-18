---
id: "07-prove-revised-browser-flows"
title: "Prove the revised browser flows"
status: pending
wave: 3
depends_on: ["06-refine-explorer-ux"]
plan: "plan.md"
spec: "../../specs/mcp-session-observability/spec.md"
---

# Task 07: Prove the Revised Browser Flows

## Acceptance

- Desktop coverage reads the rich Kandev tooltip and finds one close control.
- A long desktop tool list scrolls inside the dialog while its header stays
  visible.
- Desktop coverage opens a tool and reads its description, token estimate, and
  one argument.
- Mobile coverage proves server-to-tools-to-tool navigation and both Back
  actions.
- Mobile controls are at least 44px. The drawer clears safe areas and does not
  add document overflow.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && cd web && pnpm e2e:run tests/chat/mcp-status.spec.ts -- --project=chromium && pnpm e2e:run tests/chat/mobile-mcp-status.spec.ts -- --project=mobile-chrome
```

Write the failing assertions against a fresh production build. Use the mock
agent's real Kandev `tools/list` request for catalog evidence.

## Files likely touched

- `apps/web/e2e/tests/chat/mcp-status.spec.ts`
- `apps/web/e2e/tests/chat/mobile-mcp-status.spec.ts`
- `apps/web/e2e/pages/session-page.ts`

## Dependencies

Task 06 completes the revised explorer.

## Parallelism

Parallel-safe with Task 08. This task owns E2E files and direct helpers.

## Inputs

- Spec explorer scenarios.
- E2E causal-wait and production-build rules.
- The `chromium` and `mobile-chrome` projects.

## Output contract

Report the initial failures, final commands, inspected screenshots, geometry
assertions, files, blockers, and risks. Update this task and the plan status in
the same session.

## Results

Pending.
