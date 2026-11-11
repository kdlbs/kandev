---
created: 2026-11-10
status: complete
requirements:
  - REQ-OFFICE-THREAD-EXTRACTS-001
system_design:
  - ../../specs/office/system-design/thread-extract-comments.md
legacy_specs: []
---

# Implementation Plan: Office Thread Extract Comments

## Overview

Add a flow for users to attach a remark to selected text in an Office agent comment. The vertical slice reuses comment selection, task-scoped browser drafts, the existing comment API, and existing scheduler routing.

## Scope

### In scope

- Select text from an agent-authored Office comment and add a remark on desktop and touch layouts.
- Keep pending extracts scoped to the task and preserve draft edits during a comment request.
- Keep copied source mentions from starting new agent routes while preserving typed user mentions.
- Prevent extract creation on read-only tasks.

### Out of scope

- A new API, server draft type, or plugin contract.
- Cross-tab or cross-device draft synchronization.
- Changes to existing Office workflow wake rules.

## Technical approach

The Office adapter passes agent comments to the shared selection surface. The comments domain hook stores extracts under `office-task:<taskId>` and displays them in the task composer. The composer sends the quote and remark in one ordinary user task comment. The existing Office handler stores it, and existing scheduler and WebSocket paths process and refresh it.

The formatter protects mention syntax only for Office source passages. Typed feedback and ordinary chat extracts keep their existing mention behavior. The comment store acknowledges only unchanged extract snapshots after a successful POST.

## ASCII UI preview

### UI-01: Desktop Office task comment

Entry point: select text in an agent comment in Office task detail. The comment list scrolls independently from the composer.

```text
Agent comment: The report flags a risk in the billing migration.
                 [selected passage]
                               [Remark popover]
                               [Remark field]
                               [Add]
Composer: [quoted extract chip] [typed text...] [Send]
```

### UI-02: Phone Office task comment

Entry point: select text in an agent comment on a phone. The task view remains visible behind the selection drawer.

```text
Agent comment: The report flags a risk in the billing migration.
                 [selected passage]
+--------------------------------+
| Remark drawer                  |
| [Remark field]                 |
| [Add]                          |
+--------------------------------+
Composer: [quoted extract chip] [Send]
```

In both views, Add stores a draft. Send posts it. The preview shows order and behavior, not pixel spacing.

## Tests

- `apps/web/lib/state/slices/comments/format.test.ts`: quoted Office mentions are inactive; typed feedback mentions remain active; ordinary chat source text remains unchanged.
- `apps/web/hooks/domains/comments/use-thread-extracts.test.ts`: task-scoped draft selection and acknowledgement.
- `apps/web/components/task/simple/task-chat.extracts.test.tsx`: submission preserves edits made while a request is pending.
- `apps/web/components/task/chat/messages/message-comment-surface.test.tsx`: selection surface and drawer Add behavior.
- `apps/web/lib/state/slices/comments/message-comments.test.ts`: comment-store snapshot acknowledgement.
- `apps/backend/internal/office/scheduler/mentions_test.go`: escaped source mentions do not route; explicit mentions still route.

## E2E tests

- `apps/web/e2e/tests/office/thread-extract-comments.spec.ts` (`chromium`): add, reload, and submit multiple extracts on desktop.
- `apps/web/e2e/tests/office/mobile-thread-extract-comments.spec.ts` (`mobile-chrome`): add from the phone drawer without Run and prevent extracts on a read-only task.

## Work orders

- [x] [Task 01: Add Office thread extract comments](task-01-thread-extract-comments.md)

## Verification results

The focused frontend suite passed with 52 tests. The scheduler test, web typecheck, desktop and phone E2E flows, changed-file lint and formatting checks, and documentation validators passed. The previously failing background handoff E2E case also passed locally. GitHub CI passed on final PR head `2c4831f32df7895fda9435e02ad3f25be963a4aa`: 83 passed, 21 skipped, 0 failed, and 0 pending.

## Risks

- Unsent extracts live in the current browser tab's session storage and do not synchronize across devices.
- Existing Office mention parsing scans the posted comment body, so source text must stay escaped while new user mentions remain active.
