---
spec: docs/specs/ui/requirements/message-queue-auto-merge.md
created: 2026-08-12
status: completed
---

# Implementation Plan: Automatically Merge Consecutive Queued Messages

## Overview

Add a default-on, install-wide automatic-merge setting without changing the
manual merge switch. Build the strict tail-compatibility mutation first, route
ordinary admission and staged attachment admission through it safely, then add
settings persistence and UI. Finish with desktop/mobile E2E coverage and public
documentation.

---

## Backend

### Automatic tail-merge contract

- Add a repository operation in
  `apps/backend/internal/orchestrator/messagequeue/repository.go` that targets
  one newly inserted source ID and its immediate predecessor. It returns the
  surviving target plus whether a fold occurred; incompatibility is a normal
  `false` result, not `ErrNoMergeTarget`.
- Implement it under the existing per-session lock in
  `repository_memory.go` and as one transaction in `repository_sqlite.go`
  (the database-backed implementation also serves the env-gated Postgres
  suite). Select by source ID and session, select only the greatest lower
  position, compute the complete candidate, update the target, and delete the
  exact source. A missing/drained source never causes a search for another
  source.
- Keep manual `MergeIntoAbove` and `mergeAllowed` unchanged. Add automatic-only
  helpers in `merge.go`:
  - source compatibility: same task/model/plan mode; user identities equal and
    non-reserved, or agent identities equal with one identical non-empty
    `sender_task_id`; lifecycle/workflow/server/clarification rows excluded;
  - non-additive metadata comparison after normalizing JSON-round-tripped map
    values;
  - stable entity-reference and context-file unions;
  - combined attachment count/byte validation using the same limits and byte
    accounting as send-now;
  - content joining that adds `\n\n` only when both bodies are non-empty.
- Preserve the target's immutable fields and target-then-source order. Any
  mismatch or limit overflow leaves both rows unchanged and returns a skipped
  result.

### Admission integration

- Extend `messagequeue.Service` in `service.go` with a separate atomic
  `autoMergeEnabled` value, defaulting to true, plus getters/setters. Do not
  reuse `mergeEnabled`; test all four switch combinations.
- Within `QueueMessageWithMetadata`, snapshot capacity and automatic-merge
  state under `WithSessionAdmission`, perform the existing capped insert, then
  attempt the repository fold. Capacity remains an admission prerequisite.
  Return the surviving target on success and the source on skip.
- Treat a post-insert automatic-fold storage error as degraded success: the
  transaction leaves the accepted source separate, the service logs the
  failure, and callers do not receive an error that encourages a duplicate
  retry.
- Add a narrow deferred-finalization path for user WebSocket admissions that
  contain staged file-backed attachments. Hold `WithSessionAdmission` across
  insertion, claim, fold, and rollback so no later admission can merge into the
  provisional source. `queue_handlers.go` must insert a distinct source first,
  claim its attachments, and only then finalize the snapshotted
  automatic-merge decision. On claim failure, the existing rollback removes
  only that source ID. Inline attachments can use ordinary admission.
- Preserve exact-ID consumers. `QueueAndInterruptForPeerMessage` and
  `message_task_kandev` use the returned surviving ID, so an immediate parent
  interrupt takes the complete merged entry. Workflow/coalesce, restoration,
  explicit append, clarification, and reserved lifecycle paths remain excluded
  by their APIs or compatibility checks.
- Publish `message.queue.status_changed` only after the final merge-or-separate
  state is known. Do not add `auto_merge_enabled` to `QueueStatus`; the queue
  panel has no setting control.

### Persistent settings and startup wiring

- Extend `apps/backend/internal/system/queuesettings/types.go` with
  `AutoMergeEnabled` on `Settings`, `SettingsPatch`, and `Effective`. Apply and
  validate it like the independent manual merge boolean.
- Extend `store.go`'s pointer-backed legacy decoder so an omitted
  `auto_merge_enabled` becomes true while an explicit false survives. Saving
  writes the normalized three-field object.
- Extend `resolver.go` and `service.go` so GET/PATCH round-trip the field,
  partial updates preserve both other values, max-capacity environment locks do
  not block an auto-only patch, persistence precedes live apply, and the queue
  target receives all three effective values. In particular, an auto-only or
  manual-only PATCH under an environment capacity override must reapply the
  resolved environment capacity, not the lower persisted capacity.
- In `apps/backend/internal/backendapp/orchestrator.go`, resolve and apply the
  saved automatic setting before the orchestrator accepts work. Update
  `message_queue_settings_test.go`, `system_routes_test.go`, and
  `internal/system/queuesettings/queuesettings_test.go` for default, upgrade,
  explicit false, partial PATCH, environment lock, restart, and live-apply
  cases.

---

## Frontend

### Settings API and types

- Add `auto_merge_enabled` to `MessageQueueSettingsValue` in
  `apps/web/lib/types/system.ts`. The existing partial patch type then accepts
  an auto-only update without changing the HTTP client contract.
- Update `apps/web/lib/api/domains/settings-api.test.ts` fixtures and assertions
  to prove the field is decoded and sent alone.

### Message Queue settings card

- Extend `useMessageQueueSettingsLoad` and
  `useMessageQueueSettingsDraft` in
  `apps/web/components/settings/system/message-queue-settings.tsx` with a third
  draft/baseline. Include it in contributor revision, dirty/valid/save/discard
  logic, and concurrency-safe response reconciliation.
- Add a separate `AutoMergeToggleFields` presentation block below the existing
  manual merge block. Use the label **Automatically merge consecutive
  messages**, explanatory same-source/fallback copy, a stable test ID, and the
  existing admin/member and shared floating-save behavior.
- Keep the current Task Behavior page, card width, single settings scroll owner,
  and one-column responsive layout. The switch and surrounding row must remain
  touch-safe and clear the mobile save bar. No queue-panel component changes.
- Add all user-facing copy to `apps/web/src/locales/en/system.json`, regenerate
  the pseudo catalog, and run i18n checks and the changed-line ratchet.

---

## Tests

- **What:** strict same-source compatibility, identity preservation, ordered
  content/context/attachment merge, metadata mismatch fallback, combined-limit
  fallback, and no half-write.
  **File:** new
  `apps/backend/internal/orchestrator/messagequeue/repository_auto_merge_test.go`
  plus env-gated Postgres cases.
  **How:** table-drive the same contract over memory, SQLite, and Postgres;
  compare complete pre/post snapshots on skipped and failing paths.
- **What:** default-on service admission, disabled behavior, three-message
  chaining, cap-before-merge, concurrent admissions/drain, degraded repository
  error, and independence from manual merge.
  **File:** new `service_auto_merge_test.go` and focused updates to existing
  queue tests that intentionally need multiple compatible rows.
  **How:** RED service tests first, then race tests using the existing admission
  lock and repository hooks.
- **What:** file-backed attachment claim succeeds before fold and claim failure
  removes only the new source.
  **File:** `queue_handlers_test.go` or a new
  `queue_handlers_auto_merge_test.go`.
  **How:** handler-to-real-memory-service tests with blocking/failing attachment
  claim fakes and byte-for-byte target assertions.
- **What:** returned surviving IDs remain valid for normal peer queueing and
  parent interrupt dispatch, while different senders, clarification, workflow,
  lifecycle, plugin-source mismatch, append, coalesce, and restore do not
  conflate.
  **File:** focused cases in `internal/orchestrator`, `internal/mcp/handlers`,
  and `internal/backendapp` tests.
  **How:** integration tests through existing real service fixtures.
- **What:** settings default/upgrade/persistence/partial PATCH/live apply and
  environment-lock independence, including retention of the environment's live
  capacity after an auto-only PATCH.
  **File:** `internal/system/queuesettings/queuesettings_test.go`,
  `internal/backendapp/message_queue_settings_test.go`, and
  `internal/system/system_routes_test.go`.
  **How:** store, service, HTTP, and startup wiring tests.
- **What:** third draft loads, saves alone, discards, survives a concurrent
  response, remains editable under only a max environment lock, and is disabled
  for members.
  **File:** `message-queue-settings.test.tsx` and `settings-api.test.ts`.
  **How:** component tests with the existing save-provider harness and API
  mocks.

---

## E2E Tests

- **Scenario:** default-on setting persists and can be switched off/on without
  changing capacity or manual merge.
  **File:** `apps/web/e2e/tests/system/message-queue-settings.spec.ts`.
  **What to verify:** GET default, auto-only PATCH body, shared save bar,
  reload, environment-cap lock independence, member read-only state, and full
  baseline restoration in `afterEach`.
- **Scenario:** two compatible user messages become one when enabled and remain
  two when disabled.
  **File:** `apps/web/e2e/tests/chat/message-queue.spec.ts`.
  **What to verify:** busy agent, final queue count/content/order, no transient
  second row after the response, and setting restoration even on failure.
- **Scenario:** mobile exposes the same touch-safe switch without overflow,
  nested scroll ownership, or save-bar overlap.
  **File:**
  `apps/web/e2e/tests/system/mobile-message-queue-settings.spec.ts`.
  **What to verify:** visible switch, minimum touch size, save/discard, and
  persisted value.
- Existing management, manual merge, send-now, and reorder E2E scenarios that
  require multiple compatible rows explicitly disable auto merge for their
  setup and restore the prior install value during teardown. Update desktop and
  `mobile-*` specs together.

---

## Verification Results

- Backend repository, admission, handler, MCP, settings, startup, and race suites
  passed, including memory, SQLite, and environment-gated Postgres contracts.
- Frontend passed 34 focused component/API tests, typecheck, focused ESLint,
  pseudo-locale generation, `i18n:check`, and `i18n:ratchet`.
- Desktop Chromium E2E passed 20 tests; mobile Chromium passed 4 tests; the
  focused attribution, steering, and sidebar-count audit passed 3 tests.
- Public documentation tests passed 60 tests, the validator accepted 41
  published pages, and `git diff --check` passed.
- No blockers remain.

Review remediation verification also passed the focused automatic-merge
repository/service tests and the message-queue race suite after adding the
context-file cap and shared admission serialization for queue mutations.

---

## Implementation Waves And Parallel Candidates

```text
Wave 1:
- [x] [task-01-auto-merge-repository](task-01-auto-merge-repository.md)

Wave 2:
- [x] [task-02-auto-merge-admission](task-02-auto-merge-admission.md)

Wave 3:
- [x] [task-03-auto-merge-settings](task-03-auto-merge-settings.md)

Wave 4:
- [x] [task-04-auto-merge-settings-ui](task-04-auto-merge-settings-ui.md)

Wave 5:
- [x] [task-05-auto-merge-e2e](task-05-auto-merge-e2e.md)

Wave 6:
- [x] [task-06-auto-merge-docs](task-06-auto-merge-docs.md)
```

The dependency chain is sequential in the primary conversation. No wave
authorizes subagents; only a later explicit user request can do that.

## Environment prerequisite

If `apps/node_modules` is absent, run `pnpm install --frozen-lockfile` from
`apps/` before frontend or E2E checks. Do not change the lockfile.

## Risks and boundaries

- Automatic admission must not call the manual setting gate or loosen manual
  merge provenance rules.
- File attachment claim rollback is destructive to the just-inserted source ID;
  automatic fold must never run before that claim succeeds, and a later
  admission must never use the provisional source as its target.
- A post-insert fold error cannot be returned as an admission failure because a
  caller retry would duplicate accepted content.
- Existing tests frequently queue multiple compatible user rows. They must opt
  out explicitly when row count is part of the scenario, while product defaults
  stay on.
- No schema migration, feature flag, environment variable, retroactive sweep,
  queue-panel toggle, or capacity bypass is planned.
