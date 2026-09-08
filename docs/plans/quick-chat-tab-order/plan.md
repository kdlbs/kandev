---
created: 2026-08-26
status: completed
requirements:
  - REQ-UI-QUICK-TERMINAL-002
  - REQ-TASKS-QUICK-CHAT-AGENT-TITLES-001
system_design:
  - ../../specs/ui/system-design/quick-terminal.md
  - ../../specs/tasks/system-design/quick-chat-agent-titles.md
legacy_specs: []
---

# Implementation Plan: Quick Chat Tab Experience

## Overview

This package gives Quick Chat a stable tab order, responsive reorder controls, clear title editing,
and agent-generated conversation titles. The title lifecycle and order persistence form the first
wave because later UI work uses their stable state.

The implementation then adds sortable interactions and edit polish. Desktop and mobile Playwright
coverage proves the combined user outcome in the final wave.

## Scope

### In scope

- Persist one mixed conversation and terminal tab order for each user and workspace.
- Support mouse, touch, keyboard, and coarse-pointer reorder actions.
- Improve spinner spacing and the conversation-title editor.
- Give an ordinary Quick Chat owner the normal title instruction and `set_task_title_kandev` tool.
- Show the accepted agent title in the current tab and after reload.
- Update the public Quick Chat and MCP guidance for the title behavior.

### Out of scope

- Quick Terminal title editing.
- Agent-generated titles for Configuration Chat.
- Live drag synchronization between clients.
- Changes to the Quick Chat dialog geometry or content scroll owner.
- Split panes and command-palette tab actions.

## Technical approach

### Order persistence

Add `quick_chat_tab_order_by_workspace` to backend user settings. Use namespaced conversation and
terminal references. Resolve saved references against authoritative membership and stable baseline
sorts. Apply moves optimistically through a serialized settings save queue.

### Agent-generated titles

Send the current `agent_generated_task_titles` preference in the ordinary Quick Chat create request.
Mark an enabled task as title-pending while keeping its current label as the provisional title.

Reuse the existing owner claim before eager launch. The executor then starts the owner with the
title-capable MCP profile. When the pending owner receives a prompt, add canonical Kandev context.
Keep the existing task update and Quick Chat synchronization paths.

### Tab interactions

Build one resolved descriptor list before rendering. Use dnd-kit mouse, touch, and keyboard sensors.
Add visible coarse-pointer actions for rename, directional moves, and close.

### Editing polish

Add a fixed spinner-to-title gap. Replace the tab close action with Save and Cancel during rename.
Use one commit path and one restore path.

## Tests

- `quick_chat_sessions_test.go` covers the stable conversation baseline.
- User settings and frontend state tests cover `AC-UI-QUICK-TERMINAL-002.1`, `.4`, and `.5`.
- Quick Chat component tests cover `AC-UI-QUICK-TERMINAL-002.2`, `.3`, `.6`, `.7`, and `.8`.
- Quick Chat handler tests cover `AC-TASKS-QUICK-CHAT-AGENT-TITLES-001.1` and `.4`.
- Message, orchestrator, executor, and MCP tests cover title criteria `.2`, `.5`, `.6`, and `.7`.
- Quick Chat task-event tests cover title criterion `.3`.
- Public-doc validation covers the Quick Chat guide and the MCP title reference.

## E2E tests

- `apps/web/e2e/tests/chat/quick-chat.spec.ts` proves mixed-tab reorder, reload persistence, clear
  rename controls, and the accepted agent title. It covers both requirement sets.
- `apps/web/e2e/tests/chat/mobile-quick-chat-tabs.spec.ts` runs in `mobile-chrome`. It proves touch or
  menu reorder, rename discovery, target size, overflow containment, and reload persistence.

## Work orders

Wave 1 establishes independent state contracts:

- [x] [Task 01: Persist tab order](task-01-persist-tab-order.md)
- [x] [Task 05: Enable Quick Chat agent titles](task-05-enable-agent-titles.md)

Wave 2 changes the shared tab item in sequence:

- [x] [Task 02: Add sortable tab interactions](task-02-add-sortable-tab-strip.md)
- [x] [Task 03: Improve tab editing](task-03-improve-tab-editing.md)

Wave 3 proves the integrated result:

- [x] [Task 04: Prove desktop and mobile behavior](task-04-prove-tab-order-e2e.md)

Tasks 01 and 05 are parallel-safe. Task 02 depends on Task 01. Task 03 depends on Task 02. Task 04
depends on all implementation work orders.

## Verification results

Passed backend unit tests for settings, tasks, handlers, orchestration, MCP, and the mock agent.
Passed the web typecheck, lint, i18n checks and ratchet, focused Quick Chat tests (28 files, 235
tests), public-doc validation, and specification lint. The complete desktop Quick Chat suite passed
17 tests. The mobile Quick Chat suite passed its mixed-tab, rename, target-size, and overflow test.

## Risks

- A saved order can contain deleted or duplicate references. The resolver must ignore them without
  changing membership.
- Touch drag can compete with horizontal scrolling. Delayed activation and visible move actions
  provide both outcomes.
- The Quick Chat agent starts before its first prompt. The title-capable profile and prompt context
  must agree before Kandev dispatches that prompt.
- The title instruction repeats while title state remains pending. A successful agent or user title
  update must stop later injection.
- New action labels affect every required locale catalog.
