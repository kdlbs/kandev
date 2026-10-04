---
id: "08-cover-mcp-user-journeys"
title: "Cover MCP user journeys"
status: done
wave: 6
depends_on:
  - "07-add-scoped-mcp-selectors"
plan: "plan.md"
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-001
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-002
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-003
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-005
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
acceptance_criteria:
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.1
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.5
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.1
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.2
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.3
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.8
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.9
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.11
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.1
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.2
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.3
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.7
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.1
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.2
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.5
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.6
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.7
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.1
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.2
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.3
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.5
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.6
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.7
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.8
system_design:
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
---

# Task 08: Cover MCP User Journeys

## Summary

Add desktop and mobile Playwright coverage for marketplace setup, catalog
management, additive scope composition, and idle-session application states.
Use the real browser contract with deterministic backend fixtures.

## In scope

- Add deterministic Registry cache and MCP catalog fixture controls to the E2E
  backend where existing test hooks cannot express the required states.
- Cover custom definition creation, marketplace search/review, stale cache,
  selection impact, and guarded delete in desktop Chromium.
- Cover remote setup without installation, managed npm first-use wording, and
  an unsupported package choice.
- Cover profile, repository, task, and new-session selections plus deduplicated
  origins in a created task.
- Cover active-turn pending state, idle applied state, unsupported next-start
  deferral, and failed reconnect retry.
- Add focused phone tests for direct setup navigation, bottom sheets, touch
  targets, safe area, one scroll owner, keyboard labels, and horizontal
  overflow.
- Assert that repository MCP settings start collapsed and task MCP settings stay
  inside the closed Advanced settings section.
- Keep tests event-driven and free of fixed sleeps.

## Out of scope

- Re-testing Registry pagination or resolver matrices already covered by Go
  tests.
- Visual snapshot baselines.
- Production feature changes beyond small testability hooks.

## Acceptance

- Desktop journeys prove catalog-to-runtime behavior through public UI and
  persisted API state.
- Mobile journeys prove capability parity with native interaction patterns and
  no page-level horizontal overflow.
- Reconnect state tests distinguish requested, applied, deferred, and failed
  outcomes without relying on capability advertisement alone.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm e2e:run tests/settings/workspace-mcp-configuration.spec.ts
cd apps/web && pnpm e2e:run tests/settings/mobile-workspace-mcp-configuration.spec.ts
cd apps/web && pnpm e2e:run tests/task/task-mcp-selection.spec.ts
cd apps/web && pnpm e2e:run tests/task/mobile-task-mcp-selection.spec.ts
cd apps/web && pnpm run e2e:sleep-ratchet
```

Create the failing desktop task-union journey first. It must fail before
implementation because the task payload has no selection and origin contract.

## Files likely touched

- `apps/web/e2e/tests/settings/workspace-mcp-configuration.spec.ts`
- `apps/web/e2e/tests/settings/mobile-workspace-mcp-configuration.spec.ts`
- `apps/web/e2e/tests/task/task-mcp-selection.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-mcp-selection.spec.ts`
- `apps/web/e2e/fixtures/backend.ts`
- `apps/backend/internal/e2e/` test-support files, if the existing mock hooks
  cannot express Registry and ACP reconnect outcomes

## Dependencies

- Task 07 completes every user-facing surface and applied-state projection.

## Risks

- Registry and ACP outcomes must be deterministic. Do not call the public
  Registry or real provider CLIs from browser tests.
- Mobile assertions must verify the phone interaction, not only reuse a desktop
  locator at a smaller viewport.
- Keep setup and task flows focused enough to diagnose a failed state boundary.

## Parallelism

`sequential`

## Inputs

- Requirement sections 001, 002, 003, 005, and 006.
- System-design sections `Frontend surfaces`, `Idle-session reconfiguration`,
  `Responsive behavior`, and `Test strategy`.
- Existing settings, task-create, and mobile E2E fixture patterns.

## Results

- Added deterministic desktop and mobile browser coverage for custom catalog setup, curated marketplace review/install, degraded marketplace state, selection impact, and guarded deletion.
- Added desktop additive profile/repository/task/session coverage with deduplicated origins and applied-state verification.
- Added mobile Advanced-settings disclosure, bottom-sheet selection, touch-target, viewport, overflow, and idle-session coverage.
- Refreshed backend and plugin E2E artifacts and kept the new tests free of fixed sleeps.
- Verification passed: 4 desktop settings tests, 2 mobile settings tests, 1 desktop task test, 2 mobile task tests, and E2E sleep-ratchet.
