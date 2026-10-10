---
status: current
system: ui
created: 2026-10-06
requirements:
  - REQ-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001
---

# Composer File Search Ownership System Design

## Purpose and boundaries

Keep file-candidate ownership inside the existing `TipTapInput` instance and
its private `fetchFileResults` / `useMentionItems` glue. UI already owns reusable
editor reply ownership in its [system boundary](../README.md). Workspaces keeps
filesystem/search authority. A local cache-key and settlement correction needs
no new framework, coordinator, store contract, transport change, or ADR; this
pair preserves the narrow rationale without imposing a repo-wide policy.

Use [mention recency](composer-mention-recency.md) for ranking and
[suggestion overlays](composer-suggestion-overlays.md) for geometry. The
[`#` entity path](entity-reference-composer.md) and
[reverse search](session-search-ownership.md) retain their independent lifetimes.

## Requirement mapping

| Acceptance | Design section |
| --- | --- |
| `AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.1` | Cache identity and committed owner |
| `AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.2` | Cache identity and committed owner; current fallback |
| `AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.3` | Settlement and candidate assembly |
| `AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.4` | Cache identity and committed owner |
| `AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.5` | Cache identity and committed owner; regression seam |
| `AC-UI-COMPOSER-FILE-SEARCH-OWNERSHIP-001.6` | Current fallback; responsive and documentation audit |

## Components and actual consumers

| File / symbols | Responsibility and disposition |
| --- | --- |
| `apps/web/components/task/chat/tiptap-input.tsx`: `fetchFileResults`, `useMentionItems`, `useSuggestionConfigs`, `TipTapInput` | Sole production correction boundary; keep private helpers private and preserve the public return shape |
| `apps/web/components/task/task-chat-panel.tsx`: `TaskChatPanel`, `ChatFooter` | Passes resolved session into `ChatInputArea`; normal active ready navigation reaches the shared composer |
| `apps/web/components/task/chat/chat-input-area.tsx`: `ChatInputArea` | Keys `ChatInputContainer` by `clarificationKey`, not session |
| `apps/web/components/task/chat/chat-input-container.tsx` and `chat-input-body.tsx` | Forward current session to `TipTapInput`; the body documents deliberate reuse across task/session changes |
| `apps/web/components/task/chat/use-tiptap-editor.ts`: `useTipTapEditor`, `TipTapInputHandle` | Existing mounted editor and public `clear` / `insertText` path; preserve |
| `apps/web/components/task/chat/tiptap-suggestion.tsx`: `createMentionSuggestion` | Actual async `@` candidate consumption and menu callbacks; preserve generic lifecycle |
| `apps/web/components/task/chat/mention-menu.tsx`: `MentionMenu` | Actual rendered file rows; preserve |
| `apps/web/components/state-provider.tsx`: `StateProvider` and `createAppStore` from `lib/state/store.ts` | Actual application state fixture; preserve |
| `apps/web/lib/ws/workspace-files.ts`: `searchWorkspaceFiles`; `lib/ws/client.ts`: `WebSocketClient`; `lib/ws/connection.ts` | Real API, request IDs and protocol settlement; substitute only wire transport in tests |
| `apps/web/hooks/use-task-create-prompt-mention.ts` | Both `useInlineMention` consumers omit session; legacy file branch is not admitted |

At the design baseline the private cache tracked query only, with
`query || "__empty__"` as a sentinel. `lastFileSearchRef` survived the committed
session change. This is the
accepted causal defect; no draft mutation, wrong submission, server leak, or
universal initial-mount policy is inferred from it.

## Cache identity and committed owner

Use an instance-local single completed entry containing full `sessionId`, exact
`query`, and `results`, with an explicit absent-entry state. Compare fields rather
than joining strings or using an empty-query sentinel. Do not normalize case,
trim, change the wire query, or change `limit=20`. No pending-request sharing or
multiple-session retention is added.

Maintain a small local committed-owner token/lifetime and current lookup token.
Publish owner changes and retire work in commit-bound layout-effect setup and
cleanup, not by mutating refs for an abandoned render. A committed session change,
including null, clears the reusable entry and retires outstanding ownership.
Unmount retires it as well. Returning A-to-B-to-A cannot revive A's old token.
Same-session ordinary rerenders update the existing source refs without clearing
the entry. StrictMode cleanup/setup must create a usable fresh lifetime rather
than leave a permanently dead mounted flag. Each instance owns its own refs.

At candidate lookup admission, capture committed session/lifetime, exact query,
and current workspace for that assembly. Advance the lookup token for each
file-candidate invocation, including cache hits, so a pending earlier search
cannot overwrite a later successful lookup or cache reuse. Null owner skips file
admission. Retirement itself neither drives an editor transaction nor searches.

## Settlement and candidate assembly

After the real file search awaits, verify the captured owner lifetime and latest
lookup token before writing the completed entry or adding returned file paths.
An older same-session different-query reply is superseded too. Returning a
response without guarding candidate assembly is insufficient: a guarded cache
write alone would still expose retired files to the actual menu consumer.

Current success uses `response.files || []` and writes the completed entry.
Retired success contributes no file paths and performs no cache write. Current
failure and no-client fallback retain their existing behavior; retired failure
has no cache-clearing, draft, or retry side effect. Preserve the already-built
task, Plan, and prompt candidates and `rankMentionItems` call with the assembly's
captured query/workspace. Do not broaden this to non-file source ownership.

This guard does not promise to suppress every generic TipTap `onStart` / `onUpdate`
from an obsolete suggestion invocation or to refresh a still-open menu merely
because props changed. The guarantee is qualified to the file results returned
by this glue and the completed cache. Tests must demonstrate current menu files
when a genuine new lookup occurs and that retired replies cannot poison later
reuse. If actual generic popup lifecycle needs a separate correction, checkpoint
ROOT with causal evidence before expanding beyond this file/glue.

## Current fallback and compatibility

Preserve the initial no-client check, ignored current search rejection, empty
results fallback, non-file assembly order, text filtering, ranking/recency and
Plan position. Do not introduce logging, aborts, retries, error UI, loading UI,
cache persistence or a public helper export. Current empty success is reusable;
failed search is not a completed successful entry. File path identities for
recency remain workspace-owned, independently of session-owned search reuse.

## Regression seam

The 21-case `apps/web/components/task/chat/tiptap-input-file-search-ownership.test.tsx`
suite renders exported `TipTapInput` under actual `StateProvider` (which creates the
actual app store), with loaded prompts and genuine shared editor/suggestion/menu.
Install a real `WebSocketClient` through the existing connection seam and replace
only the WebSocket wire transport. Resolve/reject actual outbound IDs; assert
`workspace.files.search` payload `{session_id, query, limit:20}` and rendered
`mention-menu` content. Do not mock the API, store, editor, candidate predicate,
menu, or production hooks. Restore the previous client and globals, settle owned
requests, unmount, disconnect and clear owned timers after each fixture.
The fixture saves/restores only its synthetic alpha/beta draft content using
the real local-storage API and starts each synthetic session with an empty draft.
This prevents genuine draft restoration from admitting unrelated queries; it
does not replace or change the product draft behavior. Teardown advances the
owned fake timers so the actual editor's deferred destruction runs before timers
are discarded.

Use the actual public `clear` / `insertText` typing branch for `@`; the special
programmatic `#` restriction does not apply. A faithful keyboard control can
strengthen it if practical without changing product code. Keep request and DOM
assertions causal through deferred transport and bounded act/timer flushing.

Cover current alpha positive, completed alpha-to-beta identical query with real
beta row, same-owner cache reuse, different/empty/literal-sentinel query controls,
mixed old/new success and error settlement orders, null transitions, A-to-B-to-A,
ordinary rerenders, independent editors, unmount, and StrictMode replay. Verify
that retired replies do not poison a subsequent current cache reuse, rather than
only counting API calls. Preserve non-file source and fallback controls in the
actual component. Accepted historical proof is read-only; permanent tests need
their own meaningful causal RED after later implementation admission.

## Responsive and documentation audit

This is pure state/data ownership in the existing shared composer. No layout,
touch behavior, scrolling, navigation, breakpoint, copy, or geometry changes
are admitted. `/mobile-parity` permits targeted real-component tests with this
written exception; no new phone composition, ASCII layout preview, browser,
local E2E, build or mock server is required. Desktop and phone consume the same
candidate logic and keep their current overlays.

`docs/public/developer-tools.md` already describes `@` files separately from `#`
entities. Public docs, root README and screenshot catalog searches found no
file-cache lifetime instructions to amend. This pair records the local contract
and restores the published interaction without changing API, labels, navigation
or screenshots. Audit any later demonstrated documentation gap before adding
artifacts outside this package.
