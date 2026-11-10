---
status: done
created: 2026-11-10
requirements:
  - REQ-UI-COMMENT-MARKDOWN-001
system_design:
  - ../../specs/ui/system-design/comment-markdown.md
---

# Workspace-aware task comment links

## Outcome

Task identifier links in comments use the workspace that owns the comment. The task route resolves
each identifier in that workspace. The redirect keeps the workspace context and uses the canonical
task ID.

## Acceptance criteria

- AC-UI-COMMENT-MARKDOWN-001.6
- AC-UI-COMMENT-MARKDOWN-001.12
- AC-UI-COMMENT-MARKDOWN-001.13
- AC-UI-COMMENT-MARKDOWN-001.14
- AC-UI-COMMENT-MARKDOWN-001.15

## Work orders

- [Task 01: Resolve task comment links in their workspace](task-01-resolve-task-comment-links.md)
