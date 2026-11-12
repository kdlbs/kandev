import { useCallback, useEffect, useRef, useState, type RefObject } from "react";
import {
  actualSizeView,
  clampPinnedView,
  clampView,
  fitView,
  minScaleFor,
  pinnedViewForFraction,
  pinnedWipeFraction,
  rectView,
  swipeDirection,
  nextZoomStep,
  panBy,
  zoomAt,
  type Point,
  type ReviewView,
  type Size,
} from "@/lib/question-review/viewport";

const KEY_PAN_FRACTION = 0.1;
const WHEEL_ZOOM_RATE = 0.002;
const EMPTY: Size = { width: 0, height: 0 };

function useElementSize(ref: RefObject<HTMLElement | null>): Size {
  const [size, setSize] = useState<Size>(EMPTY);
  useEffect(() => {
    const element = ref.current;
    if (!element) return;
    const measure = () => setSize({ width: element.clientWidth, height: element.clientHeight });
    measure();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, [ref]);
  return size;
}

export function pointIn(element: HTMLElement, event: { clientX: number; clientY: number }): Point {
  const rect = element.getBoundingClientRect();
  return { x: event.clientX - rect.left, y: event.clientY - rect.top };
}

function distance(a: Point, b: Point): number {
  return Math.hypot(a.x - b.x, a.y - b.y);
}

/**
 * Owns zoom and pan for the review stage. The view is measured in image pixels,
 * so every layer that shares it stays aligned across mode switches.
 */
export type ReviewViewportOptions = {
  /** Wheel events anywhere in this element zoom the view (Side by side panes). */
  areaRef?: RefObject<HTMLElement | null>;
  /** False while drawing owns one-finger drags; pinch still zooms. */
  panEnabled?: boolean;
  /** Pinned-wipe pan range: either image edge may reach the center line. */
  pinned?: boolean;
  /** Called for a horizontal swipe that starts while the view is at Fit. */
  onSwipe?: (direction: 1 | -1) => void;
};

function sameView(a: ReviewView, b: ReviewView): boolean {
  return (
    Math.abs(a.scale - b.scale) < 1e-6 && Math.abs(a.x - b.x) < 1e-3 && Math.abs(a.y - b.y) < 1e-3
  );
}

// eslint-disable-next-line max-lines-per-function -- one owner for every zoom and pan input keeps the layers aligned.
export function useReviewViewport(
  stageRef: RefObject<HTMLElement | null>,
  image: Size,
  options: ReviewViewportOptions = {},
) {
  const { areaRef, panEnabled = true, pinned = false } = options;
  const stage = useElementSize(stageRef);
  const [manual, setManual] = useState<ReviewView | null>(null);
  const fit = fitView(stage, image);
  const minScale = minScaleFor(fit.scale);
  const clamp = pinned ? clampPinnedView : clampView;
  const view = manual ? clamp(manual, stage, image) : fit;
  const latest = useRef({ view, stage, image, minScale, fit, clamp, atFit: manual === null });
  latest.current = { view, stage, image, minScale, fit, clamp, atFit: manual === null };
  const pointers = useRef(new Map<number, Point>());

  const apply = useCallback((next: (current: ReviewView) => ReviewView) => {
    const { view: current, stage: s, image: i, fit: f, clamp: c, atFit } = latest.current;
    const result = c(next(current), s, i);
    // A drag that cannot move a fitted image keeps the view fitted.
    setManual(atFit && sameView(result, f) ? null : result);
  }, []);
  const center = useCallback(
    (): Point => ({ x: latest.current.stage.width / 2, y: latest.current.stage.height / 2 }),
    [],
  );
  const zoomStep = useCallback(
    (direction: 1 | -1) => {
      const min = latest.current.minScale;
      apply((v) => zoomAt(v, nextZoomStep(v.scale, direction, min), center(), min));
    },
    [apply, center],
  );
  const fitToStage = useCallback(() => setManual(null), []);
  const actual = useCallback(() => {
    const { view: v, stage: s, image: i } = latest.current;
    setManual(actualSizeView(s, i, v));
  }, []);
  const pan = useCallback((dx: number, dy: number) => apply((v) => panBy(v, dx, dy)), [apply]);

  useWheelZoom(areaRef ?? stageRef, apply, latest);
  const zoomToRect = useCallback(
    (rect: { x: number; y: number; width: number; height: number }) => {
      const { stage: s, image: i, minScale: min, clamp: c } = latest.current;
      setManual(c(rectView(s, rect, min), s, i));
    },
    [],
  );
  const zoomTo = useCallback(
    (scale: number) => apply((v) => zoomAt(v, scale, center(), latest.current.minScale)),
    [apply, center],
  );
  const setPinnedFraction = useCallback((fraction: number) => {
    const { view: v, stage: s, image: i } = latest.current;
    setManual(clampPinnedView(pinnedViewForFraction(v, s, i, fraction), s, i));
  }, []);

  const handleKey = useCallback(
    (event: { key: string; shiftKey: boolean }): boolean => {
      const { view: v, stage: s } = latest.current;
      const stepX = event.shiftKey ? v.scale : s.width * KEY_PAN_FRACTION;
      const stepY = event.shiftKey ? v.scale : s.height * KEY_PAN_FRACTION;
      const actions: Record<string, () => void> = {
        "+": () => zoomStep(1),
        "=": () => zoomStep(1),
        "-": () => zoomStep(-1),
        "0": fitToStage,
        "1": actual,
        ArrowLeft: () => pan(stepX, 0),
        ArrowRight: () => pan(-stepX, 0),
        ArrowUp: () => pan(0, stepY),
        ArrowDown: () => pan(0, -stepY),
      };
      const action = actions[event.key];
      action?.();
      return Boolean(action);
    },
    [actual, fitToStage, pan, zoomStep],
  );

  const panEnabledRef = useRef(panEnabled);
  panEnabledRef.current = panEnabled;
  const minScaleRef = useRef(minScale);
  minScaleRef.current = minScale;
  const swipeRef = useRef(options.onSwipe);
  swipeRef.current = options.onSwipe;
  const pointerHandlers = useSwipe(
    usePointerPanZoom(pointers, apply, panEnabledRef, minScaleRef),
    pointers,
    swipeRef,
    latest,
  );

  return {
    view,
    stage,
    zoomStep,
    fit: fitToStage,
    actual,
    handleKey,
    pointerHandlers,
    zoomToRect,
    zoomTo,
    atFit: manual === null,
    pinnedFraction: pinnedWipeFraction(view, stage, image),
    setPinnedFraction,
  };
}

/** Focal points are measured in the pane under the pointer, so Side by side panes agree. */
function paneOf(target: EventTarget | null, fallback: HTMLElement): HTMLElement {
  const pane = target instanceof Element ? target.closest<HTMLElement>("[data-review-pane]") : null;
  return pane ?? fallback;
}

function useWheelZoom(
  areaRef: RefObject<HTMLElement | null>,
  apply: (next: (current: ReviewView) => ReviewView) => void,
  latest: RefObject<{ minScale: number }>,
) {
  useEffect(() => {
    const element = areaRef.current;
    if (!element) return;
    const onWheel = (event: WheelEvent) => {
      event.preventDefault();
      if (event.ctrlKey || event.metaKey) {
        const focal = pointIn(paneOf(event.target, element), event);
        const factor = Math.exp(-event.deltaY * WHEEL_ZOOM_RATE);
        apply((v) => zoomAt(v, v.scale * factor, focal, latest.current.minScale));
        return;
      }
      apply((v) => panBy(v, -event.deltaX, -event.deltaY));
    };
    element.addEventListener("wheel", onWheel, { passive: false });
    return () => element.removeEventListener("wheel", onWheel);
  }, [apply, areaRef, latest]);
}

/** One pointer pans (unless drawing has taken it); two pointers pinch-zoom. */
function usePointerPanZoom(
  pointers: RefObject<Map<number, Point>>,
  apply: (next: (current: ReviewView) => ReviewView) => void,
  panEnabled: RefObject<boolean>,
  minScale: RefObject<number>,
) {
  const onPointerDown = useCallback(
    (event: React.PointerEvent<HTMLElement>) => {
      const element = event.currentTarget;
      if (event.button > 0) return;
      element.setPointerCapture?.(event.pointerId);
      pointers.current.set(event.pointerId, pointIn(element, event));
    },
    [pointers],
  );
  const onPointerMove = useCallback(
    (event: React.PointerEvent<HTMLElement>) => {
      const element = event.currentTarget;
      const previous = pointers.current.get(event.pointerId);
      if (!previous) return;
      const next = pointIn(element, event);
      const others = [...pointers.current.entries()].filter(([id]) => id !== event.pointerId);
      pointers.current.set(event.pointerId, next);
      if (others.length === 0) {
        if (panEnabled.current) apply((v) => panBy(v, next.x - previous.x, next.y - previous.y));
        return;
      }
      const anchor = others[0][1];
      const before = distance(previous, anchor);
      if (before <= 0) return;
      const ratio = distance(next, anchor) / before;
      const focal = { x: (next.x + anchor.x) / 2, y: (next.y + anchor.y) / 2 };
      apply((v) => zoomAt(v, v.scale * ratio, focal, minScale.current));
    },
    [apply, minScale, panEnabled, pointers],
  );
  const onPointerEnd = useCallback(
    (event: React.PointerEvent<HTMLElement>) => {
      pointers.current.delete(event.pointerId);
    },
    [pointers],
  );
  return {
    onPointerDown,
    onPointerMove,
    onPointerUp: onPointerEnd,
    onPointerCancel: onPointerEnd,
  };
}

type PointerHandlers = ReturnType<typeof usePointerPanZoom>;

/** Adds swipe detection for a single pointer that starts while the view is at Fit. */
function useSwipe(
  handlers: PointerHandlers,
  pointers: RefObject<Map<number, Point>>,
  onSwipe: RefObject<((direction: 1 | -1) => void) | undefined>,
  latest: RefObject<{ atFit: boolean }>,
): PointerHandlers {
  const start = useRef<{ id: number; x: number; y: number } | null>(null);
  return {
    ...handlers,
    onPointerDown: (event) => {
      handlers.onPointerDown(event);
      const single = pointers.current.size <= 1;
      start.current =
        single && latest.current.atFit && onSwipe.current
          ? { id: event.pointerId, x: event.clientX, y: event.clientY }
          : null;
    },
    onPointerUp: (event) => {
      handlers.onPointerUp(event);
      const origin = start.current;
      start.current = null;
      if (!origin || origin.id !== event.pointerId) return;
      const direction = swipeDirection(event.clientX - origin.x, event.clientY - origin.y);
      if (direction !== 0) onSwipe.current?.(direction);
    },
    onPointerCancel: (event) => {
      handlers.onPointerCancel(event);
      start.current = null;
    },
  };
}
