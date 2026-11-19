import type { PreviewCaptureMode, PreviewRegionPlacement } from "@/lib/preview-inspect-bridge";
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

export type CloseCommentStep = "discard-draft" | "keep-draft" | "close-editor" | "none";

/**
 * Closing a comment by Close, Escape, or an outside click: a new capture with
 * no text is removed, one with text stays as an unsaved draft, and a saved
 * item's editor closes without saving.
 */
export function closeCommentStep(state: {
  draftComment: string | null;
  editing: boolean;
}): CloseCommentStep {
  if (state.draftComment !== null)
    return state.draftComment.trim() ? "keep-draft" : "discard-draft";
  return state.editing ? "close-editor" : "none";
}

/** One number per capture, counted per route in list order; shared by markers and the list. */
export function captureNumbers(entries: readonly { id: string; route: string }[]) {
  const counts = new Map<string, number>();
  const numbers = new Map<string, number>();
  for (const entry of entries) {
    const next = (counts.get(entry.route) ?? 0) + 1;
    counts.set(entry.route, next);
    numbers.set(entry.id, next);
  }
  return numbers;
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

/** The previous rectangle without its content anchor, which a move invalidates. */
function withoutAnchor(rect: PreviewCaptureRect): PreviewCaptureRect {
  const {
    anchor_selector: _selector,
    anchor_offset_x: _offsetX,
    anchor_offset_y: _offsetY,
    anchor_width: _width,
    anchor_height: _height,
    ...rest
  } = rect;
  return rest;
}

/** The previous rectangle without its scroll container, once the page reports the new one. */
function withoutContainer(rect: PreviewCaptureRect): PreviewCaptureRect {
  const {
    scroll_container_selector: _selector,
    container_scroll_x: _scrollX,
    container_scroll_y: _scrollY,
    ...rest
  } = rect;
  return rest;
}

/**
 * Capture rectangle for a moved or resized region, anchored at the page's
 * current document scroll so the bridge keeps tracking it. When the page
 * reports the new position, the region takes that element and scroll
 * container; otherwise it keeps its previous container at its current scroll.
 */
export function recaptureRect(
  previous: PreviewCaptureRect,
  rect: ViewportRect,
  frame: PreviewFrameMetrics,
  placement: PreviewRegionPlacement | null = null,
): PreviewCaptureRect {
  if (placement) {
    return {
      ...withoutContainer(withoutAnchor(previous)),
      ...placement.anchor,
      ...placement.container,
      ...documentPlacement(rect, frame),
    };
  }
  const container = previous.scroll_container_selector
    ? frame.containerScroll(previous.scroll_container_selector)
    : null;
  return {
    ...withoutAnchor(previous),
    ...documentPlacement(rect, frame),
    ...(container ? { container_scroll_x: container.x, container_scroll_y: container.y } : {}),
  };
}

function documentPlacement(rect: ViewportRect, frame: PreviewFrameMetrics) {
  return {
    ...rect,
    document_x: rect.x + frame.scrollX,
    document_y: rect.y + frame.scrollY,
    scroll_x: frame.scrollX,
    scroll_y: frame.scrollY,
    viewport_width: frame.innerWidth,
    viewport_height: frame.innerHeight,
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
