---
status: active
system: agents
created: 2026-10-13
owners:
  - kandev
---

# Image file preview defaults

## Overview

Image file cards in native agent rich output should reveal their content without
an extra click. Agents owns this contract because task and Office transcripts
consume the same agent rich-output presentation.

This requirement narrowly amends the file-disclosure behavior in
[agent rich output](agent-rich-output.md). Its criteria take
precedence over AC-AGENTS-AGENT-RICH-OUTPUT-001.9 and .14 and
AC-AGENTS-AGENT-RICH-OUTPUT-002.4, .6 and .11 only for automatic image-preview
expansion and the resulting session-authorized workspace read. Other guarantees,
including target isolation and absence of agent callbacks, remain applicable.

## Requirements

### REQ-AGENTS-IMAGE-PREVIEW-DEFAULTS-001: Initially expanded image cards

#### Acceptance criteria

- **AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.1:** When a completed native rich-output image file card has no retained disclosure choice, it shall start expanded and request its preview without user interaction, on desktop and phone. This shall apply to new compositions and valid historical presentations.
- **AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.2:** Non-image file cards without a retained disclosure choice shall start collapsed and request no preview until explicitly expanded.
- **AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.3:** Users shall be able to hide and reopen an image preview. Same-target rerenders, metadata changes, viewport changes, and retained transcript remounts shall preserve that choice. Separate cards shall remain independent, including cards referencing the same image.
- **AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.4:** A target replacement shall discard the old content and pending read results and apply the new target's retained choice or image/non-image default. Automatic reads shall use the current session's authorization and exact repository and path; neither previewing nor opening a file shall mutate task state or call an agent action.
- **AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.5:** An automatically expanded preview shall show its existing loading or unavailable state when appropriate. A failed read shall not automatically retry on unrelated updates. Hiding and explicitly reopening shall retry, while reopening a successful preview shall reuse its content during the card's lifetime.
- **AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.6:** Phone images shall fit within their inline card without document horizontal overflow. Hide/reopen and file-open actions shall remain visible, keyboard operable, and touch operable with at least 44px targets. The transcript shall remain the primary vertical scroll owner.

## Out of scope

- Expansion of text, unknown-type, or non-image binary files by default.
- Changes to accepted image formats, encoding, SVG rendering, or the file viewer.
- New authoring properties, schema versions, preferences, permanent disclosure
  storage, artifact retention, feature flags, or backend authorization rules.
- Attachment thumbnails, file-tree previews, or generic provider image results.

## Implementation plan

- [Image preview defaults](../../../plans/image-preview-defaults/plan.md)
