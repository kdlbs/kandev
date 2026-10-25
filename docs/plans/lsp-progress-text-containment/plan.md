---
spec: docs/specs/platform/requirements/lsp-file-intelligence.md
created: 2026-08-04
status: completed
---

# Implementation Plan: LSP Project Progress Text Containment

## Overview

The fixed-width LSP popover currently renders server-controlled project-progress text with normal word wrapping. Long URL- or path-like segments therefore retain an intrinsic width greater than the `w-80` surface and paint outside it. Add a narrow containment rule at the shared project-progress boundary, then prove the toolbar popover, status-bar popover, and tablet drawer preserve the complete message without horizontal overflow.

## Frontend

### Shared project-progress content

- Update `apps/web/components/editors/lsp-status-button.tsx` at `ProjectProgress` so the section can shrink and inherited `overflow-wrap: anywhere` applies to server-reported active and completed titles/messages.
- Keep the complete server text available in normal document flow. Do not resize the popover, add clipping, or introduce ellipsis because the message can explain which dependency or project artifact is delaying analysis.
- Leave connection/progress state, percentages, elapsed time, and lifecycle actions unchanged. The existing `LspProgressDetails` composition continues to feed the editor-toolbar popover, application-status-bar popover, and coarse-pointer drawer.

## Tests

- No unit or backend integration test is planned: the regression is browser text layout, with no new function, protocol, state, or data transformation. The Playwright geometry checks below are the direct behavioral boundary.

## E2E Tests

- **Desktop toolbar popover:** update `apps/web/e2e/tests/lsp/lsp-file-intelligence.spec.ts` so the fake Kotlin server reports a representative long artifact URL; assert the complete message remains visible and the popover's `scrollWidth` does not exceed its `clientWidth`.
- **Desktop status-bar popover:** exercise the same long active message from `app-status-lsp`, assert the upward popover remains internally contained and within the viewport, then release initialization and continue the existing placement/follow-active-editor flow.
- **Coarse-pointer tablet drawer:** update `apps/web/e2e/tests/lsp/mobile-lsp-file-intelligence.spec.ts` with the same message and assert complete text, drawer-level containment, viewport containment, one vertical scroll owner, and no document horizontal overflow.
- Put the representative server string in `apps/web/e2e/tests/lsp/lsp-e2e-helpers.ts` and add a focused element scroll-width assertion to `apps/web/e2e/helpers/layout-assertions.ts` so all three surfaces test the same contract.
- Follow Red-Green-Refactor: first run the new geometry assertions against current CSS and record the expected overflow failure; apply the containment rule only after RED; rebuild and rerun both desktop and tablet scenarios for GREEN.

## Mobile Design Contract

- **Desktop outcome:** toolbar and status-bar entry points retain their existing anchored popover and show the full server message inside its fixed width.
- **Tablet entry point and exemplar:** the existing `LspStatusDrawer` remains the nearest shipped touch surface and continues to provide the toolbar trigger, inset bottom drawer, and lifecycle action.
- **Hierarchy and action:** connection state, project analysis, elapsed/percentage detail, and the Start/Stop/Retry action remain in their current order; only text wrapping changes.
- **Surface and geometry:** the tablet drawer remains `max-h-[80dvh]` with its existing internal vertical scroll owner, safe-area behavior, and 44px controls. Arbitrary project text wraps inside the drawer and never creates document-level horizontal scrolling.
- **Shared behavior:** one `ProjectProgress` rendering path owns text containment across both popovers and the drawer. Phone CodeMirror viewing remains intentionally LSP-free.

## Verification Results

- RED desktop: the focused Chromium run failed both toolbar and status-bar scenarios at the intended assertion, with `scrollWidth: 506px` inside `clientWidth: 320px`.
- RED tablet: after widening the synthetic server token enough to exercise the 820px tablet, the focused mobile run failed at the intended assertion, with `scrollWidth: 1406px` inside `clientWidth: 750px`.
- GREEN desktop: `pnpm e2e:run tests/lsp/lsp-file-intelligence.spec.ts -- --grep "shows server-reported project work|persists status-bar placement"` — 2 passed against rebuilt production assets.
- GREEN tablet: `pnpm e2e:run --project mobile-chrome tests/lsp/mobile-lsp-file-intelligence.spec.ts -- --grep "contained tablet drawer"` — 1 passed against rebuilt production assets.
- `pnpm run typecheck` and focused ESLint both passed; `git diff --check` passed.
- Managed E2E runs tore down their isolated backends and repositories. No temporary production code, fixture mode, or tracked build artifact remains.

## PR Review Fixup

- Codex review on `adae0deb663b99f0b06b913f67943b2637f2b126` found that a bridge close during asynchronous initialization could leave the status at `starting`. Commit `6779f2cfaa9f4d1f3f6231667d4e5299156f38ee` now reports the close as an error or mapped unavailable state and adds the pending-initialize regression.
- The follow-up Codex review found that definition navigation accepted only LSP `Location` results. Commit `e0058e8440a225c9809a9ac8b350dec41ca0bc8c` normalizes `LocationLink` results with `targetUri` and `targetSelectionRange`; provider tests cover array, scalar, and mixed response shapes.
- Review-fixup verification passed: all 8 LSP unit files (80 tests), web typecheck, focused ESLint with zero warnings, and `git diff --check`.
- After the remediation push, PR #1863 was open, ready, and mergeable on exact head `e0058e8440a225c9809a9ac8b350dec41ca0bc8c`, with 0 failed and 15 pending checks, 0 unresolved or hidden review threads, and 0 actionable issue comments. The LocationLink thread was replied to and resolved.
- A later Codex pass found two editor-ownership regressions. Commit `74f6dfea3e58ab3cdc951e37aaefff6411434e87` lets unscoped navigation use a sole session-scoped CodeMirror revealer without guessing across multiple sessions, and hides the status-bar LSP control when CodeMirror owns the active editor.
- The editor-ownership RED cases failed in the expected cursor and status-item assertions. GREEN verification passed across 12 affected test files (63 tests), web typecheck, focused ESLint with zero warnings, and `git diff --check`.
- The next Codex pass found that the two-minute zero-lease timer could terminate initialization or active server-reported work. Commit `08569254a5190be9a06347f974996c66a2655470` cancels idle cleanup while either progress state is active and starts a fresh idle period when work finishes.
- Both idle-lifecycle regressions failed against the fixed timer and passed after the change. GREEN verification passed across all 8 LSP unit files (82 tests), web typecheck, focused ESLint with zero warnings, and `git diff --check`.
- A subsequent Codex pass found that unscoped navigation could not reliably target session-qualified Monaco models and could guess across task sessions. Commit `1d57d775c3e816aa732259d318d0af341f7315f2` decodes the model identity, falls back only when exactly one session owns the target, and preserves exact matching for explicitly scoped calls.
- The encoded-filename and multiple-session cases both failed before the fix and passed afterward. GREEN verification passed across 13 LSP/editor navigation test files (116 tests), web typecheck, focused ESLint with zero warnings, Prettier, and `git diff --check`.
- The next Codex pass found that `didChange` ignored incremental sync capabilities and that Kotlin's experimental qualifier bypassed i18n. Commits `51019ef5573e240cad40c2925f251acb43d6c53f` and `9207de9a4dea89e28d7437b011c91e279837e106` add capability-aware full/incremental/disabled synchronization and resolve the qualifier at render time in English and pseudo locales.
- Protocol tests cover numeric and object capability shapes, minimal UTF-16 ranges, surrogate pairs, CRLF boundaries, and disabled sync. The two-file Kotlin E2E failed on the missing range before the fixture advertised incremental sync, then passed while verifying the ranged insertion. GREEN verification also passed across 13 related unit files (130 tests), web typecheck, focused ESLint with zero warnings, Prettier, and `git diff --check`.

## Implementation Waves And Parallel Candidates

- [x] [task-01-contain-progress-text](task-01-contain-progress-text.md) — Wave 1, completed

There are no parallel candidates: the production component and all responsive regression cases express one layout contract and should move through one Red-Green-Refactor cycle.

## Risks And Boundaries

- Measure overflow with `scrollWidth` versus `clientWidth`; descendant bounding boxes alone do not detect glyphs painting outside a correctly sized paragraph box.
- Scope arbitrary wrapping to project-progress content so unrelated code/editor typography does not change.
- Popover sizing, progress semantics, lifecycle behavior, protocol messages, and phone LSP support are out of scope.
