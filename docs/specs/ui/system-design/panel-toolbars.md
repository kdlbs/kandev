---
status: draft
system: ui
requirements:
  - REQ-UI-PANEL-TOOLBARS-001
---

# Panel toolbar design

## Purpose and boundaries

UI owns shared panel geometry. Canvas lifecycle and runtime status remain
owned by Canvases and Plugins. No backend or persistence change is required.

## Requirement mapping

| Criteria | Implementation boundary |
| --- | --- |
| 001.1, 001.2 | Shared header shell |
| 001.3, 001.6 | Caller composition and overflow |
| 001.4, 001.5 | State preservation and browser regression tests |

## Shared header shell

Extend the existing `PanelToolbar`, `PanelHeaderBar`, and `PanelHeaderBarSplit`
in `apps/web/components/task/panel-primitives.tsx`. Do not introduce a competing
component. Header geometry has one source: 1.875rem on desktop and 3rem below
768px or with a coarse primary pointer. Use border-box sizing and prevent flex
shrink. Do not change `PanelFooterBar` implicitly through `PANEL_BAR_CLASS`.

Allow standard div attributes, refs, and test identifiers through the shell.
Keep semantic controls native; do not add ARIA toolbar semantics without the
required keyboard navigation. Feature wrappers retain their handlers and labels.
Remove caller height, min-height, vertical padding, and wrapping overrides that
conflict with this contract. The split layout must permit title truncation and
reserve space for an overflow trigger without clipping focus outlines.

Use shared control sizing for touch targets. Existing 24px compact desktop
controls may remain; ordinary actions use 28px. Current 32px editor controls
cannot fit the 30px row and must be reconciled. Changes to shell size alone
cannot establish correct control geometry.

## Source audit and migration inventory

This inventory records source evidence, not measured browser results.

| Surface | Current source | Disposition |
| --- | --- | --- |
| Canvas | `settings/canvas-host-route.tsx`, `canvas-host-components.tsx` | Replace embedded PageShell with shared panel header; remove status-only header |
| Changes | `task/changes-panel-header.tsx` | Already uses split shell; verify dense actions and long labels |
| Review top bar | `task/changes-top-bar.tsx` | Remove `h-auto`, wrapping and padding override; keep PR selection through compact selector/overflow |
| Files and search | `task/file-browser-toolbar.tsx`, `file-browser-search-header.tsx` | Existing shell; verify touch controls, search field and narrow-panel containment |
| Plan | `task/task-plan-panel-header.tsx` | Existing shell; preserve Implement and revision controls through overflow |
| Monaco | `editors/monaco/monaco-editor-toolbar.tsx` | Existing shell with 32px controls; normalize controls and overflow |
| CodeMirror | `editors/codemirror/codemirror-code-editor.tsx` | Existing shell; verify action and field sizing |
| Markdown and HTML previews | `task/markdown-preview-content.tsx`, `html-preview-content.tsx` | Existing shell; verify long paths and overflow |
| Browser | `task/browser-panel.tsx` | Existing shell; remove URL field's fixed minimum-width pressure; retain refresh/open/inspect |
| Mobile file viewer | `task/mobile/mobile-file-viewer-panel.tsx` | Existing shell; adopt touch height without clipping controls |
| Bottom terminal | `task/bottom-terminal-panel.tsx` | Padding-sized header; migrate header to shared shell |
| Dock terminal | `task/terminal-panel.tsx` | No independent action header; do not invent one |
| Nested diff headers | `diff/diff-header-toolbar.tsx`, `review/review-diff-toolbar.tsx`, `editors/monaco/diff-viewer-toolbar.tsx` | Content-level file controls retain independent geometry |
| Passthrough/chat | `task/passthrough-toolbar.tsx`, `task/chat/chat-input-toolbar*.tsx` | Composer and queued-message content; retain independent geometry |

Paths above are relative to `apps/web/components/`. Repeat the search for
`PanelHeaderBar`, `PanelHeaderBarSplit`, `PanelToolbar`, `border-b`, and padding-only
headers during implementation. Record newly discovered primary toolbars in the
work order and migrate them. Exclusions need a concrete role, not a filename.

## Overflow and responsive composition

A narrow Dockview panel can occur at desktop viewport widths. Select action
presentation from available row width, not just a page breakpoint. Reuse existing
menu primitives, action models, and callbacks. Prioritize title/current selection
and primary action; place secondary actions in a visible menu before overflow.
Preserve disabled explanations and tooltip/keyboard help.

Phone canvas navigation follows the existing `MobileCanvasActions` inset drawer.
Mobile file viewing follows `mobile-file-viewer-panel.tsx`; dense content remains
a focused full-height surface. Reuse `useResponsiveBreakpoint` for presentation
and CSS media conditions for pointer-dependent sizes. Keep one content scroll
owner, dynamic viewport height, safe-area clearance, and focus return on dismiss.
A geometry change must not remount an editor or reset a canvas runtime.

## Verification

Use computed border-box heights and compare neighboring toolbar boundaries,
with a 1px tolerance at a 16px root font. Check actual control hitboxes and
containment, not class names. Exercise desktop, a narrow Dockview panel, 390px,
767px, 768px, and a 900px coarse-pointer viewport. Include long labels, multiple
PRs, busy actions, editor dirty state, and canvas startup/recovery.

The work order defines exact unit and Playwright commands. No new logs, metrics,
backend authorization rules, or saved preferences are introduced.

## Implementation plan

- [Canvas authentication and panel toolbars](../../../plans/canvas-same-origin-auth/plan.md)
