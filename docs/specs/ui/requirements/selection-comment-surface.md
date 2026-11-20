---
status: active
system: ui
created: 2026-11-16
owners:
  - kandev
---

# Selection Comment Surface Requirements

## Overview

Kandev lets a user attach a comment to something they selected: plan text,
agent-reply text, an Office thread extract, a question-review image region, a
code-editor range, a Markdown preview range, or a web-preview capture. Each of
these opens a small form to write or edit the comment. This requirement makes
that form one reusable surface, so all of these places look, position, and
respond to the keyboard the same way, and a behavior fix in one place applies
everywhere.

Each feature that uses the surface still owns its comment data, persistence,
delivery, and whether it offers a Run action. This document owns only the
shared authoring surface. The touch selection dock that opens the surface is
owned by
[touch selection actions](touch-selection-actions.md).

## Terminology

- **Selection comment surface:** The shared form for writing or editing one
  comment attached to a selection, region, or capture.
- **Anchor:** The viewport point next to which the desktop surface first
  appears, usually just below the selection or region.
- **Evidence:** What the comment is about, as shown in the surface: quoted
  text by default, or a feature-supplied preview such as an element label or a
  screenshot thumbnail.
- **Context label:** A short feature-supplied tag, such as a line range or a
  page route.
- **Hosting feature:** A feature that opens the surface and handles its Add,
  Run, Update, and Delete results.

## Requirements

### REQ-UI-SELECTION-COMMENT-SURFACE-001: One shared selection comment surface

**Intent:** Remove the duplicate comment popovers so comment authoring behaves
the same way everywhere in Kandev.

**User story:** As a user who comments on plans, replies, files, images, and
web previews, I want the comment form to work the same way in each place, so
that I don't have to learn a different form for each surface.

#### Acceptance criteria

- **AC-UI-SELECTION-COMMENT-SURFACE-001.1:** The Plan panel, agent-message and
  Office thread comments, question-review image regions, the CodeMirror file
  editor, the Markdown file preview, and web-preview feedback shall all
  author and edit selection comments through the same surface.
- **AC-UI-SELECTION-COMMENT-SURFACE-001.2:** With a fine pointer, the surface
  shall float next to its anchor and stay inside the visible bounds of its
  container. It shall be draggable by its handle and focus the comment field
  when it opens. Mod+Enter shall add or update, and Mod+Shift+Enter shall run
  when the hosting feature offers Run. Escape or a pointer press outside the
  surface shall close it, except while an add, update, or delete is pending.
- **AC-UI-SELECTION-COMMENT-SURFACE-001.3:** With a coarse pointer, the surface
  shall be a bottom Drawer with a header naming the action and its evidence.
  Its actions shall be at least 44 pixels high and it shall clear the safe
  area.
- **AC-UI-SELECTION-COMMENT-SURFACE-001.4:** The surface shall show the
  evidence and, when the hosting feature supplies one, the context label. The
  hosting feature may replace the quoted-text evidence with its own preview.
  The surface shall render that preview as content, never as raw HTML.
- **AC-UI-SELECTION-COMMENT-SURFACE-001.5:** Add shall require a non-blank
  comment. When the hosting feature rejects Add or Update, the surface shall
  stay open, keep the typed comment, and show the feature's localized error.
- **AC-UI-SELECTION-COMMENT-SURFACE-001.6:** When the surface edits an existing
  comment, it shall prefill that comment, offer Update and, when the feature
  supports it, Delete, and shall not offer Run.
- **AC-UI-SELECTION-COMMENT-SURFACE-001.7:** A hosting feature may keep the
  typed comment outside the surface. When it does, closing and reopening the
  surface shall restore the same text.
- **AC-UI-SELECTION-COMMENT-SURFACE-001.8:** The code editor and Markdown
  preview shall keep their line-range label and Add/Run actions on desktop.
  With a coarse pointer they shall use the same bottom Drawer as other
  surfaces.

## Out of scope

- Monaco and diff inline comment forms rendered in editor view zones.
- The read-only comment list popover (`CommentViewPopover`) and the selection
  triggers or docks that open the surface.
- Changing any feature's comment data, persistence, delivery, or Run routing.
