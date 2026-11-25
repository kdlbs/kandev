---
created: 2026-11-23
status: done
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-001
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
legacy_specs: []
---

# Refresh rich-output file previews on reopen

## Overview

The **Preview** action will read the current workspace file each time a user
opens a native non-image card. This corrects the reported stale text preview.
[GitHub issue #5449](https://github.com/kdlbs/kandev/issues/5449)

Agents owns the file-card contract shared by task and Office conversations.
This package amends the same-target criterion
[`AC-AGENTS-AGENT-RICH-OUTPUT-001.11`](../../specs/agents/requirements/agent-rich-output.md#req-agents-agent-rich-output-001-agent-rich-output)
and follows the
[native component design](../../specs/agents/system-design/agent-rich-output.md#native-components-and-local-state).
One work order delivers the change and its regression checks.

## Evidence

At `a2ef38bbfcffa2034576db9d1db0f897b19a7633`, the mount effect in
`FilePreview` loads an expanded card whose read state is `idle`.
The toggle starts a second read only from `error`.
A successful read stays `loaded` while the card is collapsed, so reopening
shows its cached content. This follows the previous reuse rule in
the [file target work package](../rich-file-preview-targets/plan.md#tests).

A temporary Vitest reproduction used the real renderer, registry, read hook,
and WebSocket request builder. A mocked socket read a disposable text file.
Both report formats showed the first contents after the file changed. Each
card sent one `workspace.file.get` request across two expansions.

## Scope

### In scope

- Explicit expansion of native non-image file cards in both report formats.
- Updated content or the existing unavailable state from the latest read.
- A public guide sentence that explains refresh through the existing controls.

### Out of scope

- Background file monitoring, polling, and a separate refresh control.
- Image preview preferences, the image dialog, plugin file viewers, and chart
  or table snapshots.
- Workspace API, catalog, and transcript storage changes.

## Technical approach

Use the existing image classifier in `file-preview-block.tsx`. On explicit
expansion, start `load()` before `setExpanded` for native non-image cards.
The existing idle effect handles cards that mount expanded.

`useWorkspaceFilePreview` already sends the current session, path, and
repository. Its request generation rejects late success or failure from a
previous read after a newer read starts. The existing card body shows the
loading state, the latest response, or the applicable unavailable state.

Reopening is the refresh action. It uses the control and workspace read that
users already use to open a preview. Successful image previews reuse content
under [the image preview requirement](../../specs/agents/requirements/image-preview-defaults.md).
Plugin cards use their permission-aware preview route.

## ASCII UI preview

### UI-01 - Reopen an inline text preview

```text
Desktop
  Report                         [Hide preview] [Open file]
  report.txt
  --------------------------------------------------------
  Updated report text

Phone below 420px
  Report
  report.txt
                 [Hide preview] [Open file]
  -----------------------------------------
  Updated report text
```

The existing **Preview** control opens the inline body. On reopening, the body
shows **Loading preview**, then current text or **Preview unavailable.**
The transcript owns page scrolling. Long text scrolls in the bounded preview
body. On phones, keep the existing touch targets and the stacked header at
widths below 420px. The sketch shows the shared card in task chat.
The renderer also serves Office conversations. The layout is illustrative.

## Tests

- `AC-AGENTS-AGENT-RICH-OUTPUT-001.11` and `.14` use the real renderer in
  `rich-output-renderer-file-targets.test.tsx`. Cover lazy open, updated text,
  out-of-order responses, unavailable content, and explicit retry.
- `AC-AGENTS-AGENT-RICH-OUTPUT-001.9`, `.10`, and `.12` use the existing
  target replacement, pending read, and independent card tests.
- `AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.5` uses the existing image reuse tests.
  `file-preview-plugin.test.tsx` covers the plugin read boundary.

## E2E tests

Add the same edited-file scenario to `rich-output.spec.ts` in `chromium` and
`mobile-rich-output.spec.ts` in `mobile-chrome`. Open the card from the
transcript, update the fixture file, and reopen that card.
Assert the second workspace read and current content for criterion `.11`.

## Work orders

- [x] [Task 01 - Refresh native previews on expansion](task-01-refresh-native-previews.md)

## Verification results

The base-code renderer regression failed for both report versions. The desktop
and phone browser regressions also failed because reopening sent no second file
read. After the fix, the focused renderer, hook, plugin, and view-state suites
passed all 44 tests. The Chromium and mobile Chrome refresh scenarios each
passed with a production web build.

The web typecheck and lint passed. Public-doc validation and its 63 tests
passed. The specification catalog and lint passed, as did `git diff --check`.

## Risks

Every reopening of a native non-image card adds a workspace read. Reuse the
existing loading state for slower executors and the existing generation guard
for rapid toggles.
