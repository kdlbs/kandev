---
id: "02-remove-core-projection"
title: "Remove the core prompt projection"
status: done
wave: 2
depends_on:
  - "01-remove-panel-surfaces"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-EXTRACTION-001
acceptance_criteria:
  - AC-PLUGINS-PROMPT-HISTORY-EXTRACTION-001.2
  - AC-PLUGINS-PROMPT-HISTORY-EXTRACTION-001.3
  - AC-PLUGINS-PROMPT-HISTORY-EXTRACTION-001.4
  - AC-PLUGINS-PROMPT-HISTORY-EXTRACTION-001.5
  - AC-PLUGINS-PROMPT-HISTORY-EXTRACTION-001.8
system_design:
  - ../../specs/plugins/system-design/prompt-history-extraction.md
---

# Task 02: Remove the core prompt projection

## Summary

Delete the store slice, hooks, and derivation that existed only to feed the
removed panel, and move the transcript turn-duration helpers into a module that
does not carry the removed feature's name. Transcript merging, transcript
pagination, turn state, and the message contract stay exactly as they are.

## In scope

- Split `apps/web/lib/prompt-history.ts`. Create `apps/web/lib/turn-duration.ts`
  containing `PromptDurationUnits`, the private `epochNanoseconds` parser,
  `messageTurnDurationSeconds`, and `formatPromptDuration`, byte-for-byte
  behavior-identical. Delete the entry half: `PromptHistoryEntry`,
  `PromptWithTimestamp`, `isAgentPrompt`, `floorDivideBy1000`,
  `comparePrompts`, `turnCompletionByPrompt`, and `buildPromptHistoryEntries`.
  Delete `lib/prompt-history.ts` itself. Move the duration and formatting cases
  from `lib/prompt-history.test.ts` to `lib/turn-duration.test.ts`
  (`formatPromptDuration`, `messageTurnDurationSeconds`, and its
  shared-completed-turn case); delete the `buildPromptHistoryEntries` and
  precision-edge cases with the code they cover. Reword the docblock that moves
  with `messageTurnDurationSeconds`: it currently explains what it shares with
  and how it differs from the removed panel, which no longer exists as a
  reference.
- Update the only remaining consumer,
  `apps/web/components/task/chat/messages/message-actions.tsx`, to import the
  duration helpers from `@/lib/turn-duration`. Its rendering, copy keys, and
  test IDs do not change.
- Delete the prompt projection and its loaders:
  `apps/web/lib/state/slices/session/prompt-message-actions.ts`,
  `apps/web/hooks/domains/session/use-session-prompts.ts` with both of its test
  files (`use-session-prompts.test.ts` and
  `use-session-prompts.stability.test.tsx`),
  `apps/web/hooks/use-lazy-load-prompts.ts` and its test, and
  `apps/web/lib/state/slices/session/session-slice.prompts.test.ts`.
- Remove the slice from the store: the `session` `PromptsState` and the
  `messagePrompts` field plus every prompt-only action on `SessionSlice` in
  `lib/state/slices/session/types.ts`; the initial value, action wiring, and
  `removeTaskSession` cleanup lines in
  `lib/state/slices/session/session-slice.ts`; `mergePromptHistoryState` and the
  `messagePrompts` entry in `lib/state/default-state.ts`; and in
  `lib/state/app-state-types.ts` the `messagePrompts` field plus
  `replacePromptMessages`, `prependPromptMessages`, `setPromptMessagesLoading`,
  and `setPromptMessagesLoadingMore`.
  In `lib/state/slices/session/session-slice.ts`, reduce the docblock above
  `buildUpdateMessage` — "Builds the transcript update action and keeps the prompt
  cache in sync" — to the surviving behavior ("Builds the transcript update
  action"): the sync half describes the `updatePromptMessage` call this work order
  removes, and neither search gate nor lint can see a docblock. Do **not** touch the `PromptsState`
  re-exported by `lib/state/slices/index.ts` and `lib/state/store-reexports.ts`:
  that is the settings type for the user's saved prompts, a different contract
  that stays.
- Update the tests that asserted the slice, dropping the imports and constants only
  the removed prompt block used (for example the `Message` type import in
  `lib/state/slices/session/remove-task-session.test.ts`, which is referenced only
  by the `replacePromptMessages` seed). A leftover binding is a warn-level
  `no-unused-vars` that `eslint --max-warnings 0` rejects, and the zero-match gate
  cannot see a type import:
  `lib/state/slices/session/remove-task-session.test.ts` and
  `lib/state/slices/session/session-slice.update-messages.test.ts` keep their
  transcript assertions and drop the prompt ones.
- Delete `formatRelativeCompact` from `apps/web/lib/i18n/formats.ts`: its only
  caller is the removed panel row, it has no test, and its docblock names the
  removed duration affordance. Delete the three `common:mShort`, `common:hShort`
  and `common:dShort` keys it uses from all seven catalogs
  (`src/locales/<locale>/common.json`); `pnpm run i18n:check` fails while a
  translation catalog still carries a key the English catalog dropped (locale
  parity), and `check-i18n-keys.mjs` runs without `--strict-orphans`, so an
  unused English key alone only warns. A `_verbatim.json` declaration without a
  catalog key is a separate, hard failure in the shared catalog loader, so
  confirm none of the three was declared.
- Remove the panel-only options from
  `apps/web/hooks/use-lazy-load-sentinel.ts`:
  `joinInFlightWhileLoading`, `stickToBottomWhileLoading`, and `lifecycleKey`
  (which only the deleted panel ever set; the transcript passes `rootMargin`,
  `rearmWhileIntersecting`, `shouldContinueWhileIntersecting`,
  `isCurrentGeometryEligible`, `onLoadSettled`, and `isRequestCurrent`).
  Confirm no production caller remains before deleting each one; if one does,
  keep the option and record the caller in Results. `stickToBottomWhileLoading`
  also owns the sentinel's scroll-pin machinery: `useScrollPinnedToBottom`,
  `STICK_BOTTOM_TOLERANCE_PX`, and the `isPinned`/`refreshPinned` plumbing exist
  only to keep that option's stick write fresh. Remove them with the option —
  including the "moves the pin listener with the scroller and cleans it up on
  unmount" case, which asserts that listener directly and carries no option key,
  so neither the excess-property check nor the option lists would catch it — or
  keep them and record the reason in Results with a matching row in the design's
  Removed table. If they go, also reword the
  `// eslint-disable-next-line max-params, max-lines-per-function` rationale above
  `useLazyLoadSentinel`, which names the extracted observer/settle/pin helpers, so
  it no longer cites the pin family; a comment is invisible to both gates. Also delete the
  `minUserPromptsPerLoad` option from `apps/web/hooks/use-lazy-load-messages.ts`
  — after the panel's removal it has no production caller at all (the
  transcript's callers pass only `minTextPartsPerLoad`). Removing it also
  orphans `countUserPrompts` (`use-lazy-load-messages.ts`, referenced only by the
  removed predicate) and the `loadedPrompts` accumulator and parameter, so delete
  them too: `pnpm run lint` is `eslint --max-warnings 0` with
  `@typescript-eslint/no-unused-vars` at warn, and the work order's own lint gate
  fails until they are gone. In
  `apps/web/hooks/use-lazy-load-messages.test.ts`, delete the four
  option-bearing cases in the `minUserPromptsPerLoad` describe ("loads multiple
  pages until at least the threshold of user prompts arrives", "continues the
  accumulation loop after joining a request once it settles", "stops after a
  zero-result page even when the threshold is unmet", "stops accumulating when a
  page loads prompt #1") and keep "fetches a single page when no threshold is set
  (transcript behavior)", which passes no option and is the only assertion of the
  default. Remove the docblock and case comments that name the removed panel, and
  record the kept case in Results. Finally update the sentinel's prose that names
  the prompt-history panel: its JSDoc, and the `onUserGesture` comment below it,
  which calls the wheel/touch retry the "Panel" path and frames it as a
  short-content workaround. After the removal the only caller is the transcript's
  upward-scroll handler, so reword it as that upward-input retry while a disarmed
  sentinel is still intersecting. The same sweep covers
  `hooks/use-lazy-load-messages.ts`, whose in-flight-join comment calls the
  concurrent caller "a panel callback during transcript loading"; name the
  surviving callers instead.
  `hooks/use-lazy-load-sentinel.test.ts` needs a real cutover, not just a type
  change: `joinInFlightWhileLoading` is passed at roughly a dozen call sites and
  `stickToBottomWhileLoading` at three, so every literal must drop the removed
  key: each leftover is an excess property on an object literal typed by the
  hook's option parameter, which `tsc` rejects.
  Delete the cases whose subject is the option (the "never fires or joins while
  blocked, even with joinInFlightWhileLoading" case, the option-bearing
  cases in both `stickToBottomWhileLoading` describes — the pinned-stick case and
  the not-pinned case — the "pin refresh before a load" case, whose
  option-dependent assertion is the stick target it ends on (it also asserts
  `loadMore` was called, which survives without the option), the second
  "re-arm, disarm, and stale completions" case "serializes continuation pages
  when loading state toggles around each request", which drives `isLoadingMore`
  true in flight and therefore exercises the join itself — delete it or
  re-express it around the eligibility-retry path, and record the choice in
  Results — and the `lifecycleKey` observer-lifecycle cases), and drop the option
  from the cases where it is incidental ("failure recovery and stale
  completions", "stale observers"), which must keep passing with unchanged
  assertions. Do not delete the `stickToBottomWhileLoading` describe wholesale:
  its "does not stick without the option (transcript behavior)" case passes no
  such option and is the only case asserting the non-stick default. Re-home it
  under the transcript default-option coverage next to the existing
  "defaults exactly to the transcript margin, no re-arm, and no join" case, so
  the retained evidence covers no-join and no-stick together. Also keep the
  no-option half of "fires during an in-flight load only when
  joinInFlightWhileLoading is enabled": its hook already passes `isLoadingMore:
  true` with no options and asserts the loader was not called, which is the only
  evidence for the retained "never fire while an older page is loading" gate once
  the option goes. Re-express it as "does not fire during an in-flight load
  without the option", drop the option-bearing companion, and re-home it with the
  other default-option cases. The "defaults exactly to the transcript margin"
  case cannot carry that claim: it passes `isLoadingMore: false`.
- Leave the transcript-side prompt boundary alone:
  `hooks/use-lazy-load-messages.ts` keeps its visible-pagination stop at prompt
  `#1`, and `lib/state/slices/session/message-signature.ts` keeps its
  `prompt_index` carry-forward.

## Out of scope

- `prompt_index`, the `author_type=user` filter, live-session prompt-sequence
  allocation semantics, the initial-task-brief fallback, and backend APIs or
  schema. Backend repository changes are limited to removing sequence rows in
  the same transaction as hard deletion of their owning tasks or sessions.
- The browser conversation façade, plugin scope caches, favorite state, and
  `host.ui.PromptMentionText`.
- Saved-layout compatibility proof and browser coverage (Task 03).

## Acceptance

1. No core user-message prompt projection remains: there is no `messagePrompts`
   state, no prompt-only pagination hook, no prompt-entry builder, and no
   prompt-generation or refresh bookkeeping anywhere in the web store or hooks.
2. The transcript behaves exactly as before: turn duration on a completed user
   prompt, mixed-author merging, deletion handling, session removal cleanup, and
   visible pagination still stop at prompt `#1`. The duration helper module is
   the only home for the duration arithmetic.
3. `prompt_index` and the user-message filter still exist on the message
   contract, and `lib/state/slices/session/message-signature.ts` still carries a
   known ordinal forward. The zero-match search also proves the naming half of
   AC-001.4: no retained Host contract gains a prompt-history name, branch, or
   option. Task 03's fixture suites prove the retained-shape half.

## ASCII UI preview

No rendered surface changes in this work order. The transcript hover row keeps
its existing duration presentation; see
[UI-01 in the plan](plan.md#ascii-ui-preview) for the surface changes owned by
Task 01.

## Verification

Run from the repository root after Task 01 is complete.

```bash
(cd apps/web && pnpm exec vitest run \
  lib/turn-duration.test.ts \
  hooks/use-lazy-load-sentinel.test.ts \
  hooks/use-lazy-load-messages.test.ts \
  lib/state/slices/session \
  components/task/chat/messages/message-actions.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run lint)
(cd apps/backend && go test ./internal/task/repository/sqlite -run 'PromptIndex|InitialTaskBrief' -count=1)
rg -n -i -e "prompt[ _-]?history|promptHistory|messagePrompts|useLazyLoadPrompts|useSessionPrompts|buildPromptHistoryEntries|PROMPT_HISTORY" \
  apps/web/lib apps/web/hooks apps/web/components apps/web/src
```

The final search must return no matches. The pattern is case-insensitive and
covers the spaced title, the SCREAMING_CASE constant, and the panel-only
identifiers, so a renamed survivor still fails the gate.

## Files likely touched

- `apps/web/lib/turn-duration.ts` (new)
- `apps/web/lib/turn-duration.test.ts` (new)
- `apps/web/lib/prompt-history.ts` (delete)
- `apps/web/lib/prompt-history.test.ts` (delete)
- `apps/web/components/task/chat/messages/message-actions.tsx`
- `apps/web/lib/state/slices/session/prompt-message-actions.ts` (delete)
- `apps/web/lib/state/slices/session/session-slice.prompts.test.ts` (delete)
- `apps/web/lib/state/slices/session/types.ts`, `session-slice.ts`
- `apps/web/lib/state/slices/session/remove-task-session.test.ts`
- `apps/web/lib/state/slices/session/session-slice.update-messages.test.ts`
- `apps/web/lib/state/default-state.ts`, `app-state-types.ts`
- `apps/web/lib/state/store-reexports.ts` and `apps/web/lib/state/slices/index.ts`
  (inspected only: their `PromptsState` re-export is the settings type and is not edited)
- `apps/web/hooks/domains/session/use-session-prompts.ts` (delete),
  `use-session-prompts.test.ts`, `use-session-prompts.stability.test.tsx`
- `apps/web/hooks/use-lazy-load-prompts.ts` (delete) and its test
- `apps/web/hooks/use-lazy-load-sentinel.ts` and its test
- `apps/web/hooks/use-lazy-load-messages.ts` and its test
- `apps/web/lib/i18n/formats.ts`
- `apps/web/src/locales/{en,ja,pt-pt,pseudo,zh-cn,zh-hk,zh-tw}/common.json`

## Risks

The store and hook changes are behavior-preserving deletions, but an accidental
edit to session-slice message handling would be silent. Run the full
`lib/state/slices/session` suite and require the transcript assertions in
`session-slice.update-messages.test.ts` and `remove-task-session.test.ts` to
pass without relaxing them. Renaming the duration module must not change any
assertion in `message-actions.test.tsx`.

## Dependencies

Depends on Task 01. The panel is the last importer of these modules; deleting
them first leaves an uncompilable tree.

## Parallelism

`sequential`

## Inputs

- [Extraction design](../../specs/plugins/system-design/prompt-history-extraction.md#components-and-responsibilities)
  for the retained and removed component list.
- [Turn duration requirements](../../specs/ui/requirements/prompt-turn-duration.md)
  for the transcript behavior that must survive.
- [Host prerequisites](../../specs/plugins/requirements/prompt-history-extraction-host.md)
  for the façade and message contract that must not change.

## Results

Done. Commands run from the repository root.

Subsequent pinned-prompt integration supersedes Acceptance 1's zero-projection
outcome: the core latest-prompt projection remains for
[REQ-UI-PINNED-PROMPT-AVAILABILITY-001](../../specs/ui/requirements/pinned-prompt-availability.md),
while this work order's built-in Prompt History surface and older-page loader
remain removed. The verification below records Task 02 before that integration.

```bash
(cd apps/web && pnpm exec vitest run \
  lib/turn-duration.test.ts \
  hooks/use-lazy-load-sentinel.test.ts \
  hooks/use-lazy-load-messages.test.ts \
  lib/state/slices/session \
  components/task/chat/messages/message-actions.test.tsx)
# 30 files passed, 376 tests passed
(cd apps/web && pnpm run typecheck)   # tsc --noEmit clean
(cd apps/web && pnpm run i18n:check)  # keys OK (8664 referenced, 10973 en entries), all gates OK, six catalogs complete
(cd apps/web && pnpm run lint)        # eslint --max-warnings 0 clean
(cd apps/backend && go test ./internal/task/repository/sqlite -run 'PromptIndex|InitialTaskBrief' -count=1)
# ok  github.com/kandev/kandev/internal/task/repository/sqlite  1.489s
(cd apps/backend && go test -race ./internal/task/repository/sqlite -count=1)
# ok  github.com/kandev/kandev/internal/task/repository/sqlite  369.433s
(cd apps/backend && go test -race ./internal/task/repository/sqlite -run \
  'Test(PurgePromptSequencesForSessionsChunksLargeBatches|DeleteEphemeralTasksByAgentProfileDoesNotDeleteNewlyMatchingTask|DeleteExpiredQuickChatRemovesPromptMarkerAfterDeletingTask|ClaimInitialPromptFallbackRejectsMissingSession)$' \
  -count=1)
# ok  github.com/kandev/kandev/internal/task/repository/sqlite  2.523s
```

The PostgreSQL delete/claim interleaving test compiled but skipped because
`KANDEV_TEST_POSTGRES_DSN` was not configured in this environment.

The SQLite regressions cover bind-limit chunking, newly matching tasks during
ephemeral cleanup, conditional quick-chat deletion, and fallback claims for
deleted sessions.

The final search (`apps/web/lib`, `apps/web/hooks`, `apps/web/components`,
`apps/web/src`) returned no matches in production sources. At the branch tip the
only matches in those trees are Task 03's own fixtures, which name the retired
identity on purpose (`lib/layout/layout-profiles.test.ts`,
`lib/state/dockview-right-pane.test.ts`,
`lib/state/dockview-preset-persistence.test.ts`,
`lib/state/dockview-env-switch-action.test.ts`,
`components/task/dockview-layout-restore.test.ts`), so the naming half of 001.4
holds: no retained Host contract gained a prompt-history name, branch, or option.

### Choices recorded

- `lib/turn-duration.ts` / `lib/turn-duration.test.ts` are the new home of
  `PromptDurationUnits`, `epochNanoseconds`, `messageTurnDurationSeconds`, and
  `formatPromptDuration`. The duration and formatting cases moved with them
  (19 tests); `lib/prompt-history.ts`, its entry half, and its entry cases are
  deleted. `message-actions.tsx` renders unchanged and its suite passes
  unmodified.
- All three sentinel options had no production caller left
  (`message-list-native-scroll.ts` passes only `rootMargin`,
  `rearmWhileIntersecting`, `shouldContinueWhileIntersecting`,
  `isCurrentGeometryEligible`, `onLoadSettled`, and `isRequestCurrent`), so
  `joinInFlightWhileLoading`, `stickToBottomWhileLoading`, and `lifecycleKey`
  were removed together with `useScrollPinnedToBottom`,
  `STICK_BOTTOM_TOLERANCE_PX`, and the pin plumbing.
- `minUserPromptsPerLoad` had no production caller either
  (`components/task/chat/message-list-native.tsx` passes only
  `minTextPartsPerLoad`), so it went with `countUserPrompts` and the
  `loadedPrompts` accumulator. The kept default case was re-homed as
  `useLazyLoadMessages default targets`, and the four option-bearing cases were
  deleted.
- Sentinel test cutover: the three `lifecycleKey` cases, the blocked
  `joinInFlightWhileLoading` case, the pinned-stick case, the pin-refresh case,
  and the not-pinned stick case were deleted. The no-join half of the in-flight
  case and the no-stick case were re-homed next to the transcript
  default-option coverage as "does not fire during an in-flight load without
  the option" and "does not stick to the bottom without the option (transcript
  behavior)". The "serializes continuation pages when loading state toggles
  around each request" case was deleted rather than re-expressed around the
  eligibility-retry path: its subject was the in-flight join, and
  "retries when loading becomes eligible while the sentinel remains
  intersecting" already covers the eligibility retry. The blocked gate keeps
  evidence because that case asserts `loadMore` is not called while blocked.
  The file dropped from 36 to 27 cases, and all 27 pass.
