---
status: current
system: agents
requirements:
  - REQ-AGENTS-IMAGE-PREVIEW-EXPANSION-001
---

# Image file preview expansion design

## Boundary and mapping

This design extends the native rich-output file card. It reuses the attachment
viewer and the existing workspace read. It adds a device-local appearance preference without a protocol or
backend change. The existing [image defaults design](image-preview-defaults.md)
remains authoritative for inline disclosure and read lifetime.

REQ-AGENTS-IMAGE-PREVIEW-EXPANSION-001 maps to all sections below.

## Components and control flow

`FilePreviewContent` in
`apps/web/components/task/chat/messages/kandev/rich-output/file-preview-block.tsx`
uses `ImagePreviewDialog` for a ready binary response with an image path.
The shared viewer is defined in
`apps/web/components/task/chat/image-preview-dialog.tsx`.

Pass the existing MIME-derived data URL and `file.title || file.path`. Preserve
the bounded thumbnail classes, object containment, and non-draggable image.
Use the existing translated `task:openImage`, `task:imagePreview`, and
`task:fullSize` names. Reuse the dialog's semantic button and Radix focus behavior.
The thumbnail becomes decorative inside the named button, so update affected
image-role assertions to select the trigger and its descendant image.

Keep `useWorkspaceFilePreview` and `useRichOutputViewState` unchanged. The dialog
uses loaded bytes. It does not invoke `onOpenFile` or any agent callback. Its
open state belongs to its mounted component, outside the retained view store.
The registry already keys `FilePreview` by view, session, repository, and path.
That boundary must unmount an open dialog when its target changes.

## Desktop and mobile interaction

The closest shipped interaction is `ImagePreviewDialog`, used by attachments
and generic image results. The curated mobile file-viewer precedent supplies
the single-image focus and viewport containment. A short image inspection uses
the existing modal rather than adding a route or a choice drawer.

Both viewports keep the card header, inline image, and direct image activation.
The phone header retains its existing action row. The dialog shows one image
and a visible close control. The image sizing uses `100dvh` and object containment. The dialog uses an
explicit viewport-bounded width. A `w-fit` dialog centered with `left: 50%`
can shrink to half the available viewport and display a smaller image than
the thumbnail. Keep the desktop and `sm:` maximum-width overrides.

The transcript owns scrolling before and after inspection. During inspection,
the modal locks background scrolling and initially fits the image. The proposed
[shared image inspection design](shared-image-inspection.md) extends this with
an opt-in bounded image scroll region for actual-size inspection. Check narrow,
tall, and tiny images. Keep the close control inside the viewport and clear of
safe-area edges.

The shared trigger has a 48px minimum on phones and coarse pointers.
`ImagePreviewDialog` supplies its own translated `DialogClose` button with a
48px touch target and keeps the 24px desktop icon size. The global dialog
primitive stays unchanged. The attachment-layout browser regression covers
the shared viewer caller.

Use existing focus rings and surface tokens. The shared viewer overrides the
inherited entry and exit zoom to scale 1 under reduced motion. Opacity feedback
remains. Image content does not scale on press.

## Failure and compatibility

Loading, idle, error, non-image binary, and text branches retain their current
rendering. Opening the dialog creates no new loading state or retry request.
Image decoding failures keep the existing viewer behavior and dismissal path.
Historical v1 and current v2 normalize into the same native component.

No new dependency, schema field, metric, or ADR is required.
The [native rich-output decision](../../../decisions/2026-08-14-kandev-native-agent-rich-output.md)
continues to govern file authorization and host-owned interaction.

## Verification and documentation

Test through the real renderer with only workspace transport mocked. Cover both
versions, exact image source, no extra read or file-open callback, focus return,
and target replacement while open. Browser tests own keyboard activation,
backdrop dismissal, pointer behavior, and rendered geometry.

Extend the existing desktop and phone rich-output scenarios. Include a tiny
image trigger and a narrow fine-pointer viewport to test minimum hit areas.
Keep disclosure, retries, and retained-state regressions in the targeted suite.
Update the file-preview paragraph in `docs/public/automation-and-mcp.md` during
implementation. The [work order](../../../plans/image-preview-expansion/task-01-expand-preview-images.md)
defines exact commands and acceptance coverage.

## Appearance preference

`Show image previews by default` is a default-on device-local preference under
Settings > Preferences > Appearance. Store its saved boolean in the UI slice
and `kandev.settings.imagePreviewsExpandedByDefault` local storage. Invalid values
fall back to enabled. A dedicated settings contributor owns a local draft and
uses the shared Save and Reset controls. Apply the choice only on Save.

`FilePreview` combines this preference with the image classifier for the initial
value passed to `useRichOutputViewState`. The hook preserves retained manual
choices. No effect forces already-mounted cards open or closed. Collapsed cards
perform no preview reads until opened. `FilePreviewContent` always uses the
interactive `ImagePreviewDialog`; enlargement has no user preference.

The initial PR used a setting for enlargement. That unshipped setting is removed,
including its storage key. Do not migrate its value to a different meaning.
Register the corrected control for discovery and translate its label and
description. Use the existing touch-friendly switch row. Desktop and phone
checks cover Reset, Save, reload, initial collapse, manual preview and dialog
opening, and restored automatic disclosure after re-enabling.
