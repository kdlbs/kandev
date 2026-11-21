---
status: active
system: ui
created: 2026-11-16
owners:
  - kandev
---

# Markdown default Preview requirements

## Overview

Users can choose the initial display mode for Markdown files. UI owns this reusable editor presentation preference.
The existing Preview remains the renderer. The preference belongs in Settings > Preferences > Terminal & Editors, in the Editors section.

## Requirements

### REQ-UI-MARKDOWN-DEFAULT-PREVIEW-001: Persisted initial Markdown mode

**Intent:** Let users read newly opened Markdown files without selecting Preview each time.

#### Acceptance criteria

- **AC-UI-MARKDOWN-DEFAULT-PREVIEW-001.1:** The Editors section shall expose a labeled On/Off setting named “Open Markdown files in Preview”. New accounts and existing accounts without a stored value shall default to On.
- **AC-UI-MARKDOWN-DEFAULT-PREVIEW-001.2:** Save shall persist either value across reloads and runtime restarts through account settings. Reset shall discard an unsaved change. Failed saves shall preserve the draft and existing failure feedback.
- **AC-UI-MARKDOWN-DEFAULT-PREVIEW-001.3:** With On saved, an ordinary newly opened supported Markdown file shall start in the existing Preview on desktop and mobile. With Off saved, ordinary opening shall retain main's source-view behavior.
- **AC-UI-MARKDOWN-DEFAULT-PREVIEW-001.4:** Users shall retain normal mode switching. Changing this setting shall not switch an already open file or discard edits. Focusing an open tab shall preserve its current mode. Restoring saved tabs shall retain existing restore semantics. Closing and newly opening a file shall apply the current saved default.
- **AC-UI-MARKDOWN-DEFAULT-PREVIEW-001.5:** Explicit Preview actions shall continue to open Preview regardless of the preference. Non-Markdown files, binary files, HTML previews, diffs, and external editors shall retain their existing behavior.
- **AC-UI-MARKDOWN-DEFAULT-PREVIEW-001.6:** The setting shall have translated labels and descriptions in all supported languages. Keyboard and touch users shall operate it and Save/Reset. The phone layout shall wrap copy without horizontal page overflow and provide a control target of at least 44 pixels.

## Out of scope

New rendering or editing engines, changes from PR3113, Markdown editing on phones, and changing restored-tab persistence semantics are excluded.
