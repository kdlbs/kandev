---
id: "01-unanswered-inbox"
title: "Unanswered clarification inbox"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CLARIFICATION-UNANSWERED-001
acceptance_criteria:
  - AC-TASKS-CLARIFICATION-UNANSWERED-001.1
  - AC-TASKS-CLARIFICATION-UNANSWERED-001.2
  - AC-TASKS-CLARIFICATION-UNANSWERED-001.3
  - AC-TASKS-CLARIFICATION-UNANSWERED-001.4
  - AC-TASKS-CLARIFICATION-UNANSWERED-001.5
  - AC-TASKS-CLARIFICATION-UNANSWERED-001.6
  - AC-TASKS-CLARIFICATION-UNANSWERED-001.7
  - AC-TASKS-CLARIFICATION-UNANSWERED-001.8
  - AC-TASKS-CLARIFICATION-UNANSWERED-001.9
system_design:
  - ../../specs/tasks/system-design/clarification-active-lifecycle.md
---

# Task 01: Unanswered clarification inbox

## Summary

Keep questions an agent stopped waiting on answerable as ordinary messages while the workspace opts
in, close exactly the question a late answer answered, and let agents see them before asking again.

## In scope

- `workspaces.clarification_unanswered_inbox_enabled` and
  `workspaces.clarification_unanswered_retention_days` through schema, migration, repository,
  service validation, DTO, HTTP/WS update, events, boot state, and the web store.
- `ListUnansweredClarificationBundles`, `CloseUnansweredClarificationBundle`, and
  `MergeUnansweredClarificationDuplicates` in a new repository file, with lazy retention.
- `GET /api/v1/clarification-inbox/unanswered`, `POST .../unanswered/:pendingId/close`, and
  `POST .../unanswered/:pendingId/late-answer`.
- `include_unanswered` and per-bundle `state` on `list_pending_questions_kandev`.
- The Needs-you unanswered section, the late-answer hook's target and close, and the settings card.

## Acceptance

1. With the setting off, no unanswered rows, merges, retention writes, extra requests, or `state`
   fields appear; existing lifecycle tests stay green.
2. With it on, a superseded pending question is listed until it is answered late, closed, merged,
   expired by retention, or its task is archived; terminal-session expiry and parent questions stay
   excluded, and reads stay workspace-scoped.
3. A late answer closes only its own question and defaults to the asking session; another session of
   the same task is accepted only after the asking session ended.

## Verification

The existing late-answer web hook tests and selected operational SQLite and handler tests were run.
The backend checks used temporary overlays because those operational tests are not included in this
proposal. The pull request records the precise scope and remaining unverified paths.

## Completion report

- Existing SQLite and handler tests were reused from the operational source as temporary overlays;
  the checks and limitations for this proposed change are described in the pull request.
- Late-answer receipt validation establishes a user-message admission for the task and session. It
  does not establish that the caller and sender are the same person.
- PostgreSQL runtime and end-to-end browser behavior remain unverified for this proposal.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/clarification_unanswered_query.go`
- `apps/backend/internal/clarification/inbox_unanswered_handlers.go`
- `apps/backend/internal/mcp/handlers/question_unanswered.go`
- `apps/web/components/needs-you-inbox/needs-you-inbox-unanswered-section.tsx`
- `apps/web/app/settings/workspace/workspace-unanswered-inbox-settings.tsx`
