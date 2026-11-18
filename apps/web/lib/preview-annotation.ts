import type { PreviewCaptureMode } from "@/lib/preview-inspect-bridge";
import type { PreviewCaptureRect } from "@/lib/types/http";

export type PreviewAnnotationTool = "interact" | "text" | "element" | "region";

/** The backend's pending screenshot limit; unsaved drafts count toward it. */
export const MAX_PREVIEW_SCREENSHOTS = 10;

const TOOL_MODES: Record<PreviewAnnotationTool, PreviewCaptureMode | null> = {
  interact: null,
  text: "text",
  element: "element",
  region: "screenshot",
};

/** Bridge capture mode for a tool. A blocked Region tool lets input through. */
export function bridgeModeForTool(
  tool: PreviewAnnotationTool,
  regionBlocked: boolean,
): PreviewCaptureMode | null {
  if (tool === "region" && regionBlocked) return null;
  return TOOL_MODES[tool];
}

export type EscapeStep = "close-surface" | "interact" | "exit";

/** Escape closes a comment surface, then drops to Interact, then leaves the mode. */
export function nextEscapeStep(state: {
  surfaceOpen: boolean;
  tool: PreviewAnnotationTool;
}): EscapeStep {
  if (state.surfaceOpen) return "close-surface";
  if (state.tool !== "interact") return "interact";
  return "exit";
}

export function isScreenshotLimitReached(saved: number, unsaved: number) {
  return saved + unsaved >= MAX_PREVIEW_SCREENSHOTS;
}

export type ViewportRect = { x: number; y: number; width: number; height: number };

export type PreviewFrameMetrics = {
  scrollX: number;
  scrollY: number;
  innerWidth: number;
  innerHeight: number;
  containerScroll: (selector: string) => { x: number; y: number } | null;
};

/**
 * Capture rectangle for a moved or resized region, anchored at the page's
 * current document and container scroll so the bridge keeps tracking it.
 */
export function recaptureRect(
  previous: PreviewCaptureRect,
  rect: ViewportRect,
  frame: PreviewFrameMetrics,
): PreviewCaptureRect {
  const container = previous.scroll_container_selector
    ? frame.containerScroll(previous.scroll_container_selector)
    : null;
  return {
    ...previous,
    ...rect,
    document_x: rect.x + frame.scrollX,
    document_y: rect.y + frame.scrollY,
    scroll_x: frame.scrollX,
    scroll_y: frame.scrollY,
    viewport_width: frame.innerWidth,
    viewport_height: frame.innerHeight,
    ...(container ? { container_scroll_x: container.x, container_scroll_y: container.y } : {}),
  };
}

/** Live scroll metrics of a same-origin preview iframe. */
export function previewFrameMetrics(iframe: HTMLIFrameElement): PreviewFrameMetrics | null {
  const frameWindow = iframe.contentWindow;
  const frameDocument = iframe.contentDocument;
  if (!frameWindow || !frameDocument) return null;
  return {
    scrollX: frameWindow.scrollX,
    scrollY: frameWindow.scrollY,
    innerWidth: frameWindow.innerWidth,
    innerHeight: frameWindow.innerHeight,
    containerScroll: (selector) => {
      try {
        const element = frameDocument.querySelector(selector);
        return element ? { x: element.scrollLeft, y: element.scrollTop } : null;
      } catch {
        return null;
      }
    },
  };
}
