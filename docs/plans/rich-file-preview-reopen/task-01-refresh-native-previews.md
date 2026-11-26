---
id: "01-refresh-native-previews"
title: "Refresh native previews on expansion"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-001
acceptance_criteria:
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.9
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.10
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.11
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.12
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.14
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.18
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
---

# Task 01 - Refresh native previews on expansion

## Summary

The existing preview toggle refreshes native non-image content on explicit
expansion. Historical and current rich output both use `FilePreview`.

## In scope

- Start one read for the current session, repository, and path on each
  explicit expansion of a native non-image card.
- Cover current content, missing files, explicit retries, and out-of-order
  responses in the renderer tests.
- Exercise the same file edit and reopen on desktop and phone.
- Document reopening as the refresh action in the public automation guide.

## Out of scope

- A new control, background refresh, and changes to file authorization.
- Image disclosure defaults, plugin viewer behavior, and persisted snapshots.

## Acceptance

1. Opening or reopening a native non-image card starts one read. The latest
   result replaces the old content, including when the file becomes unavailable.
2. A late response from a previous read cannot replace a newer result. Closing
   the preview and unrelated rerenders make zero additional reads.
3. Target replacement, sibling isolation, image reuse, and plugin routing
   satisfy their existing tests. The preview refresh works with click and tap.

## ASCII UI preview

### UI-01 - Reopen an inline text preview

```text
  Report                    [Hide preview] [Open file]
  report.txt
  ---------------------------------------------------
  Loading preview -> Updated report text
```

On a phone below 420px, the buttons sit on a row below the path.
See the [desktop and phone preview](plan.md#ascii-ui-preview) for the
existing inline layout. The unavailable state uses the card's current copy.
This view covers criteria `.11`, `.14`, and `.18`.

## Implementation

1. In `rich-output-renderer-file-targets.test.tsx`, replace the successful
   reuse test with `reads an edited file on reopening a mounted version %i preview`.
   Run it against versions 1 and 2. Keep the real renderer and hook.
2. Verify zero reads while initially collapsed and one read after first open.
   Change the transport response, close the card, and reopen it.
   Verify a second read for the same target and current text in the same card.
3. In the metadata test, keep the one-read assertion before closing the card.
   Verify the latest result with an older deferred success and error.
   Verify that a fresh read failure shows **Preview unavailable.** Restore the
   file and verify that another explicit expansion shows the restored content.
4. Add `reopens the rich-output preview with current workspace contents` to
   both browser specs. Seed a native version 2 card with `seedRichOutputTask`.
   Edit `RICH_OUTPUT_FILE` under the test repository and restore it in `finally`.
5. Call `watchWs` before navigation. Before each open, arm a
   `workspace.file.get` response wait for the fixture path.
   Await the response and verify the visible content. Use `.tap()` on phones.
6. Run the new tests on the base code. Verify that reopening leaves the original
   content and does not send a second read.
7. Change `FilePreview` to call `load()` before opening a native non-image
   card in `handleToggle`. Keep the idle effect, the image retry, and the
   `pluginPreview` guard. Reuse `getFileCategory` for the default and toggle.
8. In `docs/public/automation-and-mcp.md`, explain how to select **Hide
   preview**, then **Preview** to refresh a text preview.

## Verification

Run from the repository root. In a fresh worktree, first run
`pnpm --dir apps install --frozen-lockfile`.

```bash
pnpm --dir apps --filter @kandev/web test rich-output-renderer-file-targets.test.tsx use-workspace-file-preview.test.ts file-preview-plugin.test.tsx rich-output-view-state.test.tsx
pnpm --dir apps/web e2e:run --project chromium tests/chat/rich-output.spec.ts -- --grep 'reopens the rich-output preview with current workspace contents'
pnpm --dir apps/web e2e:run --project mobile-chrome tests/chat/mobile-rich-output.spec.ts -- --grep 'reopens the rich-output preview with current workspace contents'
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/task/chat/messages/kandev/rich-output/file-preview-block.tsx`
- `apps/web/components/task/chat/messages/kandev/rich-output/rich-output-renderer-file-targets.test.tsx`
- `apps/web/e2e/tests/chat/rich-output.spec.ts`
- `apps/web/e2e/tests/chat/mobile-rich-output.spec.ts`
- `apps/web/e2e/tests/chat/rich-output-helpers.ts` for the shared scenario.
- `docs/public/automation-and-mcp.md`

## Dependencies

None.

## Risks

An earlier read can finish after a rapid reopen. The existing hook rejects
older settlements after a newer load starts.

## Parallelism

`sequential`

## Inputs

- [Agent rich-output requirements](../../specs/agents/requirements/agent-rich-output.md)
- [Native component design](../../specs/agents/system-design/agent-rich-output.md#native-components-and-local-state)
- [Image preview defaults](../../specs/agents/requirements/image-preview-defaults.md)
- [Previous file target work order](../rich-file-preview-targets/task-01-reset-target-local-preview.md)

## Results

Done. Native non-image cards now read the current workspace file on every
explicit expansion. Image cache/retry behavior and plugin routing remain
unchanged. The public guide explains how to refresh a text preview.

The base-code renderer, Chromium, and mobile Chrome regressions failed as
expected. After implementation, the focused renderer, hook, plugin, and
view-state suites passed all 44 tests. Both browser scenarios passed. The web
production build, typecheck, and lint passed. Public-doc validation and its 63
tests passed. The specification catalog and lint passed. `git diff --check`
passed.
