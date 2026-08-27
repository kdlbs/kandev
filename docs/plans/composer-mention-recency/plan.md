---
created: 2026-08-27
status: done
requirements:
  - REQ-UI-COMPOSER-MENTION-RECENCY-001
system_design:
  - ../../specs/ui/system-design/composer-mention-recency.md
legacy_specs: []
---

# Implementation Plan: Rank Chat Mentions by Recency

## Overview

Add one browser-local MRU list for task, saved-prompt, and file selections in
chat `@` menus. Apply the MRU rank after candidate lookup and before menu
rendering. Record selection through the shared TipTap command path.

One vertical work order owns the storage helper, rank integration, selection
wiring, unit tests, and desktop and phone E2E proof. These parts share the same
identity and ordering contract, so a split creates no independent result.

## Scope

### In scope

- Persist 50 unique chat mention identities in newest-first order.
- Rank recent task, saved-prompt, and returned file candidates before unselected
  candidates.
- Use text relevance and stable source order after recency.
- Preserve the Plan action's baseline position.
- Record pointer, touch, Enter, and Tab selections through one command path.
- Prove desktop reload persistence and phone touch parity.

### Out of scope

- Backend persistence or sync.
- Candidate lookup changes or larger file-search results.
- Recency for task creation, agent launch, `#`, or `/` menus.
- New copy, settings, recent groups, frequency scores, or time decay.

## Technical approach

### Storage and rank

- Add `apps/web/lib/chat-mention-recency.ts` and its unit test.
- Use the versioned key `kandev.chatMentionRecency.v1`.
- Normalize unknown browser data before each read and write.
- Key task and prompt entries by stable ID. Key file entries by workspace and
  path.
- Keep one 50-entry newest-first list. Move a repeated selection to the front.
- Export a pure rank helper that preserves current behavior when no history
  exists.

### Composer integration

- Pass `workspaceId` into `useMentionItems` in
  `apps/web/components/task/chat/tiptap-input.tsx`.
- Replace the local text-only `filterItems` path with the shared rank helper.
- Extend `MentionSuggestionCallbacks` with one selection callback.
- Invoke that callback from the menu command closure after TipTap inserts the
  selected mention.
- Keep `MentionMenu`, `PopupMenu`, selected-index reset, and entity-reference
  and slash-command plugins unchanged.

### Responsive contract

- Reuse the current popup on desktop and phone.
- Keep the same listbox, row size, focus behavior, touch behavior, and scroll
  owner.
- Do not add a mobile branch. Shared ranking produces the same order on both
  viewports.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-UI-COMPOSER-MENTION-RECENCY-001.1` | Pure rank unit test and desktop E2E |
| `AC-UI-COMPOSER-MENTION-RECENCY-001.2` | Unit test with a recent weaker text match |
| `AC-UI-COMPOSER-MENTION-RECENCY-001.3` | Unit tests for baseline relevance and stable ties |
| `AC-UI-COMPOSER-MENTION-RECENCY-001.4` | Selection-callback unit test and desktop/phone E2E |
| `AC-UI-COMPOSER-MENTION-RECENCY-001.5` | Pure rank unit tests with empty and filtered queries |
| `AC-UI-COMPOSER-MENTION-RECENCY-001.6` | Storage recovery unit tests and reload E2E |
| `AC-UI-COMPOSER-MENTION-RECENCY-001.7` | Identity and workspace-scope unit tests |
| `AC-UI-COMPOSER-MENTION-RECENCY-001.8` | Shared component tests and desktop/phone E2E |

Use Red-Green-Refactor. Run the new unit and E2E assertions before production
changes and record their expected failures in the work-order results.

## E2E tests

- Add `apps/web/e2e/tests/chat/mention-recency.spec.ts` for desktop.
- Seed two task candidates whose shared query gives different text scores.
- Select the weaker text match through a unique query.
- Reopen with the shared query and assert that selection recency wins.
- Reload the page and assert that the same candidate remains first.
- Add `apps/web/e2e/tests/chat/mobile-mention-recency.spec.ts` for
  `mobile-chrome`.
- Repeat the rank proof with saved prompts and touch selection in the current
  mobile popup.
- Remove disposable prompts and tasks through existing fixture cleanup or
  explicit `finally` blocks.

## Work orders

- [x] [Task 01: Rank chat mention suggestions](task-01-rank-chat-mention-suggestions.md)

## Verification results

- Red-Green-Refactor passed. The new selection-callback test failed before the
  callback wiring existed, and the new helper tests failed before the helper
  module existed.
- Focused web unit tests passed: 2 files, 25 tests, including the filtered-query
  Plan-position regression found during PR fixup.
- Managed production-build desktop and `mobile-chrome` E2E tests passed: 1 test
  each. Desktop coverage includes same-device reload persistence.
- Web typecheck and targeted web lint passed with no errors or warnings.
- Dependency installation passed with `pnpm install --frozen-lockfile`.

## Risks

- File recency can only reorder the current 20 file-search results. The helper
  must not reintroduce stale stored paths as live candidates.
- The keyboard path calls the menu command directly. Recording only the React
  row click would miss Enter and Tab.
- A comparator that reads mutable storage during pair comparisons can become
  inconsistent. Read and normalize the MRU list once per ranking operation.
- Plan can move if it participates in the MRU comparator. Capture and restore
  its baseline index instead.
- File paths can collide across workspaces. Include the workspace in file
  identities.

## Public documentation

None. The feature changes no command, configuration key, API, install flow, or
public workflow.

## Decisions

No ADR is required. This local UI feature and its system design contain the
selected ranking and persistence rules.
