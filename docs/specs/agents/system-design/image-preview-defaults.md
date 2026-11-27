---
status: current
system: agents
requirements:
  - REQ-AGENTS-IMAGE-PREVIEW-DEFAULTS-001
---

# Image file preview defaults design

## Boundary and mapping

This is a narrow amendment to the
[native rich-output design](agent-rich-output.md#native-components-and-local-state),
not a replacement authoring or rendering path. Implementation and targeted
verification are complete under the linked work order.

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-IMAGE-PREVIEW-DEFAULTS-001 | Default disclosure, Target lifetime, Responsive presentation, Verification |

## Default disclosure

In `apps/web/components/task/chat/messages/kandev/rich-output/file-preview-block.tsx`,
`FilePreview` supplies `getFileCategory(file.path) === "image" && imagePreviewsExpandedByDefault` as the initial
value to `useRichOutputViewState`. Reuse the existing classifier, including its
case-insensitive extensions. Do not add a second MIME/extension registry or read
the file to determine the initial disclosure.

The hook's nullish fallback preserves a retained `false` value. Apply the
default only through that existing initial-value mechanism; never force
expansion from an effect. The existing effect loads expanded idle previews,
and `handleToggle` retries an error only on explicit reopening. Preserve the
existing content renderer and transport hook, including response encoding.

Automatic expansion runs when a completed presentation's card mounts, including
history and remounts. It is not a background prefetch of unmounted transcript
items. A collapsed retained image choice causes no read on remount. A fresh page
or expired report-state entry uses the saved device preference; bytes remain workspace-owned.

## Target lifetime

Keep the registry's `(sessionId, repo, path)` component key and element identity.
Replacement mounts a new card and the workspace hook invalidates old pending
settlements. The new card uses its target-scoped retained state if present,
otherwise the classifier and saved device preference determine the default. Keep report/message/authentication view-store
scoping and duplicate-element independence. Do not reset the whole presentation
or modify `useRichOutputViewState` for this change.

Missing session or transport yields the current local unavailable state.
Automatic reads are session-authorized workspace reads, never external resource
fetches or tool callbacks. Failure does not cause an automatic retry loop.

## Responsive presentation

Entry is the completed card in task or Office chat. Reuse the shipped inline
`FilePreview` and `MobileFileViewerPanel` pattern: compact evidence stays inline,
and Open file enters the existing focused viewer. Hierarchy remains title/path,
visible hide/open actions, then bounded image or loading/error content.

At widths below 420px the header actions remain on a second row. The image keeps
`max-h-64 max-w-full object-contain`. Phone/coarse-pointer actions keep their
existing 48px nominal sizing; desktop uses existing button defaults. Preserve
the transcript scroll owner and safe-area handling of the existing viewer.
The preference reuses the existing Appearance switch row. No new overlay,
breakpoint tree, or motion is required.

## Compatibility and documentation

Both historical version 1 and current version 2 use the same native component.
No schema, catalog, message rewrite, transport, metric, or migration is needed.
The parent requirement and design link this image exception to their otherwise
lazy file-preview lifecycle. Historical
completed plans remain evidence of their prior baseline; the new plan records
this later amendment rather than rewriting their results.

The [native rich-output decision](../../../decisions/2026-08-14-kandev-native-agent-rich-output.md)
records the narrow image exception. The file-preview paragraph in
`docs/public/automation-and-mcp.md` describes the implemented default.

## Verification

Use the real Renderer/registry and transport-only mocks for component tests.
Cover auto-expansion for v1/v2 image cards, uppercase paths, lazy non-images,
retained collapse through remount and metadata changes, independent duplicates,
image/text target replacement and stale success/error settlement, successful
cache reuse, and explicit error retry. Retain existing text-target regressions.

Desktop and mobile Playwright use the existing `seedRichOutputTask` payload
override with a deterministic workspace PNG and a text card. Assert a decoded
image without clicking Preview, hide/reopen, initially collapsed text, normal
file-open routing, phone containment and real touch targets. Capture the
expanded phone view. Exact commands and AC mappings are in the work order.
