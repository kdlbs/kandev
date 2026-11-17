---
status: current
system: agents
requirements:
  - REQ-AGENTS-SHARED-IMAGE-INSPECTION-001
---

# Shared image inspection design

## Boundary and mapping

REQ-AGENTS-SHARED-IMAGE-INSPECTION-001 maps to the interaction, lifetime,
and verification sections below. This extends the
[file-card expansion design](image-preview-expansion.md), whose existing
identity and read invariants remain authoritative. Task-owned
[question image inspection](../../tasks/system-design/question-image-previews.md)
has separate shortcut, answer, and dialog contracts; do not change it merely
to share code.

## Interaction and lifetime

`ImagePreviewDialog` and its direct `ImagePreviewContent` consumer in
`context-items/image-item.tsx` cover the listed surfaces. Place the temporary
fit/actual state at the shared content lifetime, ensuring content is reset on
every opening and source change even if a dialog keeps children mounted. Use
an explicit open boundary or keyed remount as needed; verify both callers.
Keep the source unchanged, including data, blob, and existing attachment URLs.
No fetch, storage, preference, protocol, or backend change is introduced.

The existing `QuestionImageViewer` provides a fit/actual scroll pattern and
translated `task:questionImageFit`, `task:questionImageActualSize`, and
`task:questionImageCanvas` labels. Reuse them directly. Do not extract a shared
abstraction unless it is smaller and preserves question behavior. A focusable,
named image region owns overflow in actual mode; its image has natural
dimensions and no constraining max-width. Fit contains the complete image.
Controls remain outside the scroll region. Reset scroll position when changing
mode or source so the fitted image and subsequent inspection start predictably.

The existing modal is suited to short inspection and retains Radix dismissal
and focus behavior. For phone, use the question viewer's focused inspection
precedent: viewport-bounded dynamic height, safe-area clearance, one image
scroll owner, fixed accessible controls and 48px nominal touch targets. Retain
desktop compact control density and no document overflow. Both direct composer
and normal dialog entry points must receive equivalent close sizing and focus
return; the composer currently owns a controlled dialog without a
`DialogTrigger`, so explicitly restore its opener when still mounted. Avoid
changing its upload, preview URL, delivery, and removal behavior. The chip
exposes its main button for focus restoration without inserting a wrapper into
the composer's context row.

## Failure and verification

Retain the existing loading and decoding behavior and always expose dismissal.
The noninteractive branch remains a plain image. Rich-output card replacement
continues to unmount its dialog; existing loaded bytes remain the only source.
No new metrics or dependencies are needed. Component tests cover mode, source,
reset, noninteractive and direct composer lifetimes. Browser tests measure
natural dimensions, internal scrolling, focus, and containment on desktop and
phone; they also check no extra file read. The
[work order](../../../plans/shared-image-inspection/task-01-shared-inspection.md)
records exact evidence and commands.
