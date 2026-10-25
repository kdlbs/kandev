---
id: "04-prove-tab-order-e2e"
title: "Prove desktop and mobile behavior"
status: completed
wave: 3
depends_on:
  - "01-persist-tab-order"
  - "02-add-sortable-tab-strip"
  - "03-improve-tab-editing"
  - "05-enable-agent-titles"
plan: "plan.md"
requirements:
  - REQ-UI-QUICK-TERMINAL-002
  - REQ-TASKS-QUICK-CHAT-AGENT-TITLES-001
acceptance_criteria:
  - AC-UI-QUICK-TERMINAL-002.1
  - AC-UI-QUICK-TERMINAL-002.2
  - AC-UI-QUICK-TERMINAL-002.3
  - AC-UI-QUICK-TERMINAL-002.6
  - AC-UI-QUICK-TERMINAL-002.7
  - AC-UI-QUICK-TERMINAL-002.8
  - AC-TASKS-QUICK-CHAT-AGENT-TITLES-001.2
  - AC-TASKS-QUICK-CHAT-AGENT-TITLES-001.3
system_design:
  - ../../specs/ui/system-design/quick-terminal.md
  - ../../specs/tasks/system-design/quick-chat-agent-titles.md
---

# Task 04: Prove Desktop and Mobile Behavior

## Summary

Add production-build browser proof for the complete tab experience. Use a deterministic mock-agent
command to prove the real title MCP path and visible tab update.

## In scope

- Replace the desktop test that expects activity-based reorder after reload.
- Prove mixed-tab drag, rename controls, agent title update, and reload persistence.
- Prove the mobile move path, rename discovery, target size, and overflow containment.

## Out of scope

- Broad Playwright verification outside the Quick Chat files.
- Container and SSH projects.
- Visual snapshot baselines.

## Acceptance

- Desktop preserves mixed order and the generated title after reload.
- Mobile proves an equivalent move outcome with no document overflow and legal touch targets.
- Both focused projects pass with retries disabled and discover the intended tests.

## Verification

```bash
(cd apps/backend && go test ./cmd/mock-agent -count=1)
make -C apps/backend build
(cd apps/web && pnpm e2e:run tests/chat/quick-chat.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-quick-chat-tabs.spec.ts -- --retries=0)
```

## Files likely touched

- `apps/backend/cmd/mock-agent/handler.go`
- `apps/backend/cmd/mock-agent/main.go`
- `apps/backend/cmd/mock-agent/mock_agent_test.go`
- `apps/web/e2e/tests/chat/quick-chat.spec.ts`
- `apps/web/e2e/tests/chat/mobile-quick-chat-tabs.spec.ts`
- A shared Quick Chat E2E helper or page object

## Dependencies

Tasks 01, 02, 03, and 05.

## Risks

- The managed E2E runner uses the prebuilt mock-agent binary. The backend build must occur after a
  fixture change.
- Desktop and mobile require separate project commands.

## Parallelism

`sequential`

## Inputs

- Both system designs and their E2E mappings.
- The mock-agent `/subtask` MCP command as the nearest real-tool fixture pattern.

## Results

The deterministic mock-agent path now proves the real title MCP call and task event relabeling. The
complete desktop Quick Chat spec passed 17 tests with retries disabled. The mobile-chrome spec
passed its mixed-tab move, rename, target-size, and overflow scenario with retries disabled.
