---
id: "03-prove-browser-flows"
title: "Prove browser flows"
status: done
wave: 3
depends_on: ["02-build-responsive-explorer"]
plan: "plan.md"
spec: "../../specs/platform/requirements/mcp-session-observability.md"
---

# Task 03: Prove Browser Flows

## Acceptance

- Desktop Playwright coverage opens the wide dialog, selects Kandev, and reads
  at least one enabled tool description.
- Desktop coverage selects a third-party server and reads the catalog limit.
- Mobile coverage proves the full-height list-to-detail flow, Back behavior,
  internal scrolling, 44px controls, and no document overflow.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/chat/mcp-status.spec.ts -- --project=chromium
cd apps/web && pnpm e2e:run tests/chat/mobile-mcp-status.spec.ts -- --project=mobile-chrome
```

Write the failing Playwright assertions against a fresh production build. Use
the mock agent's real Kandev `tools/list` request for catalog evidence.

## Files likely touched

- `apps/web/e2e/tests/chat/mcp-status.spec.ts`
- `apps/web/e2e/tests/chat/mobile-mcp-status.spec.ts`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/pages/session-page.ts`

## Dependencies

Task 02 completes the responsive explorer.

## Parallelism

Parallel-safe with Task 04 after Task 02. This task owns only E2E files and
their direct helpers.

## Inputs

- Spec explorer scenarios.
- E2E causal-wait and production-build rules.
- The mobile `mobile-chrome` Pixel 5 project.

## Output contract

Report each initial failure, final command result, screenshots or traces,
geometry assertions, files changed, blockers, and risks. Update this task and
the plan status in the same session.

## Results

Implemented desktop and mobile browser coverage with the real mock-agent
`tools/list` request. The first desktop run exposed an overly broad status-text
locator, so the assertion was scoped to the detail pane. The first mobile run
exposed the shared drawer's default 80% viewport limit. The explorer now
overrides that limit for the phone full-height surface.

Final verification:

- `pnpm e2e:run tests/chat/mcp-status.spec.ts -- --project=chromium`: 1 passed.
- `pnpm e2e:run tests/chat/mobile-mcp-status.spec.ts -- --project=mobile-chrome`:
  1 passed.
- Desktop coverage opens the explorer, reads a Kandev tool description, and
  explains the unavailable third-party catalog.
- Mobile coverage proves a 44px trigger and Back control, a 90%+ viewport
  drawer, internal detail scrolling, and no document horizontal overflow.

Fresh, compressed, and visually inspected PR assets are available at
`apps/web/.pr-assets/mcp-status--desktop-mcp-explorer-kandev.png` and
`apps/web/.pr-assets/mobile-mcp-status--mobile-mcp-explorer-kandev.png`.
