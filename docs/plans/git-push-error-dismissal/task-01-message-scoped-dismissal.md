---
id: "01-message-scoped-dismissal"
title: "Persist and render message-scoped dismissal"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-GIT-PUSH-ERROR-DISMISSAL-001
acceptance_criteria:
  - AC-TASKS-GIT-PUSH-ERROR-DISMISSAL-001.1
  - AC-TASKS-GIT-PUSH-ERROR-DISMISSAL-001.2
  - AC-TASKS-GIT-PUSH-ERROR-DISMISSAL-001.3
  - AC-TASKS-GIT-PUSH-ERROR-DISMISSAL-001.4
  - AC-TASKS-GIT-PUSH-ERROR-DISMISSAL-001.5
  - AC-TASKS-GIT-PUSH-ERROR-DISMISSAL-001.6
  - AC-TASKS-GIT-PUSH-ERROR-DISMISSAL-001.7
system_design:
  - ../../specs/tasks/system-design/git-push-error-dismissal.md
---

# Task 01: Persist and render message-scoped dismissal

## Summary

Add a narrow authenticated WebSocket action that persists dismissal on one Git push failure message and publishes the existing message update. Add the localized accessible control to the shared chat card, retain its diagnostics, and show the card again for any later push failure.

## In scope

- Add and register `message.dismiss_git_push_error` with request `{ "message_id": "<message-id>" }` and success payload `{ "message_id": "<message-id>", "dismissed_at": "<UTC RFC3339Nano timestamp>" }`; load the stored row and its actual session association, reject sessionless rows, authorize that session with `AuthorizeSessionAccess`, then validate the exact Git push error shape before writing.
- Add `git_operation_error_dismissed_at` to the existing message metadata in one write, preserve all diagnostic fields, and reuse `UpdateMessage` and its existing broadcast; return the existing timestamp without another write/event for repeated requests.
- Render Dismiss only on rows with `type: error`, `git_operation_error: true`, and `operation: push`, including existing legacy rows; hide the full row only after its persisted timestamp arrives.
- Filter dismissed rows before the regular/footer action split and suppress them in the shared renderer, so both transcript and footer/direct-render paths hide the full card and Fix action.
- Handle pending/failure state only for Dismiss: show the existing localized `common:requestFailed` toast, clear pending/disabled state on failure, keep the full card and both Fix/Dismiss controls retryable, and do not refactor generic `StandardActionButton` error handling.
- Add one sentence to `docs/public/git-operations.md` describing historical-message dismissal without implying Git repair or hiding later failures.
- Cover old push messages without a dismissal timestamp, unrelated errors, a later push message, desktop, phone, and touch target sizing.

## Out of scope

- Automated Git repair or status checks, polling, new persistence schema, general message editing or dismissal, undo, and changes to non-push error cards.

## Acceptance

- The action can mutate only the exact Git push failure row after its actual session passes `AuthorizeSessionAccess`; it does not erase or replace diagnostic metadata.
- A successful dismissal survives reload and task switching, while a later push failure stays visible; a failed save shows the localized toast, leaves the original card and both controls visible/enabled after pending state clears, and permits a successful retry.
- The Dismiss control is localized, keyboard accessible, visible on phone without hover, and measures at least 44 pixels on touch layouts.

## ASCII UI preview

See [UI-01: Task chat Git push error card](plan.md#ui-01-task-chat-git-push-error-card) for the full desktop and phone preview.

Phone view excerpt:

```text
[!] Git push failed
    remote rejected the branch
    [Fix]
    [Dismiss]
```

The phone view keeps both actions visible in the existing message card. Dismiss must be available through touch and keyboard and measure at least 44 pixels on touch layouts.

## Verification

```bash
(cd apps/backend && GOMAXPROCS=2 go test -p 1 ./internal/task/service -run '^TestDismissGitPushErrorMessage$' -count=1)
(cd apps/backend && GOMAXPROCS=2 go test -p 1 ./internal/task/handlers -run '^TestWSMessageDismissGitPushError$' -count=1)
(cd apps/web && pnpm exec vitest run hooks/processed-message-filtering.test.ts components/task/chat/message-renderer.test.tsx components/task/chat/messages/action-message.test.tsx)
# From the repository root, complete both fresh production builds before either browser spec.
make build-web
make build-backend
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/chat/git-push-error-dismissal.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/chat/mobile-git-push-error-dismissal.spec.ts)
(cd apps/web && pnpm run i18n:check)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

The desktop and mobile browser specs each seed a legacy-shaped failure, dismiss it through the UI, assert the live card disappears, open a fresh page to force server-backed hydration, switch away and back, then seed a later message and assert that the later diagnostics and Fix action remain visible. The mobile spec also asserts Dismiss is visible without hover and has a touch target of at least 44 pixels. Backend tests assert the exact persisted timestamp and preserved metadata, sequential idempotence, write failure, strict message eligibility, forbidden-session and cross-workspace denial, sessionless and unrelated-message rejection, and zero persistence calls/events for every rejected request. The focused action-message test makes the dismissal request fail, asserts the translated toast and both enabled controls, then retries successfully and asserts the server marker hides the full card. The existing `task:dismiss` and `common:requestFailed` keys cover all six supported locales, so the i18n check is the locale gate and no locale files should change. The public Git operations page receives one manual-dismissal sentence and is validated with both public-doc commands above.

## Files changed

- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/task/handlers/message_handlers.go`
- `apps/backend/internal/task/service/service_messages.go`
- `apps/backend/internal/task/service/service_messages_git_push_dismissal_test.go`
- `apps/backend/internal/task/handlers/message_handlers_git_push_dismissal_test.go`
- `apps/web/lib/utils/git-push-error-message.ts`
- `apps/web/hooks/processed-message-filtering.ts`
- `apps/web/hooks/processed-message-filtering.test.ts`
- `apps/web/components/task/chat/message-renderer.tsx`
- `apps/web/components/task/chat/message-renderer.test.tsx`
- `apps/web/components/task/chat/messages/action-message.tsx`
- `apps/web/components/task/chat/messages/action-message-actions.tsx`
- `apps/web/components/task/chat/messages/action-message.test.tsx`
- `apps/web/components/task/chat/messages/git-push-error-dismiss-button.tsx`
- `apps/web/e2e/tests/chat/git-push-error-dismissal.spec.ts`
- `apps/web/e2e/tests/chat/mobile-git-push-error-dismissal.spec.ts`
- `docs/public/git-operations.md` (one sentence)

## Implementation handoff

The parent accepted the reviewed action, manual Dismiss semantics, and existing workspace-read authorization. The user explicitly authorized implementation of this work order.

## Risks

- Keep failure feedback and pending-state reset scoped to the Dismiss control; generic `StandardActionButton` error handling remains unchanged.
- Message update publication is best effort after persistence, so browser verification must cover the live update and the authoritative state after reload.
- Dismiss must be generated from the legacy Git push metadata and not added as a server-persisted action, or existing rows would not gain the control.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/git-push-error-dismissal.md)
- [System design](../../specs/tasks/system-design/git-push-error-dismissal.md)
- Existing Git failure message producer, message access and update service, WebSocket message handler, shared action card, and `seedSessionMessage` E2E helper.

## Results

Implemented. `TestDismissGitPushErrorMessage` and `TestWSMessageDismissGitPushError` pass with constrained Go parallelism. The selected frontend suite passes 79 tests; targeted frontend ESLint and changed-code Go lint pass with zero issues. `pnpm run i18n:check`, both public-doc validators, specification catalog validation (305 decisions and 1,146 specifications), and full spec-file lint pass. `make build-web` and `make build-backend` pass. The Chromium and mobile Chrome E2E specs pass, covering live dismissal, fresh-page hydration, task switching, later failures, and the mobile touch target. Capture-enabled reruns produced four ignored screenshots listed in `apps/web/.pr-assets/manifest.json`.

PR review remediation: the padded-message-ID handler regression first failed against the reviewed implementation and passes after the handler now sends and returns the trimmed ID. The service regression confirms nil metadata is rejected without persistence writes or published events. These focused commands pass: `(cd apps/backend && GOMAXPROCS=2 go test -p 1 ./internal/task/handlers -run '^TestWSMessageDismissGitPushError' -count=1)` and `(cd apps/backend && GOMAXPROCS=2 go test -p 1 ./internal/task/service -run '^TestDismissGitPushErrorMessage' -count=1)`. The nil-metadata initialization and forbidden-error mapping were removed because their guards were unreachable on the existing authorization and eligibility paths. The accepted dismissal behavior, diagnostics, UI, and public docs are unchanged, so the existing desktop/mobile browser evidence remains applicable; no requirements or system-design update was needed.

The affected-package lint passes with zero issues in the task Git worktree: `(cd apps/backend && golangci-lint run ./internal/task/handlers ./internal/task/service --timeout=5m)`. The required full changed-from-base lint on the stale checkout reported one `nestif` finding in `apps/backend/internal/orchestrator/event_handlers_workflow.go:5989`; provenance checks show the path is unchanged from merge-base `b88aea31ad49b2cda40e8ad84452356888e36a42` on this branch and changed only on upstream `main` at `45cef11630d9ae48f8d903cfef654dfd14eb1cdd`. The conflict-free synthetic merge tree `38a67d4ad5d187acf0cc9d0e16c81e6f246d38b4`, with the tracked fixup diff applied, passed both focused Go test commands above. An extra lint attempt inside the synthetic archive was inconclusive because the archive has no `.git` metadata for revgrep and reported existing package findings; the task-worktree affected-package lint is the applicable lint result.
