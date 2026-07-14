---
spec: docs/specs/tasks/quick-chat-repository-context.md
created: 2026-07-14
status: complete
---

# Implementation Plan: Quick Chat Repository Context

## Overview

Extend quick chat's existing ephemeral-task contract to carry ordered repository inputs and
enforce isolated worktree preparation. Then extract the controlled workspace repo/branch chips
from task creation, replace the agent-card picker with a responsive setup form, and verify the
desktop and mobile flows.

## Backend

- Extend `httpStartQuickChatRequest` in
  `apps/backend/internal/task/handlers/task_http_handlers.go` with plural repository inputs.
- Normalize plural or legacy singleton fields into `[]service.TaskRepositoryInput`, reject mixed
  shapes, and force `models.ExecutorIDWorktree` for repo-backed starts.
- Preserve eager launch and rollback through `Service.DeleteTask`.

## Frontend

- Add the plural request type in `apps/web/lib/api/domains/workspace-api.ts`.
- Extract a controlled workspace repository/branch chip component from
  `apps/web/components/task-create-dialog-repo-chips.tsx` without moving task-only URL/folder and
  fresh-branch modes.
- Add `apps/web/components/quick-chat/quick-chat-setup.tsx`; use the shared agent selector,
  repository chips, approved helper copy, and responsive footer.
- Keep start supersession and orphan cleanup in `use-quick-chat-modal.ts`.

## Tests

- Backend handler tests cover plural normalization, compatibility, isolation, and validation.
- Frontend unit tests cover payload construction/default selection logic and supersession.
- Existing task chip tests guard the extraction against regressions.

## E2E Tests

- Extend `apps/web/e2e/tests/chat/quick-chat.spec.ts` for setup helper copy and repo-backed start.
- Add a `mobile-*.spec.ts` flow for the same user value and narrow layout.

## Implementation Waves

Wave 1:
- [x] [task-01-backend-contract](task-01-backend-contract.md)
- [x] [task-02-shared-repository-chips](task-02-shared-repository-chips.md)

Wave 2:
- [x] [task-03-quick-chat-setup](task-03-quick-chat-setup.md)

Wave 3:
- [x] [task-04-e2e-verification](task-04-e2e-verification.md)
