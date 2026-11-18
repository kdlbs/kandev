"use client";

import { useCallback, useEffect, useMemo, useRef } from "react";
import { useTranslation } from "react-i18next";
import { RegionOverlay } from "@/components/shared/region-annotation/region-overlay";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import type { PreviewCaptureController } from "@/hooks/use-preview-capture";
import { MAX_PREVIEW_SCREENSHOTS } from "@/lib/preview-annotation";
import { PreviewAnnotationToolbar } from "./preview-annotation-toolbar";
import {
  PreviewDraftSurface,
  PreviewSavedRegionEditor,
  PreviewSelectionComment,
} from "./preview-annotation-surfaces";
import { PreviewFeedbackCollectionSurface } from "./preview-feedback-collection";
import { useRegionDrag } from "./use-region-drag";

type Props = {
  capture: PreviewCaptureController;
  taskId: string | null | undefined;
};

/** Escape typed in Kandev while the preview owns focus steps out of the mode. */
function useLayerEscape(
  capture: PreviewCaptureController,
  root: React.RefObject<HTMLDivElement | null>,
) {
  const { active } = capture.annotation;
  const { handleEscape } = capture;
  useEffect(() => {
    if (!active) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      const target = event.target instanceof Node ? event.target : null;
      const inPreview = target === document.body || (target && root.current?.contains(target));
      if (inPreview) handleEscape();
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [active, handleEscape, root]);
}

function AnnotationStatus({ capture }: { capture: PreviewCaptureController }) {
  const { t } = useTranslation();
  const limited = capture.limitReached && capture.annotation.tool === "region";
  let error: string | null = null;
  if (capture.recaptureFailed) error = t("task:previewRegionRecaptureFailed");
  else if (capture.captureError) error = t("task:previewScreenshotFailed");
  if (!limited && !error && !capture.isRasterizing) return null;
  return (
    <div className="pointer-events-none absolute top-12 left-1/2 z-20 flex -translate-x-1/2 flex-col items-center gap-1 text-xs [@media(pointer:coarse)]:top-2">
      {capture.isRasterizing && (
        <p role="status" className="rounded-full border bg-background/95 px-3 py-1 shadow-sm">
          {t("task:previewRasterizing")}
        </p>
      )}
      {limited && (
        <p
          role="status"
          className="rounded-full border bg-background/95 px-3 py-1 shadow-sm"
          data-testid="preview-screenshot-limit"
        >
          {t("task:previewScreenshotLimit", { max: MAX_PREVIEW_SCREENSHOTS })}
        </p>
      )}
      {error && (
        <p
          role="alert"
          className="rounded-full border border-destructive/40 bg-background/95 px-3 py-1 text-destructive shadow-sm"
        >
          {error}
        </p>
      )}
    </div>
  );
}

function ActiveAnnotation({
  capture,
  taskId,
  root,
  touch,
}: Props & { root: React.RefObject<HTMLDivElement | null>; touch: boolean }) {
  const { annotation } = capture;
  const bounds = useCallback(() => {
    const element = root.current;
    return element ? { width: element.clientWidth, height: element.clientHeight } : null;
  }, [root]);
  const drag = useRegionDrag({
    regions: capture.regions,
    bounds,
    onOpen: capture.openRegion,
    onRecapture: capture.recaptureRegion,
  });
  const pendingIndexes = useMemo(
    () => new Set(drag.regions.filter((region) => region.pending).map((region) => region.index)),
    [drag.regions],
  );
  const pendingCount = capture.items.length + capture.drafts.length;
  return (
    <>
      <div aria-hidden="true" className="absolute inset-0 ring-2 ring-primary ring-inset" />
      <RegionOverlay
        items={drag.regions.map((region) => ({
          index: region.index,
          rect: region.rect,
          title: region.title,
        }))}
        selectedIndex={null}
        draft={null}
        testId="preview-region"
        pendingIndexes={pendingIndexes}
        onHandlePointerDown={annotation.tool === "region" ? drag.onHandlePointerDown : undefined}
      />
      <PreviewAnnotationToolbar
        touch={touch}
        tool={annotation.tool}
        acknowledgedMode={capture.acknowledgedMode}
        pendingCount={pendingCount}
        regionDisabled={capture.limitReached}
        onToolChange={annotation.setTool}
        onExit={annotation.exit}
        onOpenPending={() => capture.setPendingPanelOpen(true)}
        renderCount={(button) => (
          <PreviewFeedbackCollectionSurface
            taskId={taskId}
            collection={capture}
            open={capture.pendingPanelOpen}
            onOpenChange={capture.setPendingPanelOpen}
            trigger={button}
            side={touch ? "top" : "bottom"}
            portal
            drafts={{
              entries: capture.drafts,
              onResume: capture.resumeDraft,
              onDiscard: capture.discardDraft,
            }}
          />
        )}
      />
      <AnnotationStatus capture={capture} />
    </>
  );
}

/**
 * Overlay aligned with the preview iframe. It passes input through to the page
 * except on the toolbar, region handles, and the selection comment button.
 */
export function PreviewAnnotationLayer({ capture, taskId }: Props) {
  const rootRef = useRef<HTMLDivElement>(null);
  const touch = useTouchDrawer();
  useLayerEscape(capture, rootRef);
  const active = capture.annotation.active;
  return (
    <div
      ref={rootRef}
      className="pointer-events-none absolute inset-0 z-10"
      data-testid="preview-annotation-layer"
      data-active={active ? "true" : "false"}
    >
      {active && (
        <ActiveAnnotation capture={capture} taskId={taskId} root={rootRef} touch={touch} />
      )}
      <PreviewSelectionComment capture={capture} touch={touch} host={rootRef} />
      <PreviewDraftSurface capture={capture} host={rootRef} />
      <PreviewSavedRegionEditor capture={capture} host={rootRef} />
    </div>
  );
}
