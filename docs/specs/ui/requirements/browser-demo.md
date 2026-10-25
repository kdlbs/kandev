---
status: active
system: ui
created: 2026-10-05
owners:
  - kandev
---

# Browser demo requirements

## Overview

The browser demo lets visitors explore the application without a backend, installed agents, or access to local repositories.

## Requirements

### REQ-UI-BROWSER-DEMO-001: Isolated interactive demo

**Intent:** Show realistic application behavior through browser-local data and simulated operations.

#### Acceptance criteria

- **AC-UI-BROWSER-DEMO-001.1:** The dedicated demo build installs its browser transport before application boot. Normal application routes keep their real backend transport.
- **AC-UI-BROWSER-DEMO-001.2:** Seeded tasks contain distinct message histories, tool calls, an approval request, a question, and populated plan content.
- **AC-UI-BROWSER-DEMO-001.3:** Multi-repository tasks expose both repositories in their workspace files. Changed-file data supports the Changes view.
- **AC-UI-BROWSER-DEMO-001.4:** New tasks simulate multiple tool calls, then stop in an idle state ready for review.
- **AC-UI-BROWSER-DEMO-001.5:** Workflow settings expose existing workflows, templates, editable steps, and simulated import and export operations.
- **AC-UI-BROWSER-DEMO-001.6:** Repository selection, integration settings, command previews, statistics, database data, and storage actions return demo-local responses.
- **AC-UI-BROWSER-DEMO-001.7:** Task selection navigates to the selected session. The demo does not open a task preview by default.
- **AC-UI-BROWSER-DEMO-001.8:** Release builds publish the browser demo archive and checksum within the compressed-size limit.
- **AC-UI-BROWSER-DEMO-001.9:** Follow-up messages retain conversation history and trigger a brief simulated thinking and tool turn, followed by a varied answer. Direct and queued sends return to idle review without duplicate turns on retry.

## Boundaries

The demo must not execute real shell commands, contact integration providers, or modify repositories on the visitor's computer.
The demo reuses application controls and responsive layouts. It does not introduce a separate application interface.

## Implementation records

- [Browser demo plan](../../../plans/browser-demo/plan.md)
- [Browser demo design](../system-design/browser-demo.md)
