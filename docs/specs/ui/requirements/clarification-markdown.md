---
status: active
system: ui
created: 2026-10-24
owners:
  - kandev
---

# Clarification Markdown requirements

## Overview

Operators need readable questions and usable reference links in active clarifications and Inbox History.
UI owns this reusable presentation contract across question views. Task ownership, response delivery, and History eligibility retain their existing contracts.

The user identified question text and Inbox History as the affected views.
This contract extends the existing lightweight question formatting to History. It does not enable the richer comment renderer.

## Requirements

### REQ-UI-CLARIFICATION-MARKDOWN-001: Consistent question formatting

**Intent:** Show the same question content as readable Markdown wherever the operator inspects it.

#### Acceptance criteria

- **AC-UI-CLARIFICATION-MARKDOWN-001.1:** Active question prompts and expanded History prompts shall render paragraphs, line breaks, ordered lists, unordered lists, links, bold, italic, strikethrough, and inline code. Question titles and displayed option labels shall render the corresponding inline formatting. Formatting delimiters shall not appear as literal punctuation for supported, valid Markdown.
- **AC-UI-CLARIFICATION-MARKDOWN-001.2:** A collapsed History summary shall format its question title, or its prompt when the title is absent, as one truncated line. Selecting formatted text in this summary shall expand or collapse the row. It shall not open a reference destination. Full question content shall remain available after expansion.
- **AC-UI-CLARIFICATION-MARKDOWN-001.3:** Supported HTTP and HTTPS links in active questions and expanded History question content shall open their original destinations in a separate browsing context. Link activation shall not answer a question or change History state. Links inside active answer choices shall remain formatted labels whose activation selects that choice.
- **AC-UI-CLARIFICATION-MARKDOWN-001.4:** Unsafe URL schemes shall not create navigable links. Relative paths, file URLs, email URLs, and fragment destinations shall remain non-navigable labels. Raw HTML, images, headings, blockquotes, and fenced code panels shall not introduce executable or rich embedded content. Tables shall remain unsupported outside pending shared context. Unsupported formatting shall retain readable text where the limited renderer permits it.
- **AC-UI-CLARIFICATION-MARKDOWN-001.5:** On phones, expanded questions and long reference labels shall wrap within the available width without document horizontal scrolling. Collapsed summaries shall retain one-line truncation. Reference links shall support keyboard and touch activation. Existing row actions shall remain reachable.
- **AC-UI-CLARIFICATION-MARKDOWN-001.6:** A missing question title shall not hide a non-empty prompt or its lists. Missing question content shall use the existing localized fallback. Custom answers, permission requests, and context-only History summaries shall retain their literal presentation. Pending shared context follows criterion .8.
- **AC-UI-CLARIFICATION-MARKDOWN-001.7:** Rendering shall preserve stored question strings, option identifiers, and answer payloads. Inspecting History or opening a reference shall not answer, reject, re-ask, dismiss, or snooze an interaction.

- **AC-UI-CLARIFICATION-MARKDOWN-001.8:** Pending shared context shall support the block formatting in criterion .1 and readable GFM tables. Table overflow shall remain inside the table region. Context links shall follow criteria .3 and .4 without selecting, submitting, dismissing, or collapsing a clarification. Stored context shall remain unchanged.

## Related contracts

- [Inbox History](inbox-history.md) owns eligibility, ordering, row actions, and read-only access.
- [Shared clarification context](clarification-context.md) owns the separate context region.
- [Active clarification lifecycle](../../tasks/requirements/clarification-active-lifecycle.md) owns answerability and response delivery.

## Out of scope

- Rich comment Markdown, workspace file navigation, and embedded HTML or media.
- Changes to question schemas, persistence, permissions, lifecycle, or History membership.
- New option descriptions or answer summaries in History.
- Markdown in permission text, custom answers, resolved shared context, or plugin-managed chat.

## Contract amendment

Issue [#4810](https://github.com/kdlbs/kandev/issues/4810) extends pending shared context formatting.
The amendment is implemented in the context usability work package below. Existing question and History policies remain unchanged.

## Implementation plans

- [Clarification Markdown consistency](../../../plans/clarification-markdown/plan.md).

- [Clarification context usability](../../../plans/clarification-context-usability/plan.md).
