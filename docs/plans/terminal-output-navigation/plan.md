---
created: 2026-10-28
status: implemented
requirements:
  - REQ-UI-TERMINAL-OUTPUT-NAVIGATION-001
system_design:
  - ../../specs/ui/system-design/terminal-output-navigation.md
legacy_specs: []
---

# Implementation Plan: Terminal Output Navigation

## Overview

Add one explicit jump to latest output to task shell and dev-server xterm surfaces. One vertical work order covers tracking, scoped presentation, localization, documentation, and real desktop and phone evidence. Implementation starts only after a later explicit release and the ROOT-managed HEAVY lease.

## Scope

In scope: task-shell scrollback on desktop and phone, and existing dev-server output on desktop; accessible action, normal-buffer semantics, live output continuity, localized copy and focused tests. Out of scope: agent TUI, Quick Terminal, login, new phone dev-server routing, forced following, and changes to transport, persistence, buffering or process lifecycle.

## Technical approach

Share xterm viewport tracking and button presentation across `PassthroughTerminal` and read-only dev-server `ShellTerminal`, gated by the actual task host and readiness/overlay state. Use current xterm buffer and public events, not a parallel scroll model. Preserve search and existing touch handlers. Scope task callers explicitly if mode alone includes other surfaces. Reuse localization and controls; include seven locale catalogs and generate Traditional Chinese with `i18n:zh-hant`. Update the integrated-terminal how-to in `docs/public/developer-tools.md` with when the action appears and what it does.

## UI composition

On desktop, the action sits inside the task shell or dev-server terminal near its lower trailing edge while the user reads earlier output. On phone it sits within the task shell, above the existing key row, clear of safe areas and keyboard, with a 44px minimum touch target. Xterm remains the scroll owner. The action is absent at bottom, in the alternate buffer, or behind search or status overlays. Compare rendered desktop and phone results against AC-UI-TERMINAL-OUTPUT-NAVIGATION-001.1 through .5. Keyboard activation must work without sending input or unconditionally redirecting focus.

## Tests

`components/task/terminal-output-navigation.test.tsx` renders the existing terminal hosts with controlled xterm state and proves the missing action behavior in RED before implementation. A missing module or symbol is a setup failure, not RED. GREEN covers readiness, bottom/above-bottom transitions, scroll, output, resize, buffer switches, activation, focus and cleanup, including no input sends or content mutation. Eligibility/overlay tests cover both hosts and excluded agent/utility surfaces. Test real streaming continuity in browser, not only mocked events.

## E2E tests

`e2e/tests/terminal/terminal-jump-to-latest.spec.ts` (`chromium`) drives a real task shell and real dev-server process through the task route. `e2e/tests/terminal/mobile-terminal-jump-to-latest.spec.ts` (`mobile-chrome`) drives the phone task shell through its actual navigation and `MobileTerminalPane`. Each produces more than a viewport of output, scrolls using the real viewport, checks visibility, generates more output while reading, and activates the real control. Check actual xterm viewport and content, no extra input, and phone geometry/document scroll. Use isolated fixture data and processes with cleanup; no route fulfillment or synthetic surface in place of the flow.

## Work orders

- [x] [Task 01: Jump to latest output](task-01-jump-to-latest.md)

## Verification results

The host RED failed on the absent dev-server action, and the final host tests passed (4 tests). Desktop task-shell and phone-shell xterm flows passed separately; the gated desktop dev-server navigation flow passed after its actual initial snapshot and later output were synchronized. The phone capture was inspected for target placement and the final screenshots match the tested build. Scoped ESLint and i18n checks passed. See the [work order](task-01-jump-to-latest.md#results) for commands, logs and fixture limitations.

## Risks

- Xterm event ordering, alternate-buffer transitions, output eviction and resize can make visibility stale; use public events and prove real behavior.
- A shared renderer has excluded callers. A mode-only gate may expose the control outside task surfaces.
- Phone browser emulation cannot establish every physical keyboard and safe-area configuration; inspect the rendered phone composition and preserve existing bounds.
