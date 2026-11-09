---
status: active
system: ui
created: 2026-10-28
owners:
  - kandev
---

# Copy Full Commit Identifier Requirements

## Overview

A reader of an already loaded commit detail can copy its full identifier while recognizing the existing short identifier. UI owns this local presentation action; repository identity, source data and provider permissions remain with their existing owners. The [commit file navigation contract](commit-file-navigation.md) retains ownership of the surrounding detail presentation, and [clipboard feedback](clipboard-feedback.md) owns reusable confirmation timing.

## Requirements

### REQ-UI-COPY-COMMIT-ID-001: Copy the displayed commit identifier

**Intent:** Let a commit reader use the complete current identifier without manually reconstructing it from the shortened display.

#### Acceptance criteria

- **AC-UI-COPY-COMMIT-ID-001.1:** When an existing loaded commit detail header displays a nonempty identifier, it shall offer an explicitly accessible copy action adjacent to or incorporating the visible seven-character identifier. Local and GitHub-sourced details shall offer the same action.
- **AC-UI-COPY-COMMIT-ID-001.2:** Activating the action shall copy the exact full identifier of the current detail, including after the selected identifier changes, without truncating or rewriting it.
- **AC-UI-COPY-COMMIT-ID-001.3:** A successful copy shall provide localized, accessible success feedback. A failed copy shall remain retryable and shall not produce a new success claim; existing feedback from an earlier success retains its normal lifetime.
- **AC-UI-COPY-COMMIT-ID-001.4:** The action shall be visible and keyboard operable on desktop and phone, with a 28px desktop target and at least 44px in both dimensions for phone or coarse-pointer use, including a narrow fine-pointer phone. It shall remain contained without document horizontal overflow.
- **AC-UI-COPY-COMMIT-ID-001.5:** Existing header visibility, metadata, file index, diffs and navigation shall remain unchanged. The action shall not require a save, Git write, API request, or agent, session, or workspace operation.

## Out of scope

- Commit rows, context menus, history, file index actions, new empty or error headers, and new metadata surfaces.
- Commit acquisition, validation, repository/source identity, backend or Git changes.

## Implementation plan

- [Copy full commit identifier](../../../plans/copy-full-commit-identifier/plan.md)
