---
status: active
system: ui
created: 2026-08-31
owners:
  - kandev
---

# Surface Text Hierarchy Requirements

## Overview

Titles and descriptions in alerts, alert dialogs, dialogs, drawers, and sheets
serve different reading jobs. Titles are short labels that can benefit from
balanced multi-line wrapping. Descriptions are prose and structured content
that must use natural line lengths, especially on phones and in longer
localizations.

The current shared alert and alert-dialog descriptions invert that rule:
descriptions balance on viewports below the `md` breakpoint and use pretty
wrapping only above it. Every consumer inherits the phone behavior, including
multi-paragraph confirmations. The other surface families do not carry that
exact defect, but lack one shared semantic wrapping contract.

## Terminology

- **Title:** The accessible heading naming an alert or overlay surface.
- **Description:** Supporting prose or structured content associated with the
  title.
- **Structured description:** A description rendered through `asChild` or an
  equivalent composition and containing multiple paragraphs, lists, or groups.

## Requirements

### REQ-UI-SURFACE-TEXT-HIERARCHY-001: Semantic title and description wrapping

**Intent:** Make alert and overlay text readable at every supported width by
matching wrapping behavior to the text's semantic role.

#### Acceptance criteria

- **AC-UI-SURFACE-TEXT-HIERARCHY-001.1:** Shared Alert, AlertDialog, Dialog,
  Drawer, and Sheet titles shall balance multi-line headings at phone, tablet,
  and desktop widths. Their descriptions shall use natural or pretty prose
  wrapping and shall not switch to balanced body copy at a responsive
  breakpoint.
- **AC-UI-SURFACE-TEXT-HIERARCHY-001.2:** A structured description shall retain
  semantic paragraph and list structure, readable spacing, and explicit
  left-aligned prose when its containing header centers short phone copy.
- **AC-UI-SURFACE-TEXT-HIERARCHY-001.3:** At a 320 CSS px viewport and larger
  supported widths, translated text and dynamic names or identifiers shall wrap
  inside the surface without document-level horizontal overflow or inaccessible
  text.
- **AC-UI-SURFACE-TEXT-HIERARCHY-001.4:** Consumers shall be able to override a
  shared wrapping or alignment default through the existing `className`
  contract when their content has a documented presentation need.
- **AC-UI-SURFACE-TEXT-HIERARCHY-001.5:** The wrapping change shall preserve
  accessible title-description relationships, focus behavior, dismissal,
  action order, and all consumer business behavior.

## Out of scope

- Changing global font families, font sizes, colors, or line-height tokens.
- Rewriting every dialog's product copy.
- Changing dialog placement, navigation, or state ownership.
- Applying vertical containment or action sizing to every existing surface.
- Removing intentional text wrapping from non-title, non-description UI.
