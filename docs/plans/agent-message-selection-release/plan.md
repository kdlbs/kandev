---
status: complete
created: 2026-10-19
requirements:
  - REQ-UI-AGENT-MESSAGE-COMMENTS-001
system_design:
  - ../../specs/ui/system-design/agent-message-selection.md
---

# Capture message selections released in padding

The message-local mouse-release handler missed valid selections when the
pointer ended in surrounding padding. The repair captures mouse release at the
document boundary and preserves message-local selection validation.

## Work order

1. [Capture and validate selections](task-01-selection-release.md), done.

## Verification and scope

Twenty focused tests and six browser cases passed, including real drags in Task
Chat and Quick Chat and the existing mobile drawer flows. Typecheck, changed-file
lint, formatting, and the translation ratchet passed. The work order records the
commands. Public documentation already describes the intended selection flow.

The repair changes mouse input handling only. Touch capture and the phone drawer
remain unchanged. The main risk is accepting another message's selection or
replacing an open editor's quote; focused tests cover those boundaries.
