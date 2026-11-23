---
status: active
system: tasks
created: 2026-11-22
owners:
  - kandev
---

# Attachment references in task feedback

## Overview

A user can place evidence beside the feedback it explains. The existing [question answer contract](question-answer-attachments.md) owns clarification responses. This extension covers task comments.

## Requirements

### REQ-TASKS-COMMENT-FILES-001: Files associated with a comment

#### Acceptance criteria

- **AC-TASKS-COMMENT-FILES-001.1:** Users can attach multiple files to a plan comment, an agent output selection, a walkthrough step, or a file comment. File comments include editor selections, diff lines and whole-file Review comments. Existing text and anchor rules still apply.
- **AC-TASKS-COMMENT-FILES-001.2:** Desktop and phone users can select, inspect and remove a file before saving. Pending uploads block submission. A failed upload identifies the affected file and allows retry or removal while preserving text, anchor and ready files.
- **AC-TASKS-COMMENT-FILES-001.3:** Each delivered comment identifies its own files, in selection order, by readable paths in the receiving execution. Two comments with identically named files retain distinct references. Images and other files use the same reference contract.
- **AC-TASKS-COMMENT-FILES-001.4:** Saving or editing a plan comment retains its files across reload and session changes in the task. Send targets the selected session. Run targets the primary session. Session-local comments retain their existing draft scope.
- **AC-TASKS-COMMENT-FILES-001.5:** Admission rejects foreign, expired or unavailable references and limit violations without consuming pending comments. Retrying the same acknowledged send does not duplicate delivery. A materialization failure prevents agent dispatch and preserves recoverable message evidence.
- **AC-TASKS-COMMENT-FILES-001.6:** Comments and answers without files retain their current behavior. Existing question answers continue to associate files with the specific question through the text tool result.

See the [system design](../system-design/comment-attachment-references.md) and [plan](../../../plans/comment-attachment-references/plan.md).
