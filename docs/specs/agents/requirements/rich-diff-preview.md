---
status: active
system: agents
created: 2026-11-25
owners:
  - kandev
---

# Rich diff preview requirements

## Overview

An agent can already reference a workspace diff with FilePreview. Readers need to identify changes and inspect later hunks inside the conversation.

### REQ-AGENTS-RICH-DIFF-PREVIEW-001: Read unified diffs in rich output

**User story:** As a reader, I want to see which lines changed between two snapshots and scroll or page to later changes.

#### Acceptance criteria

- **AC-AGENTS-RICH-DIFF-PREVIEW-001.1:** Expanding a native FilePreview for a unified diff with a .diff or .patch extension shows file headers, hunk headers, context, additions and deletions in patch order within each revision. Snapshot labels and multi-file patches are supported.
- **AC-AGENTS-RICH-DIFF-PREVIEW-001.2:** Additions and deletions retain their literal signs and have distinct theme-aware backgrounds. Whitespace and long lines remain readable on desktop and phone. The chat stays within its viewport.
- **AC-AGENTS-RICH-DIFF-PREVIEW-001.3:** The reader can reach the last line of a 733-line diff returned by the workspace reader. A bounded initial render indicates when more content is available.
- **AC-AGENTS-RICH-DIFF-PREVIEW-001.4:** Existing file authorization, plugin selection, open, collapse and reopen behavior apply. Unrecognized text retains the ordinary text preview. Binary and unavailable files retain their existing states.
- **AC-AGENTS-RICH-DIFF-PREVIEW-001.5:** Each new native diff card expands automatically and opens side by side on desktop and phone, with the old revision on the left and the new revision on the right. Readers can switch to unified patch order and back using a labeled control. Switching keeps the current change in view.

## Out of scope

Patch application, interactive review comments, and comparing two live workspace files.
