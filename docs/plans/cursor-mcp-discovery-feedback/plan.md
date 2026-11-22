---
created: 2026-11-20
status: done
requirements:
  - REQ-AGENTS-MCP-PREP-002
system_design:
  - ../../specs/agents/system-design/agent-mcp-preparation.md
legacy_specs: []
---

# Implementation Plan: Cursor MCP discovery feedback

## Overview

Identify Cursor MCP discovery in the task setup panel and explain failed
discovery on desktop and phone. Agents owns this outcome because the reason
describes agent-native MCP preparation. The task conversation shows that result.
[GitHub issue #5407](https://github.com/kdlbs/kandev/issues/5407)

## Evidence and root cause

On `de63958fc71`, two temporary Playwright probes seeded a waiting session with
a completed preparation result. The steps contained a Cursor discovery
`timeout` without a server ID and successful verification of
`kandev-diagnostics`. Both Chromium and mobile Chrome showed the error
summary, **Discover servers**, and the successful verification row. The
discovery row had zero **Details** buttons before and after reload.

`agentMcpStepLabel` selects a generic discovery label. In `StepRow`, the
generic command/output expander excludes every agent MCP step.
`StepRowActions` delegates failed MCP rows to `AgentMcpPrepareActions`, which
requires a server ID. Cursor reports inventory errors before it can identify a
server. The backend and UI state already carry the Cursor provider and recorded
reason. [Discovery producer](../../../apps/backend/internal/agent/runtime/lifecycle/cursor_plugin_mcp.go)
· [Row renderer](../../../apps/web/components/session/prepare-progress.tsx)
· [Server actions](../../../apps/web/components/task/agent-mcp-prepare-actions.tsx)

## Scope

Task transcript labels, localized details for recorded discovery reasons,
desktop and phone interaction, and the Cursor preparation section of the public
agent guide. The package consumes the existing inventory, preparation events,
saved results, and server recovery contracts.

## Technical approach

Make two changes in the existing preparation row.

1. Match the recorded provider and step kind to label the row **Discover Cursor
   MCP servers**. Use the existing server suffix for rows with a server ID.
2. Add an inline **Details** disclosure for failed Cursor discovery. Use the
   shared `Collapsible` and a lookup from the recorded failure code to a
   translation. Use general discovery copy for missing or unknown codes.

Live events and saved results already reach the same renderer. Keep the
disclosure in one adjacent component to stay within the renderer's lint budget.
The [system design](../../specs/agents/system-design/agent-mcp-preparation.md#cursor-discovery-feedback)
lists the supported reasons.

## ASCII UI preview

**UI-01** · Task conversation, expanded environment setup. Covers
AC-AGENTS-MCP-PREP-002.8, AC-AGENTS-MCP-PREP-002.9 and AC-AGENTS-MCP-PREP-002.10.

```text
Current desktop and phone
  Environment setup finished with errors
    [x] Discover servers
    [ok] Verify connection: kandev-diagnostics

Desktop, details collapsed
  Environment setup finished with errors
    [x] Discover Cursor MCP servers
        [Details v]
    [ok] Verify connection: kandev-diagnostics

Desktop, details expanded
  Environment setup finished with errors
    [x] Discover Cursor MCP servers
        [Details ^]
        Checking enabled Cursor Marketplace plugins timed out (5-second limit).
        Kandev skipped Marketplace plugin imports for this setup attempt.
        Other configured MCP servers can still connect.
    [ok] Verify connection: kandev-diagnostics

Phone, details expanded
  Environment setup finished
  with errors
    [x] Discover Cursor MCP servers
        [ Details ^               ]
        Checking enabled Cursor
        Marketplace plugins timed
        out (5-second limit).
        Kandev skipped Marketplace
        plugin imports for this
        setup attempt. Other
        configured MCP servers
        can still connect.
    [ok] Verify connection:
         kandev-diagnostics
```

The phone conversation supplies the scroll owner and safe-area handling.
The inline text follows the failed MCP command details in
`AgentMcpPrepareActions`. One short explanation fits under its row in the
conversation. The same visible **Details** trigger is content-sized on desktop
and spans the row on phones. It uses the shared 28px desktop
size and meets the 44px minimum on phones and coarse-pointer devices. The
labels, hierarchy and operability define the preview. Wrapping and spacing
are illustrative.

## Tests

Run the existing preparation renderer and server recovery Vitest suites.

## E2E tests

- Chromium exercises the live timeout, keyboard disclosure, reason fallback,
  and both rows of the saved result after reload. AC-AGENTS-MCP-PREP-002.8
  through AC-AGENTS-MCP-PREP-002.10.
- Mobile Chrome exercises the same saved result with a tap, checks the 44px
  target, and checks that the copy fits. AC-AGENTS-MCP-PREP-002.9 and
  AC-AGENTS-MCP-PREP-002.10.

The required red test is **explains a Cursor MCP discovery timeout during setup
and after reload**. Before implementation the provider label assertion fails.
The fixture also proves the disclosure is missing on current source.

## Work orders

- [x] [Task 01: Explain Cursor MCP discovery failures](task-01-discovery-feedback.md)

Task 01 is the only work order. It owns the user-visible change and its tests.

## Verification results

- Saved-result reproduction passed in Chromium (1 test in 9.3s) and mobile
  Chrome on Pixel 5 (1 test in 7.3s). Each checked first load and reload.
- A Chromium WebSocket probe exercised progress, completion, and reload with
  the same outcome (1 test in 5.6s). It used injected preparation events and
  stored fixture data.
- All 34 existing frontend checks passed across the four preparation suites
  used during investigation. The specification catalog, 36 linter tests,
  specification lint, and documentation coverage preflight passed. The
  preflight also accepted the projected renderer changes against this package.

The three investigation browser probes and the implementation regressions used
isolated Playwright fixtures with the current Go backend and Vite build. Task 01
is complete.

Implementation verification passed:

- The desktop preparation spec passed all 3 tests, including the live failure
  updates, keyboard disclosure, fallback reasons, and saved-result reload.
- The Pixel 5 preparation spec passed all 3 tests, including disclosure tap,
  44px target size, and wrapping.
- The focused Vitest suites passed 38 tests. Typecheck, Oxlint, i18n checks and
  ratchet, public-doc validation, specification validation and lint, coverage,
  and `git diff --check` passed.

## Risks

- The timeout code covers both credential access and the service request. The
  message describes the lookup as a whole.
- The generic `unavailable` code can also describe a local discovery failure.
  A Marketplace-specific effect needs a known inventory reason.
- Both existing recovery browser specs assert the old generic label for a
  successful Cursor discovery row. Update those assertions with the renderer.
