---
spec: docs/specs/ui/requirements/message-metadata-overflow.md
created: 2026-08-14
status: done
---

# Implementation Plan: Message metadata dialog scroll containment

## Overview

The chat message "Message Metadata" dialog clips its entries at
`max-h-[85vh]` with no scrollbar when the debug fields exceed the dialog
height. `turn_metadata` is the last of ten fields and carries large JSON
(`runtime_config_snapshot` with a `config_options` array), so it lands below
the visible area and becomes unreachable. The fix adopts the repo's
established scrollable-dialog pattern so the entries area scrolls while the
title and close button stay visible.

## Root cause

`apps/web/components/task/chat/messages/message-actions.tsx`
(`MessageDebugDialog`, ~L192-232):

- `DialogContent className="max-h-[85vh] overflow-hidden sm:max-w-2xl"` — the
  base `DialogContent` (`@kandev/ui/dialog`) is `display: grid`.
- The entries container `div.grid.gap-3.overflow-auto.pr-1` is a grid item of
  that grid with **no height constraint**. CSS grid auto-rows grow to content
  height, so the container's `clientHeight == scrollHeight` and its
  `overflow-auto` never produces a scrollbar.
- When the combined entries exceed 85vh, `DialogContent`'s `overflow-hidden`
  clips the tail. Browser-verified with the real compiled Tailwind CSS and
  the real merged class strings (`twMerge`): dialog
  `clientHeight 508 / scrollHeight 1061`, entries container
  `clientHeight 993 == scrollHeight 993`, and the `turn_metadata` `<pre>`
  at `rectTop 803` — entirely below the visible 45–555 region, unreachable.

Why `twMerge` matters: `cn()` in `@kandev/ui` is `twMerge(clsx(...))`, which
drops the base `grid` class when a caller passes `flex`. The fix relies on
that (as `github-app-policy-dialog` already does), so the merged output is
`flex ... flex-col` with no `grid`.

## Frontend change

`apps/web/components/task/chat/messages/message-actions.tsx`
(`MessageDebugDialog`):

- `DialogContent`: `"max-h-[85vh] overflow-hidden sm:max-w-2xl"` →
  `"flex max-h-[85vh] flex-col overflow-hidden sm:max-w-2xl"` (twMerge drops
  the base `grid`; the dialog becomes a flex column).
- `DialogHeader`: add `className="shrink-0"` so the title never collapses.
- Entries container: `"grid gap-3 overflow-auto pr-1"` →
  `"grid min-h-0 flex-1 gap-3 overflow-auto pr-1"` (height-constrained flex
  child; its existing `overflow-auto` now engages). Keep `grid` on the
  container itself — the entries stay stacked as they are today.

No other production file changes. The per-field `MetadataValue` `<pre>` keeps
`max-h-[48vh] overflow-auto` unchanged.

### Mobile design contract

- **Desktop outcome:** the metadata dialog's entries area scrolls when
  content exceeds the 85vh cap; title and close button stay visible.
- **Mobile entry point:** the same info-button trigger in the chat message
  action row; the metadata dialog is a centered `Dialog` on every viewport
  (no drawer variant exists for it).
- **Nearest shipped mobile exemplar:** the dialog itself before the fix; the
  change is a pure scroll-containment fix inside the existing centered modal,
  not a composition change.
- **Presentation choice:** unchanged centered dialog; no drawer or navigation
  change.
- **Scroll owner:** the entries container is the single internal scroll
  region; the document never scrolls horizontally or vertically for this
  dialog.
- **Touch targets:** unchanged (no controls move).
- **Parity proof:** `mobile-message-metadata-overflow.spec.ts` seeds the same
  fixture, opens the dialog on the Pixel-5 viewport, and asserts the entries
  area scrolls and the dialog stays within the viewport.

## Tests

### E2E regression (RED first)

The bug is CSS layout: jsdom cannot compute layout, so the regression must be
a real-browser Playwright test.

Test-support prerequisite: the e2e harness currently seeds message metadata
but never turn metadata (`ensureSeededTurn` creates turns without metadata),
so the exact reported field cannot be exercised. Extend the harness minimally:

- `apps/backend/internal/office/testharness/routes.go`:
  `seedMessageRequest` gains `turn_metadata` (optional
  `map[string]interface{}`); `seedMessageHandler` applies it to the ensured
  turn via `repo.GetTurn` + `repo.UpdateTurn` when present.
- `apps/web/e2e/helpers/api-client.ts`: `seedSessionMessage` gains an
  optional `turnMetadata` field forwarded to the harness body.

New specs:

- `apps/web/e2e/tests/chat/message-metadata-overflow.spec.ts` (chromium
  project): seed a task, session, and a message whose `turn_metadata` carries
  a `runtime_config_snapshot` with ~40 `config_options` (the reported shape);
  open `/t/:taskId`, open the metadata dialog via the info trigger
  (`aria-label` "Show message metadata"), then assert:
  - the entries container's `scrollHeight > clientHeight` (scrollable), and
  - after scrolling the entries container to the bottom, the `turn_metadata`
    field is fully inside the dialog viewport and the dialog title is still
    visible.
  This fails on the current layout (container `scrollHeight == clientHeight`;
  `turn_metadata` below the fold) and passes after the fix.
- `apps/web/e2e/tests/chat/mobile-message-metadata-overflow.spec.ts`
  (mobile-chrome project): same fixture and assertions on the Pixel-5
  viewport, plus no document-level horizontal overflow.

### Component tests

Existing `message-actions.test.tsx` covers trigger rendering and stays green;
no new jsdom test is added because the regression is layout-only and Playwright
owns it (repo convention: pure UI behavior is proven in the browser).

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint \
  components/task/chat/messages/message-actions.tsx \
  e2e/tests/chat/message-metadata-overflow.spec.ts \
  e2e/tests/chat/mobile-message-metadata-overflow.spec.ts \
  e2e/helpers/api-client.ts)
(cd apps && pnpm --filter @kandev/web test -- \
  components/task/chat/messages/message-actions.test.tsx \
  components/task/chat/messages/message-debug-metadata.test.ts)
(cd apps/web && pnpm e2e:run --host -- tests/chat/message-metadata-overflow.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome -- \
  tests/chat/mobile-message-metadata-overflow.spec.ts)
(cd apps/backend && make test ./internal/office/testharness/...)
git diff --check
```

## Implementation waves

Single sequential wave; the harness extension, E2E specs, and the layout fix
share files and one RED-GREEN cycle. `parallel-safe`: none.

- [x] [Task 01: Seed turn metadata and prove the overflow in E2E](task-01-seed-turn-metadata-and-prove-overflow.md)
- [x] [Task 02: Make the metadata dialog entries area scroll](task-02-scrollable-metadata-dialog.md)

## Risks

- `twMerge` is load-bearing: if the merged class list ever keeps `grid`
  alongside `flex`, `display: grid` wins (verified: `.grid` sorts after
  `.flex` in the compiled CSS) and clipping returns. The E2E scroll assertion
  is the gate.
- Dropping `min-h-0` from the entries container makes the flex child refuse
  to shrink below content (`min-height: auto`), restoring the clip. The E2E
  catches it.
- The harness turn-metadata extension is test-support only and gated behind
  `KANDEV_E2E_MOCK`; it does not change production behavior.

## Open questions

None.

## Verification results

- `go test ./internal/office/testharness/` — passed (incl. new
  `TestSeedMessagePersistsTurnMetadata`).
- `gofmt -l internal/office/testharness/` — clean; `golangci-lint run
  ./internal/office/testharness/...` — 0 issues.
- RED: `message-metadata-overflow.spec.ts` failed on the current layout with
  `scrollHeight 970 == clientHeight 970` (entries container not scrollable,
  `turn_metadata` below the fold).
- GREEN: `pnpm e2e:run --host -- tests/chat/message-metadata-overflow.spec.ts`
  — 1 passed (7.6s). `pnpm e2e:run --host --project mobile-chrome --
  tests/chat/mobile-message-metadata-overflow.spec.ts` — 1 passed (8.1s).
- `pnpm --filter @kandev/web test -- components/task/chat/messages/message-actions.test.tsx components/task/chat/messages/message-debug-metadata.test.ts` — 2 files, 10 tests passed.
- `pnpm run typecheck` — passed.
- `pnpm exec eslint` on the changed frontend files — passed, 0 errors.
- `pnpm run i18n:ratchet` — clean (0 added + 1 modified file).
- `pnpm exec prettier --check` on the changed frontend files — passed after
  `--write` on the mobile spec.
- `git diff --check` — passed.

Merged class strings after `twMerge` (base + fixed `DialogContent` className):
`grid` is dropped and `flex`/`flex-col` are kept, so the dialog becomes a flex
column; `min-h-0 flex-1` on the entries container makes its `overflow-auto`
engage. Before the fix `turn_metadata` `<pre>` sat at `rectTop 803` with the
dialog's visible area ending at 555 (browser repro); after the fix the E2E
scrolls the entries container and finds the `turn_metadata` label fully inside
the dialog, with the title and close button still on screen.
