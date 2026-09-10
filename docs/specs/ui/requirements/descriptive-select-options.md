---
status: active
system: ui
created: 2026-09-04
owners:
  - kandev
---

# Descriptive Select Options Requirements

## Overview

Some selectors need supporting text for each choice, but that text must not
turn the selected value into a long, non-wrapping label. The UI system owns the
reusable presentation contract that separates a concise option name from its
description while preserving the selector's existing value and behavior.

The first covered consumer is the Kandev MCP strategy selector used when a
user creates or configures a custom TUI agent.

## Terminology

- **Option name:** The concise text that identifies a selectable value and is
  shown in the closed trigger after selection.
- **Option description:** Supporting text shown with an option while the list
  is open.

## Requirements

### REQ-UI-DESCRIPTIVE-SELECT-OPTIONS-001: Compact descriptive selector entries

**Intent:** Let users compare described choices without allowing explanatory
text to distort the selector or its containing surface.

**User story:** As a user choosing a Kandev MCP strategy for a custom TUI
agent, I want each strategy name and mechanism to have separate visual roles,
so that I can scan the choices without the dialog overflowing.

#### Acceptance criteria

- **AC-UI-DESCRIPTIVE-SELECT-OPTIONS-001.1:** When the custom TUI agent MCP
  strategy selector is open, each backend-supplied strategy shall show its
  strategy name on the first row and its mechanism description on the second
  row.
- **AC-UI-DESCRIPTIVE-SELECT-OPTIONS-001.2:** When a described strategy is
  selected, the closed trigger shall show only its concise strategy name and
  shall not copy the mechanism description into the trigger.
- **AC-UI-DESCRIPTIVE-SELECT-OPTIONS-001.3:** At supported desktop and phone
  widths, the selector, its open option list, and the containing dialog shall
  remain within the viewport without document-level horizontal overflow.
- **AC-UI-DESCRIPTIVE-SELECT-OPTIONS-001.4:** Option descriptions shall remain
  programmatically associated with their option names, and strategy selection,
  keyboard behavior, dismissal, and submitted values shall remain unchanged.
- **AC-UI-DESCRIPTIVE-SELECT-OPTIONS-001.5:** On coarse-pointer viewports, the
  selector trigger and option rows shall provide touch targets at least 44 CSS
  pixels high while fine-pointer desktop controls retain their existing
  density.

## Out of scope

- Changing the available MCP strategy keys, descriptions, ordering, or backend
  API.
- Changing custom TUI agent creation, persistence, or launch behavior.
- Rewriting the concise existing Off option or other selector copy.
- Changing every select control that does not present per-option descriptions.
