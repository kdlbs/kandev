---
created: 2026-11-16
status: implemented
requirements:
  - REQ-UI-AGENT-TASK-REFERENCE-001
system_design:
  - ../../specs/ui/system-design/agent-task-references.md
legacy_specs: []
---

# Implementation plan: Agent task references

## Overview

Deliver one vertical slice: recognized agent Markdown links become task chips,
direct navigation works across devices, and agent guidance teaches the format.
The user selected direct opening and authorized implementation. The renderer,
guidance, translations, documentation, and focused checks are complete.

## Scope

In scope: task conversation replies and agent-plan bodies, explicit Markdown
links, existing chip styling, optional desktop disclosure, mobile navigation,
task/Office prompt guidance, localization, and focused public documentation.

Out of scope: new tools, URI schemes, persistence, live metadata fetches,
composer changes, terminal decoration, and previews that require confirmation.

## Technical approach

The [system design](../../specs/ui/system-design/agent-task-references.md)
owns the exact format and rendering contract. Add a pure route recognizer and
a stable Markdown anchor map at the agent-message boundary. Reuse
`TaskMentionChip`, `TaskLink`, the Tooltip primitive, and loaded task selectors.
Keep existing user-mention semantics and file-link fallback intact.

Update `kandev-context.md` and `office-context.md` only after the renderer exists
within the same work order. No provider adapter changes are needed. All agents
whose output reaches `AgentMessageContent` or `AgentPlanMessage` receive the
same rendering. Task/Office prompts teach the format. Terminal-only output and
other renderers keep ordinary text. Test deterministic mock-agent Markdown
through the real persisted-message path, not live model compliance.

This package complements the completed
[user-mention package](../chat-task-mentions/plan.md). Keep its work order done
and rerun its relevant regression suites if shared chip code changes.

## UI preview

These descriptions define structure, not exact pixel values. The conversation
preview uses Kandev rich output as requested, rather than an ASCII mockup.

**UI-01: Desktop agent reply, normal state.** The sentence reads, "The remaining
work is tracked in" followed by one tinted chip containing a task icon and
"Fix reconnect handling". The chip ends within the sentence. Click or Enter
opens the task. The chip follows the task's personal color, or the primary accent when none is assigned. Hover or focus shows the full
title above cached status and session-count badges. Labeled rows show creation
date, workflow, and parent when available. The canonical cache includes
sidebar-owned tasks. The chat remains
the scroll owner.

**UI-02: Phone agent reply, long title.** The same sentence is followed by a
single wrapping chip. Its full title can occupy multiple lines. Each chip has
at least 44px active height and stays inside the message. A single tap opens
the task. There is no tooltip or drawer. Back returns to the source task.

**UI-03: Missing metadata and fallback.** A valid reference still shows its
authored title. Desktop disclosure says to open the task for details when no
metadata is loaded. Malformed references remain ordinary Markdown. An
unavailable destination uses the existing task-route error surface.

UI-01 covers AC .1, .2, .3, and .5. UI-02 covers AC .2 and .4. UI-03 covers
AC .3, .5, and .6. The work order owns rendered evidence for these views.

## Tests

- AC .1, .5, .6: new `lib/tasks/agent-task-reference.test.ts`, cases
  `accepts exact task links` and `rejects lookalike destinations`.
- AC .1-.6: new
  `components/task/chat/messages/agent-task-reference-components.test.tsx`,
  cases `renders authored title`, `delegates file links`, `keeps raw and copy
  source`, `handles streaming boundaries`, and `discloses loaded metadata`.
  Exercise actual agent reply and agent-plan composition, not only the chip.
- AC .7: new backend `internal/sysprompt/task_references_test.go`,
  `TestTaskReferenceGuidance`, including task and Office formatting with and
  without built-in package bindings.
- Existing user-mention and shared Markdown suites guard unchanged contracts.

## E2E tests

- New `e2e/tests/chat/agent-task-references.spec.ts`, project `chromium`:
  scripted mock-agent reply with a real target, hover/focus/Escape, direct
  navigation without main-frame reload, browser Back, refresh, rename,
  duplicate titles, unavailable destination, plan-body integration, and raw view.
  Cover AC .1, .2, .3, .5, .6, and .7.
- New `e2e/tests/chat/mobile-agent-task-references.spec.ts`, project
  `mobile-chrome`: long and adjacent titles, 44px measured hit areas, containment,
  no document overflow, exact target, and Back. Cover AC .1, .2, and .4.
- Include a phone-width fine-pointer case in the desktop suite. Inspect light,
  dark, and phone screenshots. Reuse the scripted-message pattern in
  `markdown-math.spec.ts` and causal backend waits.

## Work orders

- [x] [Task 01: Render agent task references](task-01-render-agent-task-references.md)

One sequential work order includes implementation, tests, guidance, and docs.
No delegation is authorized.

## Verification results

Final affected checks: 54 frontend tests passed. Backend prompt tests, typecheck,
lint, and translation checks passed. Browser checks: four desktop and two
phone tests passed. Earlier implementation checks remain in the work order.
See the [work-order results](task-01-render-agent-task-references.md#results).

## Risks

- File-link recognition can intercept `/t/id` unless task recognition runs first.
- A tooltip trigger can break anchor behavior if it replaces or nests controls.
- Cached metadata can be missing or old. It must remain optional and must not
  alter the authored label or destination.
- Agent output can ignore guidance. Unrecognized output retains normal Markdown.
