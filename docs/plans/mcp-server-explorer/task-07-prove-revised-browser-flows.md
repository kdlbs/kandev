---
id: "07-prove-revised-browser-flows"
title: "Prove the revised browser flows"
status: done
wave: 3
depends_on: ["06-refine-explorer-ux"]
plan: "plan.md"
spec: "../../specs/platform/requirements/mcp-session-observability.md"
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

Implemented the revised desktop and mobile flows in the two planned E2E
specifications. The first desktop run failed because the old test expected a
description paragraph in each tool row. The compact rows intentionally omit
descriptions. A later run exposed a duplicate hidden Radix tooltip node, so the
test now reads the visible tooltip portal. The first mobile geometry check
sampled Vaul during its entry transition. It now polls until the drawer settles
and still requires the final bottom edge to stay inside the viewport.

Final verification passed:

```text
cd apps/web && pnpm e2e:run tests/chat/mcp-status.spec.ts -- --project=chromium
1 passed

cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run tests/chat/mobile-mcp-status.spec.ts -- --project=mobile-chrome
1 passed

cd apps/web && pnpm exec eslint e2e/tests/chat/mcp-status.spec.ts e2e/tests/chat/mobile-mcp-status.spec.ts
passed

cd apps/web && pnpm run e2e:sleep-ratchet
passed
```

The desktop test confirms one close control, the green Active Kandev status,
an independently scrollable tool list, a fixed summary header, token values,
tool arguments, focus return, scroll restoration, and third-party limits. The
mobile test confirms the servers-to-tools-to-tool path, both Back actions,
controls of at least 44px, a full-height settled drawer, internal tool-list
overflow, and no document overflow.

Fresh inspected captures:

- `mcp-status--desktop-mcp-explorer-tools.png`
- `mcp-status--desktop-mcp-explorer-tool-detail.png`
- `mobile-mcp-status--mobile-mcp-explorer-tool-detail.png`

No blockers remain. The tests depend on stable accessible names and MCP test
fixtures, which is intentional browser-level contract coverage.
