---
id: "04-e2e-and-harness"
title: "E2E coverage and harness author_type"
status: done
wave: 4
depends_on: ["03-frontend-auto-load"]
plan: "plan.md"
spec: "../../specs/ui/requirements/prompt-history-panel.md"
---

# Task 04: E2E coverage and harness author_type

## Acceptance

- The e2e harness can seed user messages: `POST /api/v1/_test/messages` accepts an optional `author_type` (default `agent`), and the e2e `seedSessionMessage` helper passes it through.
- The mobile spec asserts the seeded prompt row shows `#1` and includes a long-history Pixel scenario that uses a CDP touch-scroll gesture, holds one older-page response, observes `task:loadingOlderMessages`, releases the response, reaches `#1`, and verifies there is no button.
- The desktop spec (existing two-prompt flow) asserts row-0 shows `#2` and row-1 shows `#1`; before sending the second prompt it fetches the settled session and asserts exactly one user message exists (the seeded first prompt), pinning the ordinals deterministically.
- A new desktop spec proves both behaviors together: with a session of 121 user prompts, the panel's top row shows `#121`; the `#2` marker is not rendered before scrolling; scrolling the panel to the bottom repeatedly auto-loads older pages until the `#2` marker row and the `#1` description row render, with no button clicks and no `load-older-messages` button inside the panel.

## Verification

```bash
cd apps/backend && go test ./internal/office/testharness/...
cd apps/web && pnpm e2e:run -- e2e/tests/task/prompt-history-auto-load.spec.ts e2e/tests/task/prompt-history-panel.spec.ts
cd apps/web && pnpm e2e:run -- --project mobile-chrome --no-build -- e2e/tests/task/mobile-prompt-history-panel.spec.ts
```

The first E2E invocation is the fresh-artifact desktop run; it builds the backend and `build:e2e` frontend. The second explicitly selects `mobile-chrome` (Pixel 5) and reuses those fresh artifacts with `--no-build`; the default Chromium project ignores `mobile-*.spec.ts`. Run from a worktree with `cd apps && pnpm install --frozen-lockfile` done once.

## Files likely touched

- `apps/backend/internal/office/testharness/routes.go` — `seedMessageRequest` gains optional `AuthorType string` (`author_type`) and `CreatedAt *time.Time` (`created_at`); `seedMessageHandler` defaults author type to `models.MessageAuthorAgent`, validates/uses explicit RFC3339 timestamps when supplied, reads the seeded user row through `GetMessageWithPromptIndex`, and `publishMessageAdded` includes `prompt_index` for user rows plus `created_at` with `time.RFC3339Nano`. This keeps live harness WS events aligned with HTTP payloads.
- `apps/web/e2e/helpers/api-client.ts` — `seedSessionMessage` opts gain `authorType?: "user" | "agent"` and `createdAt?: string`; pass both fields when provided.
- `apps/web/e2e/tests/task/mobile-prompt-history-panel.spec.ts` — assert `prompt-history-number-0` contains `#1`; add a long-history test or shared helper that installs a route handler for one older-page request, holds its response before the CDP touch-scroll gesture, asserts the loading indicator, releases the response, then touch-scrolls until the `#1` row appears and asserts `load-older-messages` is absent.
- `apps/web/e2e/tests/task/prompt-history-panel.spec.ts` — add the existing desktop flow's numbering assertions: after settle, assert exactly one user message exists (the seeded first prompt); then row 0 (the UI-sent second prompt) is `#2`, row 1 (the seeded first prompt) is `#1`.
- `apps/web/e2e/tests/task/prompt-history-auto-load.spec.ts` (new) — see Tests.

## Tests

- New `prompt-history-auto-load.spec.ts`: call `test.setTimeout(180_000)` before creating the task; the 121-prompt seed plus settlement and pagination must not use the repo-wide 60-second default.
  1. `createTaskWithAgent` with `description: FIRST_PROMPT_MARKER` (becomes user prompt `#1`); poll until the session settles (`COMPLETED`/`WAITING_FOR_INPUT`).
  2. Fetch the settled session's messages; assert the session contains exactly ONE user-authored message (the boot description, which becomes prompt `#1`), making the seeded marker's absolute ordinal deterministically `#2` and the last seed `#121`. Compute the maximum `created_at` across all rows (including boot-turn agent rows), set `seedBase = maxCreatedAt + 1 second`, assign `SECOND_PROMPT_MARKER` the earliest seed timestamp (`seedBase`), then seed the remaining 119 user prompts at strictly increasing, one-microsecond-aligned timestamps at least 1 millisecond apart. Fetch the session again after seeding and assert the 120 stored user timestamps are strictly increasing before opening the panel; this satisfies repository validation and the pre-seed count assertion pins the marker's absolute ordinal.
  3. Register a route handler that continues any `/messages?before=...` request until a `panelScrollTriggered` flag is set; then `goto /t/<id>`; `waitForLoad`; `waitForDockviewReady`; open the "+" menu; click `add-panel-prompt-history-item`.
  4. Set `panelScrollTriggered=true` immediately before scrolling the panel root to `scrollHeight`. The panel may issue its own older-page request OR join an in-flight transcript/backfill request through the shared coordinator (no distinct HTTP request); assert on the shared outcome instead of holding a panel-specific request: `prompt-history-row-0` contains `#121`, `SECOND_PROMPT_MARKER` is not attached, and the panel shows `{t("task:loadingOlderMessages")}` while `isLoadingMore` is true; release any held response before continuing.
  5. Bounded loop: scroll the panel root (`prompt-history-panel`, `overflow-y-auto`) to `scrollHeight`; poll until rows grow or a short timeout; continue until `FIRST_PROMPT_MARKER`'s row is attached with `#1` (cap ~10 iterations). Then assert `SECOND_PROMPT_MARKER` is attached with `#2`. Do not assume a 20-message page: if transcript/backfill wins the shared coordinator race, its first-request-wins limit may return 100 rows.
  6. Assert `load-older-messages` testid has count 0 inside the panel.
- Extend `mobile-prompt-history-panel.spec.ts` with the same long-history seed helper: after settlement assert exactly one pre-seed user message exists, then compute `seedBase = max(created_at) + 1 second`, seed marker first at `seedBase`, seed the remaining 119 prompts at one-microsecond-aligned timestamps at least 1 millisecond apart, fetch again, and assert strict stored timestamp order. Call `test.setTimeout(180_000)`. Use a route handler armed only immediately before the CDP `Input.synthesizeScrollGesture` touch scroll; hold the panel-triggered older-page response, assert `{t("task:loadingOlderMessages")}`, release it, then repeat touch scrolling until `#1`; assert no `load-older-messages` button.
- Harness unit coverage: explicit `author_type: "user"` persists a user-author row with the computed `prompt_index`, omitted `author_type` persists `models.MessageAuthorAgent` without the field, an explicit RFC3339 `created_at` is preserved/used for deterministic ordering, and `publishMessageAdded` preserves both `prompt_index` and fractional precision with `time.RFC3339Nano`.

## Dependencies

Tasks 01–03 (the spec asserts ordinals from the backend contract and auto-load from the panel wiring).

## Parallelism

Sequential.

## Inputs

- Spec: numbering and auto-load scenarios; the mobile flow note in `Out of scope`.
- Plan: `E2E Tests`.
- Existing patterns: `seedBigConversation` in `apps/web/e2e/tests/chat/message-pagination.spec.ts` (seed-after-settle ordering, poll helpers); the desktop spec `apps/web/e2e/tests/task/prompt-history-panel.spec.ts` (add-panel flow, `SessionPage` helpers `addPanelButton`, `waitForDockviewReady`, `clickTab`); the mobile spec `mobile-prompt-history-panel.spec.ts` (Panels picker flow).

## Output contract

Summary, files changed, exact commands and results (including the harness test and each e2e spec), blockers/risks, then mark this task `done` and update its checkbox in `plan.md`.

## Results

Implemented and verified 2026-08-19.

Summary: The e2e harness `POST /api/v1/_test/messages` accepts optional `author_type` (default `agent`) and RFC3339 `created_at`; user seeds are read back through `GetMessageWithPromptIndex` so the live WS event carries the same `prompt_index` as the HTTP list path, and `publishMessageAdded` serializes `created_at` with `time.RFC3339Nano` plus `prompt_index` for user rows. `seedSessionMessage` passes `authorType`/`createdAt` through. New shared seed helper `e2e/helpers/prompt-history-long-seed.ts` (121-prompt session, pre-seed count assertion, `seedBase = max(created_at) + 1s`, marker first at `seedBase`, remaining 119 at one-microsecond-aligned timestamps ≥1ms apart, strict stored-order refetch). Desktop spec asserts exactly one pre-send user message and `#2`/`#1` on rows 0/1; mobile spec asserts `#1` and adds the long-history Pixel test (held older-page request, `task:loadingOlderMessages`, release, repeat CDP touch scrolls to `#1`, no `load-older-messages`); new `prompt-history-auto-load.spec.ts` proves both behaviors (held request → loading row → release → bounded scroll loop to `#1`, marker `#2`, no button).

Files changed: `apps/backend/internal/office/testharness/routes.go`, `apps/backend/internal/office/testharness/routes_test.go`, `apps/web/e2e/helpers/api-client.ts`, `apps/web/e2e/helpers/prompt-history-long-seed.ts` (new), `apps/web/e2e/tests/task/prompt-history-auto-load.spec.ts` (new), `apps/web/e2e/tests/task/prompt-history-panel.spec.ts`, `apps/web/e2e/tests/task/mobile-prompt-history-panel.spec.ts`.

Commands and results:
- `cd apps/backend && go test ./internal/office/testharness/...` — ok.
- `cd apps/web && pnpm e2e:run -- e2e/tests/task/prompt-history-auto-load.spec.ts e2e/tests/task/prompt-history-panel.spec.ts` — 2 passed (fresh build; the desktop numbering spec and the auto-load spec).
- `cd apps/web && pnpm e2e:run -- --project mobile-chrome --no-build -- e2e/tests/task/mobile-prompt-history-panel.spec.ts` — 2 passed (Pixel 5: jump + long-history touch-scroll).

Blockers/risks: none. Deviation note: the desktop auto-load spec holds `/messages?before=...` requests until a `panelScrollTriggered` flag; the plan's "set the flag immediately before scrolling" was implemented as release-after-assert because no transcript/backfill before-request fires automatically for a settled 121-prompt session (the last prompt is in the first page), so the panel's own sentinel request is the one held. Initial seed attempt failed with identical whole-second timestamps (the seed helper's fraction regex replaced instead of padded); fixed by padding the millisecond fraction to 6 digits.
