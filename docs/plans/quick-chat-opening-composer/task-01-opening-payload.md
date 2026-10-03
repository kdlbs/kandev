---
id: "01-opening-payload"
title: "Preserve and deliver the complete opening payload"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-QUICK-CHAT-COMPOSER-001
acceptance_criteria:
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.6
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.8
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.9
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.10
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.12
system_design:
  - ../../specs/tasks/system-design/quick-chat-opening-composer.md
---

# Task 01: Preserve and deliver the complete opening payload

## Summary

Make the existing launch paths carry and recover the complete opening message.
Keep current setup callers functional while adding attachment-aware delivery.

## In scope

- Add a full opening payload to the Quick Chat store and session plumbing with
  a compatibility bridge for string callers. Retain stable message identity.
- Extend `useQuickChatInitialPrompt` recovery to text and descriptors. Bind each
  attempt to its original session and submitter. Preserve newer manual drafts.
- Extend both HTTP chat launch requests for passthrough attachments. Validate,
  claim, and deliver through existing services and launch intents. Cover rollback
  and explicit same-session recovery after an accepted launch fails.
- Preserve no-prompt API callers, saved-prompt behavior, config tool mode, title
  generation, and request-generation guards. Reconcile a superseded start's
  completed session into the workspace tabs without activating or deleting it.
- Review follow-up: retain authenticated attachment ownership in detached
  configuration launches and bind deferred delivery to its original session
  and task generation.

## Out of scope

Visible setup redesign, new MCP modes, provider capability expansion, and schema migrations.

## Acceptance

1. Structured opening payloads retain attachments through blocked admission,
   acceptance, rejection, remount, and a newer-draft race without automatic replay.
2. Both terminal-backed routes pass text and attachments exactly once through
   the existing launch intents; invalid or foreign attachments never dispatch.
3. Rollback preserves usable attachment recovery for an explicit retry;
   accepted-session recovery keeps that session identity. Existing callers pass.

## Verification

Write regression tests first and record the failing results before the fix.
Run from the repository root:

```bash
(cd apps/backend && go test ./internal/task/handlers ./internal/orchestrator -run 'QuickChat|ConfigChat|ChatOpening|OpeningPayload|OpeningAttachment|Passthrough' -count=1)
(cd apps/web && pnpm exec vitest run components/quick-chat/use-quick-chat-initial-prompt.test.ts components/quick-chat/quick-chat-session-view.test.tsx components/config-chat/use-config-chat.test.ts lib/state/slices/ui/quick-chat-actions.test.ts lib/state/slices/ui/quick-chat-sync.test.ts lib/local-storage.test.ts)
(cd apps/web && pnpm run typecheck)
git diff --check
```

No rendered layout changes belong to this work order. Task 03 supplies browser proof.

## Files likely touched

- `apps/backend/internal/task/handlers/task_http_handlers.go` and its tests.
- `apps/web/lib/api/domains/workspace-api.ts`.
- `apps/web/lib/state/slices/ui/types.ts`, `quick-chat-actions.ts`, and reconciliation tests.
- `apps/web/components/quick-chat/use-quick-chat-initial-prompt.ts` and its tests.
- `apps/web/components/quick-chat/quick-chat-content.tsx`, `quick-chat-session-view.tsx`, `quick-chat-modal.tsx`.
- `apps/web/components/config-chat/use-config-chat.ts` and its tests.
- `apps/web/lib/local-storage.ts` and its tests, only for existing draft helper extensions.

## Dependencies

None. Read the attachment and saved-prompt designs before changing dispatch.

## Risks

An HTTP launch and a shell handoff must never both own one payload. Rollback can
remove claimed files, so test actual claim ownership, not only forwarded arguments.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/quick-chat-opening-composer.md).
- [Design](../../specs/tasks/system-design/quick-chat-opening-composer.md).
- Root and scoped `AGENTS.md`; `/tdd`, `/mobile-parity`, and `/e2e` as applicable.

## Results

Implemented the structured opening payload for Quick Chat and configuration
sessions, including attachment descriptors and stable message identity. Persisted
opening drafts before structured dispatch, retained text and attachment recovery
after rejection, and preserved a newer manual draft. Both HTTP launch paths now
validate and deliver the prompt and attachments through their existing launch
intents. Failed launch attachments return to staging before cleanup. Quick Chat
rolls back only when no session was allocated; otherwise it retains the task and
returns the session identity for explicit recovery. Late completed sessions are
reconciled into tabs without taking active focus or deleting the task.

TDD regressions first failed because the HTTP routes dropped the opening payload
and accepted attachments without a prompt. The frontend recovery tests first
failed because the store only seeded a string message. After implementation:

- The focused Go handler/service tests and the planned handler/orchestrator suite
  passed.
- The six planned web test files passed (114 tests).
- `pnpm run typecheck` passed.
- `git diff --check` passed.

Review follow-up is complete. A SQLite-backed handler regression stages a
file-backed attachment, cancels the request context after HTTP 200, and uses the
real `ClaimMessageAttachments` service for both the owner and a foreign user.
Another regression schedules session A's opening delivery, switches to unblocked
session B before the microtask flush, and proves B only consumes its own payload.

- `go test ./internal/task/handlers -count=1` passed.
- Six focused frontend test files passed: 48 tests. The broader affected suite
  passed: 31 files, 320 tests.
- `pnpm run typecheck`, focused ESLint, and `pnpm run build:vite` passed.
- `git diff --check` passed.

Merged-base PR fixup verification also passed:

- `go test -trimpath ./internal/task/handlers -count=1` and
  `go test -trimpath -race ./internal/task/handlers -count=1` passed. The suite
  includes a real file-backed staged upload claimed by its owner after request
  cancellation and rejected for a foreign user.
- `go test -trimpath ./internal/orchestrator -count=1` passed.
- The focused deferred-delivery regression passed with the Quick Chat suite; an
  unblocked session B received only its own payload and handoff callback.
- `git diff --check` passed after the merge integration.
