---
status: active
system: ui
created: 2026-09-22
owners:
  - kandev
---

# Markdown Math Requirements

## Overview

Readers need formatted formulas in agent responses, document previews, and comments.
UI owns this reusable presentation contract across shared Markdown surfaces.
Task storage and editor behavior remain with their existing owners.

## Requirements

### REQ-UI-MARKDOWN-MATH-001: Readable formulas

**Intent:** Readers can understand mathematical expressions without reading TeX source.

#### Acceptance criteria

- **AC-UI-MARKDOWN-MATH-001.1:** Inline `$E = mc^2$` shall show formatted math within the surrounding sentence.
- **AC-UI-MARKDOWN-MATH-001.2:** Standalone `$$\frac{a}{b}$$` shall show a display formula. Delimiters on separate lines shall also work.
- **AC-UI-MARKDOWN-MATH-001.3:** Chat messages, Markdown previews, document bodies, comments, and other shared Markdown surfaces shall show the same formula semantics.
- **AC-UI-MARKDOWN-MATH-001.4:** Formulas shall remain legible in light and dark themes, with an accessible mathematical representation.
- **AC-UI-MARKDOWN-MATH-001.5:** On phones, wide display formulas shall scroll within their content region without widening the page or hiding formula content.
- **AC-UI-MARKDOWN-MATH-001.6:** Incoming chat formulas shall render with animations enabled or disabled. Incomplete or invalid formulas shall not prevent surrounding content from rendering.

### REQ-UI-MARKDOWN-MATH-002: Markdown compatibility

**Intent:** Formula support preserves ordinary text and the existing content safety boundary.

#### Acceptance criteria

- **AC-UI-MARKDOWN-MATH-002.1:** `$100 and $200` and escaped dollar signs shall remain literal text without math containers.
- **AC-UI-MARKDOWN-MATH-002.2:** Single-dollar math requires non-whitespace characters immediately inside both delimiters. Dollar signs inside inline code and ordinary fenced code shall remain literal.
- **AC-UI-MARKDOWN-MATH-002.3:** Existing headings, tables, task links, file links, emoji, line breaks, and Mermaid diagrams shall retain their behavior.
- **AC-UI-MARKDOWN-MATH-002.4:** Formula rendering shall not enable executable HTML, unsafe URLs, or trusted TeX commands from untrusted content.
- **AC-UI-MARKDOWN-MATH-002.5:** Existing preview source-line comments and surrounding chat text animation shall retain their positions and behavior.

## Out of scope

- A formula editor, TeX document compilation, and custom macro configuration.
- New behavior in TipTap editing, exported share snapshots, clarification prompts, or authentication setup hints.
- Backend changes, persisted source rewriting, and new settings.

## Implementation plans

- [Markdown math](../../../plans/markdown-math/plan.md)
