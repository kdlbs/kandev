"use client";

import { useEffect, useRef } from "react";
import {
  isInspectorMessage,
  previewOrigin,
  type InspectorMessage,
  type PreviewCaptureDraft,
  type PreviewCaptureMode,
  type PreviewHighlightedLayout,
  type PreviewRegionLayout,
  type PreviewScreenshotRegion,
} from "@/lib/preview-inspect-bridge";
import { sanitizePreviewPageRoute } from "@/lib/preview-feedback-source";
import type { PreviewCaptureRect } from "@/lib/types/http";

export type PreviewInspectorHandlers = {
  iframeRef: React.RefObject<HTMLIFrameElement | null>;
  enabled: boolean;
  onPageChanged: (route: string, title: string, bridgeReloaded: boolean) => void;
  onCandidate: (label: string | null) => void;
  onModeAcknowledged: (mode: PreviewCaptureMode | null) => void;
  onCapture: (draft: PreviewCaptureDraft) => void;
  onRegionSelected: (region: PreviewScreenshotRegion) => void;
  onSelectionChanged: (rect: PreviewCaptureRect | null) => void;
  onRegionLayout: (
    regions: PreviewRegionLayout[],
    highlighted: PreviewHighlightedLayout | null,
  ) => void;
  onEscape: () => void;
  /** The rect is where the page reports the marker's target, when it knows. */
  onMarkerActivated: (id: string, rect?: PreviewHighlightedLayout["rect"]) => void;
  onPagePointerDown: () => void;
};

function withSanitizedRoute<T extends { page_route: string }>(payload: T): T {
  return { ...payload, page_route: sanitizePreviewPageRoute(payload.page_route) };
}

function dispatchCapture(data: InspectorMessage, handlers: PreviewInspectorHandlers) {
  if (!handlers.enabled) return;
  if (data.type === "capture-completed") handlers.onCapture(withSanitizedRoute(data.payload));
  if (data.type === "screenshot-region-selected") {
    handlers.onRegionSelected(withSanitizedRoute(data.payload));
  }
}

function dispatchInspectorMessage(data: unknown, handlers: PreviewInspectorHandlers) {
  if (!isInspectorMessage(data)) return;
  switch (data.type) {
    case "inspector-ready":
    case "route-changed":
      handlers.onPageChanged(
        sanitizePreviewPageRoute(data.payload.page_route),
        data.payload.page_title,
        data.type === "inspector-ready",
      );
      break;
    case "candidate-changed":
      handlers.onCandidate(data.payload.label);
      break;
    case "capture-mode-changed":
      handlers.onModeAcknowledged(data.payload.mode);
      break;
    case "capture-completed":
    case "screenshot-region-selected":
      dispatchCapture(data, handlers);
      break;
    case "selection-changed":
      handlers.onSelectionChanged(handlers.enabled ? data.payload.rect : null);
      break;
    case "region-layout":
      handlers.onRegionLayout(data.payload.regions, data.payload.highlighted ?? null);
      break;
    case "escape":
      handlers.onEscape();
      break;
    case "marker-activated":
      handlers.onMarkerActivated(data.payload.id, data.payload.rect);
      break;
    case "page-pointerdown":
      handlers.onPagePointerDown();
      break;
    default:
      break;
  }
}

/** Routes validated bridge messages from this preview's iframe to the given handlers. */
export function usePreviewInspectorEvents(handlers: PreviewInspectorHandlers) {
  const handlersRef = useRef(handlers);
  handlersRef.current = handlers;

  useEffect(() => {
    function handleMessage(event: MessageEvent) {
      const current = handlersRef.current;
      const iframe = current.iframeRef.current;
      if (!iframe || event.source !== iframe.contentWindow) return;
      const origin = previewOrigin(iframe);
      if (!origin || event.origin !== origin) return;
      dispatchInspectorMessage(event.data, current);
    }
    window.addEventListener("message", handleMessage);
    return () => window.removeEventListener("message", handleMessage);
  }, []);
}
