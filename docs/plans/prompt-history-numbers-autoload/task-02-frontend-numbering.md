---
id: "02-frontend-numbering"
title: "Frontend prompt numbering"
status: done
wave: 2
depends_on: ["01-backend-prompt-index"]
plan: "plan.md"
spec: "../../specs/ui/requirements/prompt-history-panel.md"
---

# Task 02: Frontend prompt numbering

## Acceptance

- `Message` and the WS `MessageAddedPayload` types carry `prompt_index?: number`, and `toMessage` maps it, so messages entering the store from every channel (HTTP list, boot payload, WS add/update) keep their ordinal.
- `PromptHistoryEntry` gains `promptNumber: number | null`; `buildPromptHistoryEntries` sets it from the message's `prompt_index` (null when absent or 0).
- Prompt ordering uses a BigInt epoch-nanosecond parse normalized to UTC, but the backend-order key is `epochNanoseconds / 1000n` (pinned UTC microsecond storage precision) with message id as the tie-break. Full nanoseconds are retained separately for duration subtraction/bounds. Never compare epoch nanoseconds as Number. Mixed-width fractions, offsets, and digits 7–9 must match backend `(created_at_microsecond, id)` ordering while preserving full-nanosecond duration arithmetic.
- Each prompt-history row renders a small `#N` label at the start of the prompt bubble, in front of the prompt text (before the robot icon and the truncated text), visible in both collapsed and expanded states, with `data-testid={`prompt-history-number-${index}`}` where `index` is the 0-based rendered row index, matching the existing `prompt-history-row/duration/expand-${index}` test IDs. A row whose entry has `promptNumber === null` renders no label. No i18n keys are added (`#N` is not translatable copy; precedent: `#${pr.pr_number}` in `components/github/pr-topbar-button.tsx`, `queuePositionLabel` in `components/task/chat/queued-ghost-message.tsx`).

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- lib/prompt-history.test.ts components/task/prompt-history-panel-content.test.tsx lib/ws/handlers/messages.test.ts lib/state/slices/session/session-slice.merge-messages.test.ts
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/types/http.ts` — `Message.prompt_index?: number`.
- `apps/web/lib/types/session-events.ts` — the `MessageAddedPayload` declaration gains `prompt_index?: number`; `apps/web/lib/types/backend.ts` continues to re-export it unchanged.
- `apps/web/lib/ws/handlers/messages.ts` — `toMessage` maps `prompt_index: payload.prompt_index`.
- `apps/web/lib/prompt-history.ts` — `PromptHistoryEntry.promptNumber: number | null`; `buildPromptHistoryEntries` sets `promptNumber: prompt.prompt_index && prompt.prompt_index > 0 ? prompt.prompt_index : null`; replace millisecond-only `Date.parse` ordering with a BigInt epoch-nanosecond RFC3339 key, derive the separate microsecond-truncated ordering key, and use full BigInt subtraction for duration bounds.
- `apps/web/lib/state/slices/session/message-signature.ts` and the reconciliation path — before signature comparison, build a merged incoming snapshot that carries forward `previous.prompt_index` when `next.prompt_index === undefined`; then treat a newly present index as a meaningful snapshot difference even when `updated_at` is unchanged. Include `prompt_index` in `signatureOf`'s `updated_at` short-circuit branch as well as its content-hash fallback. This preserves a known index from older/transient payloads while allowing a later HTTP/boot snapshot to replace an unnumbered row without touching `updated_at`.
- `apps/web/components/task/prompt-history-panel-content.tsx` — `PromptHistoryRow` renders the label span at the bubble start (sibling of the text span and the expanded box, so it survives expansion), styled `text-[10px]`-scale and muted, `data-testid={`prompt-history-number-${index}`}`.
- Tests: `apps/web/lib/prompt-history.test.ts` (extend), `apps/web/components/task/prompt-history-panel-content.test.tsx` (extend), `apps/web/lib/ws/handlers/messages.test.ts` (extend), `apps/web/lib/state/slices/session/session-slice.merge-messages.test.ts` (extend).

## Tests

- `lib/prompt-history.test.ts`: an entry's `promptNumber` equals the message's `prompt_index`; absent/0/undefined → null; input ordering does not matter (the function sorts internally); ordering pairs identical through fractional digits 1–6 and differing only in digits 7–9 plus reverse-ordered IDs order by microsecond-truncated timestamp then id; mixed-width fractions (`.1Z`, `.123456789Z`, and no fraction), timezone offsets, and a legacy pair tied after microsecond truncation but distinct in full nanoseconds verify that list order uses `(microseconds,id)` while next-prompt duration uses full-nanosecond subtraction, including the expected clamp/overlap behavior: the reversed pair clamps to a zero (never negative) duration, and the newest prompt whose turn is still running (no `completed_at` and no next loaded prompt) shows no duration.
- `prompt-history-panel-content.test.tsx`: with messages carrying `prompt_index` 1..N (newest first input), rows render `#N` … `#1` (row 0 = highest number); a message without `prompt_index` renders no label; the label is present when the row is expanded.
- `lib/ws/handlers/messages.test.ts`: both `session.message.added` and `session.message.updated` payloads with `prompt_index` produce stored messages carrying it; an update preserves the ordinal when merged into an existing row.
- `lib/state/slices/session/session-slice.merge-messages.test.ts`: a refetch with unchanged `updated_at` but newly present `prompt_index` updates the stored row through the `updated_at` signature path; a later payload omitting the field does not clear an existing ordinal because reconciliation carries it forward.

## Dependencies

Task 01 (the backend must emit `prompt_index` for the end-to-end value; the unit tests here use locally built messages).

## Parallelism

Sequential.

## Inputs

- Spec: `What` numbering bullet, `API surface`, numbering scenarios.
- Plan: `Frontend > 2`.
- Existing shapes: `PromptHistoryEntry` in `apps/web/lib/prompt-history.ts`; `PromptHistoryRow` bubble markup in `apps/web/components/task/prompt-history-panel-content.tsx`; `MessageAddedPayload` used by `lib/ws/handlers/messages.ts`; `#${…}` label precedent in `components/github/pr-topbar-button.tsx` and `components/task/chat/queued-ghost-message.tsx`.

## Output contract

Summary, files changed, exact commands and results, blockers/risks, then mark this task `done` and update its checkbox in `plan.md`.

## Results

Implemented and verified 2026-08-19.

Summary: `Message` (`lib/types/http.ts`) and WS `MessageAddedPayload` (`lib/types/session-events.ts`) carry `prompt_index?: number`; `toMessage` maps it. `PromptHistoryEntry` gains `promptNumber: number | null` (from `prompt_index`, null when absent/0). `lib/prompt-history.ts` now parses RFC3339/RFC3339Nano into a full BigInt epoch-nanosecond key (fraction stripped before whole-second `Date.parse`, offset suffix retained for UTC normalization, digit run right-padded to nine digits), orders by `epochNanoseconds / 1000n` with id tie-break (BigInt, never Number), and computes duration bounds with full-nanosecond subtraction floored to seconds and clamped at zero. `message-signature.ts` includes `prompt_index` in both signature branches and carries a known ordinal forward onto payloads that omit it. The panel renders a `#N` label (`PromptNumberLabel`, `data-testid="prompt-history-number-<index>"`) at the bubble start before the robot icon, visible in collapsed and expanded states, only when `promptNumber !== null`; no i18n keys (precedent `#${pr.pr_number}`).

Files changed: `apps/web/lib/types/http.ts`, `apps/web/lib/types/session-events.ts`, `apps/web/lib/ws/handlers/messages.ts`, `apps/web/lib/prompt-history.ts`, `apps/web/lib/state/slices/session/message-signature.ts`, `apps/web/components/task/prompt-history-panel-content.tsx`, tests: `lib/prompt-history.test.ts`, `components/task/prompt-history-panel-content.test.tsx`, `lib/ws/handlers/messages.test.ts`, `lib/state/slices/session/session-slice.merge-messages.test.ts`.

Commands and results:
- `cd apps && pnpm --filter @kandev/web test -- lib/prompt-history.test.ts components/task/prompt-history-panel-content.test.tsx lib/ws/handlers/messages.test.ts lib/state/slices/session/session-slice.merge-messages.test.ts` — 4 files, 75 tests passed.
- `cd apps/web && pnpm run typecheck` — ok (ES2017 target: BigInt via `BigInt(...)` calls, no `n` literals).
- eslint (default + i18n configs) on changed files — 0 problems.

Blockers/risks: none. Note: the SQLite driver stores `YYYY-MM-DD HH:MM:SS[.fffffffff]+00:00` text; the backend's normalized key is the authority, and the frontend floor division `epochNanoseconds / 1000n` was verified against it in Task 01 fixtures.
