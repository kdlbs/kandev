---
id: "01-resolve-task-comment-links"
title: "Resolve task comment links in their workspace"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-COMMENT-MARKDOWN-001
acceptance_criteria:
  - AC-UI-COMMENT-MARKDOWN-001.6
  - AC-UI-COMMENT-MARKDOWN-001.12
  - AC-UI-COMMENT-MARKDOWN-001.13
  - AC-UI-COMMENT-MARKDOWN-001.14
  - AC-UI-COMMENT-MARKDOWN-001.15
system_design:
  - ../../specs/ui/system-design/comment-markdown.md
---

# Task 01: Resolve task comment links in their workspace

## Summary

Pass the task workspace prefix and ID through comment rendering. Include the workspace ID in each
task identifier link. Resolve each identifier through the workspace-scoped Office API. Then
replace the identifier URL with the canonical task ID and preserve query parameters.

Keep UUID routes working. Recognize identifiers generated from custom workspace prefixes. Show
separate UI states for a missing workspace, an unknown task, and a retryable API failure.

## Verification

Frontend tests cover comment workspace context, identifier patterns, redirects, query values, and
lookup errors. Backend tests cover workspace-scoped identifier lookup.
