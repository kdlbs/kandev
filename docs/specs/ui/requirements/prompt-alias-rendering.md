---
status: draft
system: ui
created: 2026-09-02
owners:
  - kandev
---

# Prompt Alias Rendering Requirements

## Overview

Saved prompts can be referenced in user messages with an `@name` alias. The
transcript already presents recognized aliases as prompt chips, but the same
message content is rendered differently in the anchored last-prompt bar and
Prompt history. Consistent presentation lets users identify referenced prompt
content regardless of which transcript surface they are reading.

## Terminology

- **Saved prompt alias:** An `@name` token recognized by the existing prompt
  mention matcher and corresponding to a saved prompt in the prompt store.
- **Prompt chip:** The transcript's visual representation of a recognized saved
  prompt alias, including its hover preview when content is available.

## Requirements

### REQ-UI-PROMPT-ALIAS-001: Consistent saved prompt alias presentation

**Intent:** Show recognized saved prompt aliases consistently across transcript
surfaces without changing the message content sent to agents.

**User story:** As a task reader, I want saved prompt aliases to look the same
in the transcript, pinned last-prompt bar, and Prompt history, so that I can
recognize referenced prompts while reviewing any surface.

#### Acceptance criteria

- **AC-UI-PROMPT-ALIAS-001.1:** When a user message contains an alias matching a
  saved prompt, the transcript, anchored last-prompt bar, and Prompt history
  shall render that alias as the same prompt chip, including its saved-prompt
  name metadata and hover preview when the saved prompt has content.
- **AC-UI-PROMPT-ALIAS-001.2:** When a user message contains an unrecognized
  `@` token, each surface shall leave it as ordinary text using the existing
  prompt-name matching rules; rendering shall not invent a chip for an unknown
  name.
- **AC-UI-PROMPT-ALIAS-001.3:** Alias chips shall continue to render inside the
  existing Markdown structures supported by the transcript renderer. Rich
  Markdown code spans and link destinations shall remain ordinary rendered
  content, while aliases in link labels may use the chip's visual treatment
  without creating nested interactive controls. The pinned and history surfaces
  shall preserve their current compact, expandable, and scrollable behavior.
- **AC-UI-PROMPT-ALIAS-001.4:** Updating the saved prompt collection shall update
  alias chip recognition and hover content in mounted pinned or history views;
  the fix shall not alter persisted message text, prompt expansion semantics, or
  the raw-message view.
- **AC-UI-PROMPT-ALIAS-001.5:** The presentation shall remain available on
  desktop and phone Prompt history surfaces, while preserving the existing
  desktop-only visibility rule for the anchored last-prompt bar.

## Out of scope

- Changing prompt alias parsing, matching, expansion depth, or agent delivery.
- Changing saved prompt persistence, prompt CRUD, or message APIs.
- Adding prompt numbers, navigation behavior, or new Markdown features.
- Rendering aliases in passthrough, comments, plans, or unrelated editors.
