---
created: 2026-09-25
status: implemented
requirements:
  - REQ-TASKS-GIT-PUSH-ERROR-DISMISSAL-001
system_design:
  - ../../specs/tasks/system-design/git-push-error-dismissal.md
legacy_specs: []
---

# Implementation Plan: Git Push Error Dismissal

## Overview

Add a durable manual Dismiss action to one Git push failure card. Implement the authenticated message update first in the same vertical slice as the shared desktop and phone card behavior, then verify persistence, authorization, failure feedback, and visibility of later failures.

## Scope

### In scope

- Add a message-ID-scoped WebSocket action for dismissing only Git push error messages.
- Persist the acknowledgment in existing message metadata and publish the existing message update event.
- Show a localized accessible Dismiss control on the shared action card and hide that exact card after the persisted update.
- Keep diagnostics stored and leave unrelated errors and later push failures visible.
- Update `docs/public/git-operations.md` with one sentence explaining that Dismiss hides only the historical failure message, does not change Git state, and later failures remain visible.

### Out of scope

- Git status checks, automatic success inference, polling, attempted-HEAD correlation, a new table or alert projection, undo, and confirmation.
- Dismissal of non-push Git errors or generic errors.

## Technical approach

The Git failure producer in `apps/backend/internal/backendapp/gateway.go` already stores `git_operation_error`, `operation`, and `error_output`; leave it unchanged. Add `message.dismiss_git_push_error` to `apps/backend/pkg/websocket/actions.go` and register it in `apps/backend/internal/task/handlers/message_handlers.go`. Its request payload is exactly `{ "message_id": "<message-id>" }`; its success payload is `{ "message_id": "<message-id>", "dismissed_at": "<UTC RFC3339Nano timestamp>" }` in the standard WebSocket response envelope. Implement the service operation in `apps/backend/internal/task/service/service_messages.go`: load the message and its actual session association, reject a sessionless row, authorize that session with `AuthorizeSessionAccess`, then require `type: error`, `git_operation_error: true`, and `operation: push`. Reject invalid or inaccessible rows without persistence or an update event. Write `git_operation_error_dismissed_at` once while retaining all other metadata, and call `UpdateMessage` for the standard broadcast. An already-marked row returns the original timestamp without another write or update event.

In `apps/web/components/task/chat/messages/action-message.tsx`, add a Dismiss control for eligible push error rows using the row's exact ID and existing `task:dismiss` label. Determine eligibility from strict `type: error` and Git push metadata, not from a new persisted action array, so existing legacy messages get the control. Use `apps/web/lib/utils/git-push-error-message.ts` for the strict predicates; filter dismissed rows in `apps/web/hooks/processed-message-filtering.ts` before messages split into transcript and footer action lists, and guard `apps/web/components/task/chat/message-renderer.tsx` for direct render callers. This removes the entire dismissed card, including Fix, from both the transcript and the separately rendered footer action path. Keep dismissal pending/error handling local to the Dismiss control; on failure show `toast.error(t("common:requestFailed"))` with the existing toast/localization helpers, clear pending state in `finally`, and leave both controls retryable. Do not optimistically hide the row or change generic `StandardActionButton` error handling. No new locale values or storage schema are required.

Dismissal is shared for everyone who can access the task conversation. The authenticated WebSocket request plus `AuthorizeSessionAccess` enforce the existing `authz.ScopeWorkspaceRead` authorization for the stored session; no caller-provided ownership or metadata fields are accepted.

## ASCII UI preview

### UI-01: Task chat Git push error card

Desktop view, before and after dismissal:

```text
Before                                  After
[!] Git push failed                    [Transcript continues]
    remote rejected the branch
    [Fix] [Dismiss]
```

Phone view, same content in one column with touch-sized controls:

```text
[!] Git push failed
    remote rejected the branch
    [Fix]
    [Dismiss]
```

The preview requires the existing diagnostic card and Fix action to remain grouped with a visible Dismiss button. Desktop buttons may share a row; phone buttons stack in the existing card layout and each touch target measures at least 44 pixels. Keyboard users can focus and activate Dismiss. A later push failure renders its own copy of this card. The drawing is structural, not a pixel specification.

## Tests

- `apps/backend/internal/task/service/service_messages_git_push_dismissal_test.go`: `TestDismissGitPushErrorMessage` proves success for one eligible row; denial for a forbidden session and a cross-workspace session; rejection of a sessionless row, wrong type, wrong operation, missing marker, and unrelated messages; and zero persistence calls/events for each rejection. It also proves retention of content, `error_output`, Fix actions, and other metadata; one update event on success; no second write/event for a repeated request; and no mutation on persistence failure.
- `apps/backend/internal/task/handlers/message_handlers_git_push_dismissal_test.go`: `TestWSMessageDismissGitPushError` proves registration, the message-ID-only request, exact success payload, and propagation of authorization, validation, and persistence failures.
- `apps/web/hooks/processed-message-filtering.test.ts`: proves a dismissed push error is removed before transcript/footer separation while a later message ID and unrelated errors remain visible.
- `apps/web/components/task/chat/message-renderer.test.tsx`: proves the shared direct-render path suppresses both action-card and status-row variants carrying the persisted dismissal marker.
- `apps/web/components/task/chat/messages/action-message.test.tsx`: proves an existing legacy message with only `type: error`, `git_operation_error: true`, and `operation: push` gets Dismiss without a newly persisted Dismiss action; failed requests show localized feedback and leave Fix and Dismiss available.
- The focused dismissal test also proves the pending/disabled state is released on failure and a retry succeeds, after which the persisted marker hides the card.

## E2E tests

- `apps/web/e2e/tests/chat/git-push-error-dismissal.spec.ts` in project `chromium`: after fresh root builds of web and backend, seed a legacy-shaped push failure using `ApiClient.seedSessionMessage`, dismiss it and assert the live card disappears, open a fresh page for the same task to force server-backed hydration, verify it remains hidden, switch to another task and back, then seed a different push-failure message and verify its diagnostics and Fix action appear. Backend service tests assert the persisted metadata write.
- `apps/web/e2e/tests/chat/mobile-git-push-error-dismissal.spec.ts` in project `mobile-chrome`: after fresh root builds of web and backend, repeat the live dismissal, fresh-page reload, task-switch, and later-failure flow through touch; assert Dismiss is visible without hover and its measured target is at least 44 pixels.
- Exercise unauthorized and mismatched task/session access at the service and WebSocket handler boundary; a second full browser identity fixture is unnecessary.
- Existing `task:dismiss` and `common:requestFailed` keys are present in all six supported locales, so add no locale values; run the i18n check across every locale as a compatibility gate.
- Update and validate the one-sentence `docs/public/git-operations.md` addition with `node --test scripts/validate-public-docs.test.mjs` and `node scripts/validate-public-docs.mjs`.

## Work orders

- [x] [Task 01: Persist and render message-scoped dismissal](task-01-message-scoped-dismissal.md)

## Verification results

Implemented and verified. The focused service and handler tests pass with `GOMAXPROCS=2 go test -p 1`; the three selected frontend files pass 79 tests; targeted ESLint and changed-code Go lint pass with zero issues. `pnpm run i18n:check`, both public-doc validators, specification catalog validation (305 decisions and 1,146 specifications), and full spec-file lint pass. `make build-web` and `make build-backend` pass. The desktop Chromium and mobile Chrome dismissal specs each pass, proving live dismissal, fresh-page hydration, task switching, later failure visibility, and the 44-pixel mobile target. Capture-enabled reruns produced four ignored screenshots in `apps/web/.pr-assets/manifest.json`. The web build reported Vite chunk-size, deprecated-option, and ineffective dynamic-import warnings; the backend build reported the missing macOS codesign utility, with no build failures.

## Risks

- The existing generic action-button error state is not rendered, so the new Dismiss path must supply visible localized error feedback on failed writes.
- Dismiss is client-generated from the legacy Git push metadata, so it remains available to already-stored rows without writing a new `actions` entry or changing the producer.
