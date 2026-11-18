"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import html2canvas from "html2canvas-pro";
import { deleteAttachment, uploadAttachment } from "@/lib/api/domains/attachment-api";
import { previewFrameMetrics, recaptureRect, type ViewportRect } from "@/lib/preview-annotation";
import { requestRegionAnchor, type PreviewScreenshotRegion } from "@/lib/preview-inspect-bridge";
import { rasterizePreviewRegion } from "@/lib/preview-screenshot";
import type {
  PreviewCaptureRect,
  TaskPreviewFeedback,
  TaskPreviewFeedbackSnapshot,
} from "@/lib/types/http";
import type { PreviewScreenshotDraft, usePreviewDrafts } from "./use-preview-drafts";

export type PreviewCaptureError = "raster" | "upload" | "workspace";

type Options = {
  iframeRef: React.RefObject<HTMLIFrameElement | null>;
  drafts: ReturnType<typeof usePreviewDrafts>;
  workspaceId: string | null;
  items: TaskPreviewFeedback[];
  replaceSaved: (
    id: string,
    expectedVersion: number,
    captureRect: PreviewCaptureRect,
    screenshotAttachmentId: string,
  ) => Promise<TaskPreviewFeedbackSnapshot | null>;
};

async function rasterize(iframe: HTMLIFrameElement | null, rect: PreviewCaptureRect) {
  const documentElement = iframe?.contentDocument?.documentElement;
  // i18n-exempt: internal raster failure; callers display localized recovery copy.
  if (!documentElement) throw new Error("Preview document is unavailable");
  const image = await rasterizePreviewRegion(documentElement, rect, (element, options) =>
    html2canvas(element, options),
  );
  return {
    ...image,
    previewUrl: URL.createObjectURL(image.blob),
    fileName: `preview-${Date.now()}.png`,
  };
}

/** Uploads screenshot bytes as a staged prompt attachment for the task workspace. */
export async function uploadPreviewScreenshot(
  workspaceId: string,
  screenshot: { blob: Blob; fileName: string },
) {
  const file = new File([screenshot.blob], screenshot.fileName, { type: "image/png" });
  const uploaded = await uploadAttachment(file, {
    workspaceId,
    kind: "image",
    deliveryMode: "prompt",
  });
  return uploaded.attachment_id;
}

/**
 * Rasterizes a moved or resized region at the page's current scroll, while
 * asking the page which element the region now sits on.
 */
async function rasterizeMoved(
  iframe: HTMLIFrameElement | null,
  previous: PreviewCaptureRect,
  rect: ViewportRect,
) {
  const metrics = iframe ? previewFrameMetrics(iframe) : null;
  // i18n-exempt: internal raster failure; callers display localized recovery copy.
  if (!iframe || !metrics) throw new Error("Preview document is unavailable");
  const unanchored = recaptureRect(previous, rect, metrics);
  const [screenshot, anchor] = await Promise.all([
    rasterize(iframe, unanchored),
    requestRegionAnchor(iframe, rect),
  ]);
  return { screenshot, captureRect: recaptureRect(previous, rect, metrics, anchor) };
}

/** Region rasterization for new captures and for moved or resized regions. */
export function usePreviewScreenshots({
  iframeRef,
  drafts,
  workspaceId,
  items,
  replaceSaved,
}: Options) {
  const [captureError, setCaptureError] = useState<PreviewCaptureError | null>(null);
  const [rasterizing, setRasterizing] = useState(0);
  const [recaptureFailed, setRecaptureFailed] = useState(false);
  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const track = useCallback(async <T>(work: () => Promise<T>) => {
    setRasterizing((count) => count + 1);
    try {
      return await work();
    } finally {
      if (mountedRef.current) setRasterizing((count) => count - 1);
    }
  }, []);

  /** Adds the region as a capturing draft at once, then fills its image or removes it. */
  const captureRegion = useCallback(
    (region: PreviewScreenshotRegion) => {
      setCaptureError(null);
      const payload = { ...region, kind: "screenshot" as const, screenshot: null };
      const id = drafts.add(payload);
      void track(() => rasterize(iframeRef.current, region.capture_rect))
        .then((screenshot) => {
          const current = drafts.find(id)?.payload;
          if (!mountedRef.current || current?.kind !== "screenshot") {
            URL.revokeObjectURL(screenshot.previewUrl);
            return;
          }
          drafts.update(id, (draft) => ({ ...draft, payload: { ...current, screenshot } }));
        })
        .catch(() => {
          if (!mountedRef.current) return;
          drafts.discard(id);
          setCaptureError("raster");
        });
    },
    [drafts, iframeRef, track],
  );

  const recaptureDraft = useCallback(
    async (draft: PreviewScreenshotDraft, id: string, rect: ViewportRect) => {
      const { screenshot, captureRect } = await track(() =>
        rasterizeMoved(iframeRef.current, draft.capture_rect, rect),
      );
      drafts.replaceScreenshot(id, { ...draft, capture_rect: captureRect, screenshot });
      return true;
    },
    [drafts, iframeRef, track],
  );

  const recaptureItem = useCallback(
    async (item: TaskPreviewFeedback, rect: ViewportRect) => {
      const previous = item.capture_rect;
      if (!workspaceId || !previous) return false;
      const { screenshot, captureRect } = await track(() =>
        rasterizeMoved(iframeRef.current, previous, rect),
      );
      URL.revokeObjectURL(screenshot.previewUrl);
      const attachmentId = await uploadPreviewScreenshot(workspaceId, screenshot);
      const replaced = await replaceSaved(item.id, item.version, captureRect, attachmentId);
      if (!replaced) void deleteAttachment(attachmentId).catch(() => undefined);
      return Boolean(replaced);
    },
    [iframeRef, replaceSaved, track, workspaceId],
  );

  /** Recaptures a draft or saved region at a new rectangle; false keeps the old one. */
  const recaptureRegion = useCallback(
    async (id: string, rect: ViewportRect) => {
      setRecaptureFailed(false);
      const draft = drafts.find(id)?.payload;
      const item = items.find((candidate) => candidate.id === id);
      let ok = false;
      try {
        if (draft?.kind === "screenshot") ok = await recaptureDraft(draft, id, rect);
        else if (item?.kind === "screenshot") ok = await recaptureItem(item, rect);
      } catch {
        ok = false;
      }
      if (!ok && mountedRef.current) setRecaptureFailed(true);
      return ok;
    },
    [drafts, items, recaptureDraft, recaptureItem],
  );

  return {
    captureError,
    setCaptureError,
    isRasterizing: rasterizing > 0,
    recaptureFailed,
    captureRegion,
    recaptureRegion,
  };
}
