---
status: active
system: agents
created: 2026-10-19
owners:
  - kandev
---

# Image file preview expansion

## Overview

Users can inspect a larger image by activating its inline file preview, as they
can with image attachments. Agents owns this behavior because native rich-output
file cards share one contract across task and Office transcripts.

This capability adds a temporary image dialog. The existing
[image preview defaults](image-preview-defaults.md) continue to govern inline
disclosure, loading, retries, and retained choices.

## Requirements

### REQ-AGENTS-IMAGE-PREVIEW-EXPANSION-001: Open a larger image

#### Acceptance criteria

- **AC-AGENTS-IMAGE-PREVIEW-EXPANSION-001.1:** Activating a ready image file preview shall open the same image in a larger, viewport-fitted attachment-style dialog. This shall work in current and historical rich-output presentations.
- **AC-AGENTS-IMAGE-PREVIEW-EXPANSION-001.2:** The image trigger shall support click, tap, Enter, and Space. Its accessible name shall identify the file. Closing the dialog shall return focus to the trigger when it still exists. Escape, the close control, and outside activation shall dismiss the dialog.
- **AC-AGENTS-IMAGE-PREVIEW-EXPANSION-001.3:** Opening and closing the dialog shall preserve inline disclosure and shall not open the file editor, repeat the workspace read, or invoke an agent action.
- **AC-AGENTS-IMAGE-PREVIEW-EXPANSION-001.4:** Replacing the file target or removing its card shall dismiss its dialog. The dialog shall not retain another session's, repository's, or file's image.
- **AC-AGENTS-IMAGE-PREVIEW-EXPANSION-001.5:** On phones, the image and dialog shall fit the viewport without document horizontal overflow. Image triggers and close controls shall have at least 44px touch targets. The image shall retain its aspect ratio. Dismissal shall restore transcript interaction.

- **AC-AGENTS-IMAGE-PREVIEW-EXPANSION-001.6:** Appearance settings shall provide an accessible, default-on Show image previews by default switch. Save shall persist the choice on this device across reloads. Reset shall discard unsaved changes. With this option off, image cards without a retained disclosure choice shall start collapsed and perform no preview read until opened. Manual disclosure choices shall take precedence. Users shall always be able to enlarge a visible image. The control shall work on desktop and phone.

## Out of scope

- File-tree/editor image viewers, attachment redesign, and images inside Markdown.
- New image formats, SVG rendering changes, pinch zoom, pan, downloads, or galleries.
- Changes to file authorization, retention or rich-output schemas.
- New expansion actions for loading, unavailable, text, or non-image binary previews.

## Implementation plans

- [Image preview expansion](../../../plans/image-preview-expansion/plan.md)
