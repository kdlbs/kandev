---
status: draft
system: ui
created: 2026-07-28
owners:
  - kandev
---
# Repair last-prompt transcript pinning Requirements

## Overview

Preserve the observable behavior documented for Repair last-prompt transcript pinning.

## Requirements

### REQ-UI-LAST-PROMPT-PINNING-REGRESSIONS-001: Repair last-prompt transcript pinning

**Intent:** Preserve the observable behavior documented for Repair last-prompt transcript pinning.

#### Acceptance criteria

- **AC-UI-LAST-PROMPT-PINNING-REGRESSIONS-001.1:** The anchored desktop bar appears only after the last prompt has scrolled **above** the transcript's visible content — i.e. the user continued scrolling down past it. It stays closed while the prompt sits fully **below** the viewport (not yet reached, e.g. while browsing earlier history above it), even though the prompt has no visible intersection with the viewport in that case too. The bar itself remains flush below the dockview view-tab selector.
- **AC-UI-LAST-PROMPT-PINNING-REGRESSIONS-001.2:** The always-on **Scroll to last prompt** control (in the chat status bar, and inside the anchored bar) uses the straight upward-arrow icon while the last prompt sits above the viewport, and flips to a downward-arrow icon while the prompt sits below the viewport instead — the icon always points the direction the transcript will actually scroll. Its action is unchanged: it always jumps to the top of the last prompt. **Scroll to start of transcript** uses the bar-to-up icon regardless of direction.
- **AC-UI-LAST-PROMPT-PINNING-REGRESSIONS-001.3:** The collapsed pinned prompt displays at most two rendered lines. Its expand control appears exactly when rendered prompt content is clipped; it is absent when all rendered content fits.
- **AC-UI-LAST-PROMPT-PINNING-REGRESSIONS-001.4:** The expanded pinned prompt remains internally scrollable and caps its height at 40% of the transcript panel's actual height (not a fixed pixel value), so it stays proportionate whether the panel is a tall full-screen view or a short embedded/split view.
- **AC-UI-LAST-PROMPT-PINNING-REGRESSIONS-001.5:** The pinned prompt preserves the user message's Markdown formatting, including inline code, block code, lists, headings, and tables. Its compact and expanded layouts may constrain height, but do not render Markdown source as plain text.
- **AC-UI-LAST-PROMPT-PINNING-REGRESSIONS-001.6:** On mobile, the pinned bar remains absent and the existing scroll-to-last-prompt action remains the discoverable, touch-accessible fallback.
- **AC-UI-LAST-PROMPT-PINNING-REGRESSIONS-001.7:** **Scroll to last prompt** and **Scroll to start of transcript** reliably land on their target even if the agent streams new content into the transcript while the scroll animation is still in progress; a streamed message never silently snaps the transcript back to the bottom mid-scroll.

## Migrated source detail

## Problem

The desktop anchored last-prompt bar currently appears once the prompt's top crosses the transcript scrollport, even while some of its content remains visible. The scroll action also uses a bent return-arrow rather than an upward arrow. The pinned prompt's expand action is decided by character count, so it can appear when no rendered content is hidden and disappear when wrapped content is clipped. Finally, the visibility check does not consider direction: scrolling *up* past the last prompt to browse earlier history also counts as "fully left the scrollport", incorrectly opening the anchored bar and leaving its up-arrow scroll control pointing the wrong way.

## Desired behavior

- The anchored desktop bar appears only after the last prompt has scrolled **above** the transcript's visible content — i.e. the user continued scrolling down past it. It stays closed while the prompt sits fully **below** the viewport (not yet reached, e.g. while browsing earlier history above it), even though the prompt has no visible intersection with the viewport in that case too. The bar itself remains flush below the dockview view-tab selector.
- The always-on **Scroll to last prompt** control (in the chat status bar, and inside the anchored bar) uses the straight upward-arrow icon while the last prompt sits above the viewport, and flips to a downward-arrow icon while the prompt sits below the viewport instead — the icon always points the direction the transcript will actually scroll. Its action is unchanged: it always jumps to the top of the last prompt. **Scroll to start of transcript** uses the bar-to-up icon regardless of direction.
- The collapsed pinned prompt displays at most two rendered lines. Its expand control appears exactly when rendered prompt content is clipped; it is absent when all rendered content fits.
- The expanded pinned prompt remains internally scrollable and caps its height at 40% of the transcript panel's actual height (not a fixed pixel value), so it stays proportionate whether the panel is a tall full-screen view or a short embedded/split view.
- The pinned prompt preserves the user message's Markdown formatting, including inline code, block code, lists, headings, and tables. Its compact and expanded layouts may constrain height, but do not render Markdown source as plain text.
- On mobile, the pinned bar remains absent and the existing scroll-to-last-prompt action remains the discoverable, touch-accessible fallback.
- **Scroll to last prompt** and **Scroll to start of transcript** reliably land on their target even if the agent streams new content into the transcript while the scroll animation is still in progress; a streamed message never silently snaps the transcript back to the bottom mid-scroll.

## Regression scenarios

### Desktop visibility threshold

**GIVEN** the last prompt is in a desktop transcript
**WHEN** any portion of it remains visible within the transcript scrollport
**THEN** the anchored prompt bar remains closed.

**WHEN** the prompt has fully left the scrollport above it (scrolled past, further down the transcript)
**THEN** the anchored prompt bar is open directly below the view-tab selector, and the scroll button uses the upward-arrow icon.

**WHEN** the prompt has fully left the scrollport below it (not yet reached, e.g. the user scrolled up to browse earlier history)
**THEN** the anchored prompt bar remains closed, and the scroll button — if enabled — uses the downward-arrow icon while still jumping to the last prompt on click.

### Prompt expansion

**GIVEN** a desktop transcript is scrolled past the last prompt
**WHEN** the collapsed pinned prompt has no clipped rendered content
**THEN** no expand control is shown.

**WHEN** the prompt wraps past two rendered lines
**THEN** an expand control is shown; expanding reveals a prompt capped at 40% of the transcript panel's height, scrollable internally, and proportionate to the panel's actual size rather than a fixed pixel height.

### Pinned prompt formatting

**GIVEN** the last user prompt contains Markdown
**WHEN** the pinned prompt is shown in either collapsed or expanded state
**THEN** its Markdown uses the same rich-text rendering as the original user-message bubble, without introducing a second copy action.

### Mobile fallback

**GIVEN** the anchored-prompt setting is enabled
**WHEN** the transcript is viewed on a phone viewport
**THEN** the pinned bar is not rendered and the upward scroll-to-last-prompt control is available.

### Streaming-scroll resilience

**GIVEN** a user clicks **Scroll to start of transcript** or **Scroll to last prompt**
**WHEN** the agent streams a new message into the transcript before the scroll animation settles
**THEN** the transcript still lands at the requested target instead of being snapped back to the bottom.

## Constraints

- Preserve the existing `show_anchored_prompt_bar` setting and desktop-only behavior.
- The native transcript renderer must use the directional (above/below/visible)
  threshold for the rendered prompt node.
- The repair does not change prompt persistence, API contracts, or the task transcript data model.

## Out of scope

- Changing the default setting value.
- Altering the transcript's tab layout or dockview geometry.
