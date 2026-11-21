import { sanitizePreviewPageRoute } from "@/lib/preview-feedback-source";
import { attachmentContentUrl } from "@/lib/api/domains/attachment-api";
import type { PreviewHighlightedLayout, PreviewRegionLayout } from "@/lib/preview-inspect-bridge";
import type { PreviewCaptureRect, TaskPreviewFeedback } from "@/lib/types/http";
import type { PreviewDraftEntry } from "./use-preview-drafts";

/** A screenshot region drawn over the preview, in iframe-viewport CSS pixels. */
export type PreviewRegionView = {
  id: string;
  index: number;
  rect: { x: number; y: number; width: number; height: number };
  pending: boolean;
  /** The image is still being captured. */
  busy: boolean;
  /** The outline no longer lines up reliably with the content it was drawn over. */
  drifted: boolean;
  /** Shared with the in-page marker and the comment list. */
  number: number;
  title?: string;
  /** The captured image, once there is one. */
  imageUrl?: string;
  /** The scroll container's box the outline is clipped to, when it is partly scrolled out. */
  clip?: { x: number; y: number; width: number; height: number };
};

type RegionSource = {
  id: string;
  route: string;
  rect: PreviewCaptureRect;
  pending: boolean;
  busy: boolean;
  imageUrl?: string;
};

export function regionSources(items: TaskPreviewFeedback[], drafts: PreviewDraftEntry[]) {
  const saved: RegionSource[] = items.flatMap((item) =>
    item.kind === "screenshot" && item.capture_rect
      ? [
          {
            id: item.id,
            route: sanitizePreviewPageRoute(item.page_route),
            rect: item.capture_rect,
            pending: false,
            busy: false,
            imageUrl: item.screenshot_attachment
              ? attachmentContentUrl(item.screenshot_attachment.attachment_id)
              : undefined,
          },
        ]
      : [],
  );
  const unsaved: RegionSource[] = drafts.flatMap((draft) =>
    draft.payload.kind === "screenshot"
      ? [
          {
            id: draft.id,
            route: draft.payload.page_route,
            rect: draft.payload.capture_rect,
            pending: true,
            busy: draft.payload.screenshot === null,
            imageUrl: draft.payload.screenshot?.previewUrl,
          },
        ]
      : [],
  );
  return [...saved, ...unsaved];
}

export function regionViews(
  sources: RegionSource[],
  route: string,
  layouts: Record<string, PreviewRegionLayout>,
  labels: { comments: Record<string, string>; numbers: Map<string, number> },
): PreviewRegionView[] {
  return sources
    .filter((region) => region.route === route)
    .flatMap((region, index) => {
      const layout = layouts[region.id];
      if (layout && !layout.visible) return [];
      const { x, y, width, height } = layout?.rect ?? region.rect;
      return [
        {
          id: region.id,
          index,
          rect: { x, y, width, height },
          pending: region.pending,
          busy: region.busy,
          drifted: layout?.drifted ?? false,
          imageUrl: region.imageUrl,
          number: labels.numbers.get(region.id) ?? index + 1,
          title: labels.comments[region.id],
          clip: layout?.clip,
        },
      ];
    });
}

function lastTextRect(draft: PreviewDraftEntry["payload"]) {
  const rects = draft.text_anchor?.rects;
  return rects?.length ? rects[rects.length - 1] : null;
}

function targetRect(id: string, target: PreviewHighlightedLayout | null) {
  return target?.id === id ? target.rect : null;
}

/**
 * A saved item's editor opens beside its region outline, then its highlighted
 * marker, then where its marker was when clicked, so it never waits for the page.
 */
export function editingItemRect(
  item: TaskPreviewFeedback | null,
  regions: PreviewRegionView[],
  highlighted: PreviewHighlightedLayout | null,
  activated: PreviewHighlightedLayout | null = null,
) {
  if (!item) return null;
  const region = regions.find((candidate) => candidate.id === item.id);
  if (region) return region.rect;
  return targetRect(item.id, highlighted) ?? targetRect(item.id, activated);
}

export function openDraftRect(
  openDraft: PreviewDraftEntry | null,
  regions: PreviewRegionView[],
  activated: PreviewHighlightedLayout | null = null,
) {
  if (!openDraft) return null;
  return (
    regions.find((region) => region.id === openDraft.id)?.rect ??
    targetRect(openDraft.id, activated) ??
    openDraft.payload.capture_rect ??
    lastTextRect(openDraft.payload)
  );
}
