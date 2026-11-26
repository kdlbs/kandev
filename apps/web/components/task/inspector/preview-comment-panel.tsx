"use client";

import { useEffect, useRef, useState, type ReactNode, type RefObject } from "react";
import { IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import type { PreviewCaptureController } from "@/hooks/use-preview-capture";
import { cn } from "@/lib/utils";
import { PreviewAnnotationControls } from "./preview-annotation-controls";
import { PreviewAnnotationLayer, useLayerEscape } from "./preview-annotation-layer";
import { PreviewFeedbackCollection } from "./preview-feedback-collection";
import { previewCommentList } from "./preview-comment-list-model";

/**
 * Below this host width the panel overlays the page instead of narrowing it,
 * starting below the toolbar so every tool stays reachable.
 */
export const PREVIEW_PANEL_DOCK_MIN_WIDTH = 720;

/** Scrolls the entry whose comment is open into view. */
function useScrollActiveIntoView(activeId: string | null, body: RefObject<HTMLElement | null>) {
  useEffect(() => {
    if (!activeId) return;
    const entry = body.current?.querySelector('[data-active="true"]');
    entry?.scrollIntoView?.({ block: "nearest" });
  }, [activeId, body]);
}

/** Tracks whether the host is too narrow to dock the panel beside the page. */
export function useNarrowHost(host: RefObject<HTMLElement | null>) {
  const [narrow, setNarrow] = useState(false);
  useEffect(() => {
    const element = host.current;
    if (!element) return;
    const measure = () => setNarrow(element.clientWidth < PREVIEW_PANEL_DOCK_MIN_WIDTH);
    measure();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, [host]);
  return narrow;
}

/** The annotation mode's comment list, docked beside the preview on fine pointers. */
export function PreviewCommentPanel({
  capture,
  overlay,
}: {
  capture: PreviewCaptureController;
  overlay: boolean;
}) {
  const { t } = useTranslation();
  const bodyRef = useRef<HTMLDivElement>(null);
  const list = previewCommentList(capture);
  useScrollActiveIntoView(list.activeId, bodyRef);
  return (
    <aside
      aria-label={t("task:previewFeedbackTitle")}
      data-testid="preview-comment-panel"
      data-overlay={overlay ? "true" : "false"}
      className={cn(
        "flex h-full w-80 shrink-0 flex-col border-l bg-background",
        overlay && "absolute top-11 right-0 bottom-0 z-30 h-auto rounded-tl-md border-t shadow-lg",
      )}
    >
      <div className="flex items-center justify-between gap-2 border-b px-3 py-2">
        <h2 className="text-sm font-medium">{t("task:previewFeedbackTitle")}</h2>
        <Button
          type="button"
          variant="ghost"
          className="h-7 min-w-7 cursor-pointer"
          aria-label={t("task:previewHideComments")}
          data-testid="preview-comment-panel-hide"
          onClick={() => capture.commentPanel.setOpen(false)}
        >
          <IconX className="h-4 w-4" />
        </Button>
      </div>
      <div ref={bodyRef} className="min-h-0 flex-1 overflow-y-auto p-3">
        <PreviewFeedbackCollection
          collection={capture}
          touch={false}
          list={list}
          drafts={{
            entries: capture.drafts,
            onResume: capture.openCapture,
            onDiscard: capture.discardDraft,
          }}
        />
      </div>
    </aside>
  );
}

/**
 * The preview frame with its annotation layer and toolbar, and the comment
 * panel docked beside it (or over its right edge when the host is narrow).
 * The toolbar stacks above the overlay panel.
 */
export function PreviewAnnotationHost({
  capture,
  showLayer,
  taskId,
  children,
}: {
  capture: PreviewCaptureController;
  showLayer: boolean;
  taskId: string | null | undefined;
  children: ReactNode;
}) {
  const hostRef = useRef<HTMLDivElement>(null);
  const frameRef = useRef<HTMLDivElement>(null);
  const narrow = useNarrowHost(hostRef);
  useLayerEscape(capture, frameRef);
  const active = showLayer && capture.annotation.active;
  return (
    <div ref={hostRef} className="relative flex h-full w-full min-h-0">
      <div
        ref={frameRef}
        className="relative h-full min-w-0 flex-1"
        data-testid="preview-annotation-frame"
      >
        {children}
        {showLayer && <PreviewAnnotationLayer capture={capture} />}
        {active && <PreviewAnnotationControls capture={capture} taskId={taskId} frame={frameRef} />}
      </div>
      {active && capture.commentPanel.docked && (
        <PreviewCommentPanel capture={capture} overlay={narrow} />
      )}
    </div>
  );
}
