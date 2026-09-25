---
status: active
system: ui
created: 2026-09-17
owners:
  - kandev
---

# Review file comments

## Overview

A reviewer can leave feedback about a whole file without selecting diff lines.
UI owns this local review interaction and its browser-held feedback, alongside
existing inline comments. Task message delivery retains its existing ownership.

## Terminology

- **File comment:** review feedback attached to a repository and file, without a line anchor.
- **Line comment:** existing feedback attached to selected diff lines.

## Requirements

### REQ-UI-REVIEW-FILE-COMMENTS-001: Whole-file review feedback

**Intent:** Let reviewers discuss file structure, naming, or non-text changes
through the same feedback workflow as line comments.

#### Acceptance criteria

- **AC-UI-REVIEW-FILE-COMMENTS-001.1:** Every file displayed in Review shall expose a keyboard-accessible Comment on file action without requiring line selection. It shall work for collapsed files, Markdown preview, deleted and renamed files, and files without a textual diff.
- **AC-UI-REVIEW-FILE-COMMENTS-001.2:** The editor shall identify the target file and repository. Adding non-whitespace text shall create one pending whole-file comment; empty submission shall be disabled. Cancel shall create no comment. Pending file comments shall be readable, editable, and removable from their file section.
- **AC-UI-REVIEW-FILE-COMMENTS-001.3:** File comments shall retain their originating session and repository/file identity across Review close/reopen and same-tab reload, using the same browser-local lifetime as line comments. Identical relative paths in different repositories shall not share new file comments. Creating a comment shall not mark a file reviewed or modify its contents.
- **AC-UI-REVIEW-FILE-COMMENTS-001.4:** File comments shall contribute to the file and review comment counts and the Fix comments overview. They shall display a whole-file label without fabricated line numbers, old/new side, or code excerpt. Existing line comments shall retain their anchors and presentation.
- **AC-UI-REVIEW-FILE-COMMENTS-001.5:** Fix comments and normal composer submission shall include whole-file feedback and its unambiguous file/repository context alongside pending line feedback for the current session. File comments shall use the same sending, queuing, clearing, and error behavior as the corresponding existing line-comment route.
- **AC-UI-REVIEW-FILE-COMMENTS-001.6:** On phones, the visible file actions menu shall provide Comment on file and open the editor in that file section. Add, Cancel, edit, and delete shall be reachable without hover and have at least 44px touch targets. Long paths and comments shall not cause document horizontal overflow; focus shall move to the editor and return to its opener on cancellation. All new interface copy shall be localized.

## Out of scope

Provider-hosted PR/MR comments, server synchronization, cross-browser persistence,
thread replies, automatic re-anchoring after renames, and changes to existing
send reliability or review-state semantics.

## Implementation plans

[Review file comments](../../../plans/review-file-comments/plan.md).
