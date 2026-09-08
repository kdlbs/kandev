---
id: "01-prompt-duration-domain"
title: "Prompt duration domain"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/prompt-history-panel.md"
---

# Task 01: Prompt duration domain

## Acceptance

- `buildPromptHistoryEntries(messages, turns)` returns one entry per user message (`author_type === "user"`) with parseable `created_at`, newest first, each with `messageId`, `sessionId`, `content`, `sentAt` (`created_at`), `durationSeconds: number | null`, and `isLastPrompt: boolean` — TRUE only for the newest valid prompt OF THAT ENTRY'S SESSION (per-session semantics, ties broken by id ascending: with multiple sessions in the inputs, each session has exactly one `isLastPrompt` entry, and it is the one whose duration obeys the completed-turn rule).
- Duration follows the spec rules exactly: end = earlier of the prompt's own-session turn `completed_at` (only when parseable) and the next same-session prompt's `created_at`; `durationSeconds = floor(max(0, end − start) / 1000)`; `null` when no end exists or when the entry is the last prompt of its session whose turn is not completed.
- Prompt filtering happens FIRST and defines both ordering and the next-prompt bound: prompts = user messages with parseable `created_at`; the next prompt is the immediately following entry of the same session in that filtered, sorted list, so an unparseable user message can never become a next-prompt candidate. A valid prompt followed by an invalid user message is bounded by the next valid prompt.
- All associations are session-scoped: a turn bounds a prompt only when `turn.session_id === message.session_id` (matched via `turn_id`); the next-prompt bound only considers prompts of the same `session_id`. Ordering is deterministic: ascending `created_at`, ties broken by `id` ascending, then reversed for output.
- `formatPromptDuration(seconds, units)` is a pure numeric helper with INJECTED BARE LOCALIZED unit labels (`units: { s: string; m: string; h: string }` resolved from catalog keys via runtime `t()` at the call site — never module-scope `t`, never hardcoded English, since this is new user-facing copy); the helper APPENDS the numeric count (`${count}${units.s}` → `5s`) and renders `0s` / `Xs` / `Xm Ys` / `Xh Ym Zs` shapes, clamping negatives to `0s`. Tests assert en composition (`5m 23s`) with INJECTED labels ONLY — the pseudo-locale catalog-path assertion belongs to Task 02 (Wave 2), because the catalog keys are created there and Task 01 (Wave 1) runs before they exist.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- lib/prompt-history.test.ts lib/state/slices/session/turn-actions.test.ts lib/state/slices/session/remove-task-session.test.ts lib/state/hydration/hydrator.test.ts lib/ssr/session-page-state.test.ts hooks/domains/session/use-session-turns.test.ts
```

(The hydration work — `replaceSessionTurns`, the route-hydration marker in `hydrator.ts`, and `useSessionTurns` — is part of this task, so its tests must run here too; the duration-focused test alone is not sufficient.)

Table-driven cases must include, per plan `Tests > Duration derivation`: completed turn; interrupted by next prompt; next prompt before turn completion; last prompt with running turn (`null`) and with completed turn; missing `turn_id` with no later prompt (`null`); missing `turn_id` WITH a later valid prompt → bounded by that next prompt's `created_at` (the spec requires a duration when a later prompt supplies the bound, so an implementation that demands a turn must fail); DANGLING `turn_id` — a present `turn_id` ("missing") that matches NO turn in the supplied turns, followed by a valid prompt → still bounded by the later prompt (an implementation that returns `null` for any present-but-unresolved `turn_id` must fail); negative clamp to 0s; equal timestamps (deterministic order, 0s); unparseable prompt `created_at` excluded; fixture valid prompt → invalid user message → later valid prompt (invalid absent, first bounded by the later valid prompt); unparseable `completed_at` treated as absent; same-turn multiple prompts (earlier bounded by later); `turn_id` matching a turn of another session never bounds; next prompt of another session never bounds; newest-first order with id tie-break; `isLastPrompt` per-session semantics (two sessions in one input → exactly one `isLastPrompt` per session, each the session's newest valid prompt; ties broken by id); the per-session last prompt with a running turn → `null` duration; `formatPromptDuration` buckets with INJECTED labels (s, m, h, negative, zero).

## Files likely touched

- `apps/web/lib/prompt-history.ts` (new)
- `apps/web/lib/prompt-history.test.ts` (new)
- `apps/web/lib/state/slices/session/turn-actions.ts` (`replaceSessionTurns(sessionId, turns)` — UPSERT-MERGE per fetched turn reusing EXACTLY the existing `addTurn` reducer semantics (`Object.assign(existing, turn, { completed_at: existing.completed_at ?? turn.completed_at })`): `completed_at` forward-preserved (the no-downgrade invariant is scoped TO `completed_at`); `metadata` OVERWRITTEN per the existing reducer (accepted, documented — a deferred snapshot can overwrite metadata but never a completion); absent turns appended; `activeBySession` untouched)
- `apps/web/lib/state/slices/session/types.ts` (`turns.hydratedBySession: Record<string, boolean>` — the HYDRATION MARKER; array existence is NOT hydration evidence because the live `session.turn.started` WS handler's `addTurn` creates `turns.bySession[sessionId]`)
- `apps/web/lib/state/slices/session/session-slice.ts` (`defaultSessionState.turns` gains `hydratedBySession: {}` — plus a default-state assertion; typed fixtures that provide a `turns` object are updated so the hook never reads an undefined marker map; `removeTaskSession` deletes `turns.hydratedBySession[sessionId]` alongside `bySession`/`activeBySession` so a deleted session cannot leave a stale `true` that suppresses a future fetch)
- `apps/web/lib/state/default-state.ts` (`mergeInitialState` DEEP-merges `turns.hydratedBySession` — default `{}` wins for keys the SSR payload lacks; a boot payload with only `bySession`/`activeBySession` must still yield a defined marker map)
- `apps/web/lib/ssr/session-page-state.ts` (`buildSessionState` INCLUDES the marker: `hydratedBySession: { [sessionId]: true }` for the actually-merged initial session, `{}` when none — the full `TurnsState` flows through `FetchedSessionData.initialState` (`ReturnType<typeof taskToState>`) and `StateHydrator`'s `Partial<AppState>` unchanged; there is no `HydrationState`-typed route payload to thread)
- `apps/web/lib/state/hydration/hydrator.ts` (initialize `draft.turns.hydratedBySession ??= {}` before any marker write; set `turns.hydratedBySession[sessionId] = true` ONLY for sessions whose snapshot was ACTUALLY merged — forced/initial session or newly inserted; a protected active session skipped by `mergeSessionMap` keeps the marker ABSENT)
- `apps/web/lib/state/hydration/hydrator.test.ts` (EXISTS — extend: forced initial hydration sets the marker; protected active session non-merge → marker absent and `useSessionTurns` still fetches; EMPTY successful snapshot sets the marker; boot payload with turns → defined marker map, mark only after actual merge)
- `apps/web/lib/ssr/session-page-state.test.ts` (EXISTS — extend: successful initial turn hydration yields `turns.hydratedBySession: { [sessionId]: true }`; no-session/task-only state yields `{}`; a FAILED optional turn hydration does NOT claim the session hydrated)
- `apps/web/hooks/domains/session/use-session-turns.ts` (new — returns `Turn[]`; fetch `listSessionTurns` when the session's HYDRATION MARKER is absent (even if a live partial array exists), set the marker on success (including empty responses); TURNS-OWNED sequence namespace — own module-level counter/map keyed `turns:<sessionId>` or hook-local, copying the `use-session-messages.ts` pattern rather than importing its shared helpers, so message fetches cannot advance the turns sequence; PLUS a hook-local ACTIVE-SESSION GENERATION — monotonic, incremented on EVERY active-session transition, captured per request and compared before merge alongside the sequence, so a fetch for a no-longer-current generation is discarded)
- `apps/web/lib/state/slices/session/turn-actions.test.ts` (`replaceSessionTurns` preserves `activeBySession` and sets the hydration marker; MERGE — `addTurn`/`completeTurn` before the response survives the snapshot, `completed_at` not erased; a deferred snapshot with older `metadata` OVERWRITES it, documenting the existing reducer semantics)
- `apps/web/lib/state/slices/session/remove-task-session.test.ts` (EXISTS — extend: `removeTaskSession` clears `turns.hydratedBySession[sessionId]` along with `bySession`/`activeBySession`; a reintroduced session id does NOT inherit a stale `true` marker)
- `apps/web/hooks/domains/session/use-session-turns.test.ts` (new — fetch when marker absent even with a live partial array (event-before-fetch regression), no refetch after the marker is set, stale-sequence discard, CROSS-RESOURCE isolation (a message fetch committing after the turns fetch started does not discard the turns response), sibling-switch A→B hydrates B, STALE-SESSION — delayed A response after A→B is discarded and B's derived state is unaffected, DEFERRED A→B→A — A's response arriving after the return to A is discarded because the generation advanced on both transitions)
- `apps/web/components/task/prompt-history-panel-content.tsx` (consume the `useSessionTurns(sessionId)` hook's return value alongside `useSessionMessages` — NOT a raw `turns.bySession` read, so sibling hydration cannot be skipped)

## Dependencies

None.

## Parallelism

Sequential.

## Inputs

- Spec: `What` → `Duration rules` (including the robustness rules), and the duration scenarios (completed, interrupted, running last prompt, shared-turn prompts, unparseable timestamp, equal timestamps). Durations must render for completed prompts of ANY active session, not just the route-initial one (the sibling-hydration requirement).
- Plan: `Frontend > 1. Duration domain` (including the TURN HYDRATION paragraph) and `Tests > Duration derivation` / `Tests > Turn hydration`.
- Data shapes: `Message` (`id`, `session_id`, `turn_id?`, `author_type`, `content`, `created_at`) and `Turn` (`id`, `session_id`, `started_at`, `completed_at?`) from `apps/web/lib/types/http.ts`. Prompt definition mirrors `getLastUserMessageId` in `apps/web/components/task/chat/message-list-shared.tsx`. Duration formatting mirrors the private `formatDuration` in `apps/web/components/task/chat/messages/agent-status.tsx` (do not refactor that copy).
- Hydration plumbing: `listSessionTurns` at `apps/web/lib/api/domains/session-api.ts:138`; route initialization (`session-page-state.ts` plus the `task-detail-route.tsx` client fallback via `fetchSessionDataForTask`) hydrates turns for the route's INITIAL session only — sibling switches lack turn hydration, which is why the client hook is needed; the fetch-sequence pattern in `apps/web/hooks/domains/session/use-session-messages.ts` (`nextFetchSeq`/`commitFetchSeq` — COPY the pattern into a turns-owned namespace, do not import the shared helpers); the `addTurn` forward-preservation semantics in `apps/web/lib/state/slices/session/turn-actions.ts` that `replaceSessionTurns` must reuse; the live `session.turn.started` WS handler (`apps/web/lib/ws/handlers/turns.ts`) that creates `turns.bySession[sessionId]` without a snapshot (why the hydration marker is needed); `mergeSessionMap`'s active-session skip behavior in `apps/web/lib/state/hydration/merge-strategies.ts` (the marker must only be set for actually-merged sessions); `defaultSessionState.turns` in `apps/web/lib/state/slices/session/session-slice.ts` (the marker's default-state home) and the typed fixtures that provide a `turns` object (update them or the typecheck fails); the initial-state path — `mergeInitialState`'s shallow `turns` spread in `apps/web/lib/state/default-state.ts` and the SSR `buildSessionState` turns payload in `apps/web/lib/ssr/session-page-state.ts` (both must preserve a defined marker map); `removeTaskSession` in `apps/web/lib/state/slices/session/session-slice.ts` (must purge the marker).

## Risks

- Timestamps are ISO strings; parse with `new Date(...).getTime()` and treat unparseable values as absent (excluded prompt / ignored end candidate) rather than `NaN` — a naive sort comparator or subtraction produces `NaN` durations and undefined ordering.
- A message's `turn_id` may be absent (e.g. the initial task-description message); the rule "duration only when a later prompt bounds it" must not crash on the missing turn. The same `turn_id` must never match a turn of another session.

## Output contract

Summary, files changed, exact commands and results, blockers/risks, then mark this task `done` and update its checkbox in `plan.md`.
