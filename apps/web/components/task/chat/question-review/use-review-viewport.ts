import { useCallback, useEffect, useRef, useState, type RefObject } from "react";
import {
  actualSizeView,
  clampView,
  fitView,
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
export function useReviewViewport(
  stageRef: RefObject<HTMLElement | null>,
  image: Size,
  areaRef?: RefObject<HTMLElement | null>,
  panEnabled = true,
) {
  const stage = useElementSize(stageRef);
  const [manual, setManual] = useState<ReviewView | null>(null);
  const view = manual ? clampView(manual, stage, image) : fitView(stage, image);
  const latest = useRef({ view, stage, image });
  latest.current = { view, stage, image };
  const pointers = useRef(new Map<number, Point>());

  const apply = useCallback((next: (current: ReviewView) => ReviewView) => {
    const { view: current, stage: s, image: i } = latest.current;
    setManual(clampView(next(current), s, i));
  }, []);
  const center = useCallback(
    (): Point => ({ x: latest.current.stage.width / 2, y: latest.current.stage.height / 2 }),
    [],
  );
  const zoomStep = useCallback(
    (direction: 1 | -1) => apply((v) => zoomAt(v, nextZoomStep(v.scale, direction), center())),
    [apply, center],
  );
  const fit = useCallback(() => setManual(null), []);
  const actual = useCallback(() => {
    const { view: v, stage: s, image: i } = latest.current;
    setManual(actualSizeView(s, i, v));
  }, []);
  const pan = useCallback((dx: number, dy: number) => apply((v) => panBy(v, dx, dy)), [apply]);

  useWheelZoom(areaRef ?? stageRef, apply);

  const handleKey = useCallback(
    (event: { key: string; shiftKey: boolean }): boolean => {
      const { view: v, stage: s } = latest.current;
      const stepX = event.shiftKey ? v.scale : s.width * KEY_PAN_FRACTION;
      const stepY = event.shiftKey ? v.scale : s.height * KEY_PAN_FRACTION;
      const actions: Record<string, () => void> = {
        "+": () => zoomStep(1),
        "=": () => zoomStep(1),
        "-": () => zoomStep(-1),
        "0": fit,
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
    [actual, fit, pan, zoomStep],
  );

  const panEnabledRef = useRef(panEnabled);
  panEnabledRef.current = panEnabled;
  const pointerHandlers = usePointerPanZoom(pointers, apply, panEnabledRef);

  return { view, stage, zoomStep, fit, actual, handleKey, pointerHandlers };
}

/** Focal points are measured in the pane under the pointer, so Side by side panes agree. */
function paneOf(target: EventTarget | null, fallback: HTMLElement): HTMLElement {
  const pane = target instanceof Element ? target.closest<HTMLElement>("[data-review-pane]") : null;
  return pane ?? fallback;
}

function useWheelZoom(
  areaRef: RefObject<HTMLElement | null>,
  apply: (next: (current: ReviewView) => ReviewView) => void,
) {
  useEffect(() => {
    const element = areaRef.current;
    if (!element) return;
    const onWheel = (event: WheelEvent) => {
      event.preventDefault();
      if (event.ctrlKey || event.metaKey) {
        const focal = pointIn(paneOf(event.target, element), event);
        apply((v) => zoomAt(v, v.scale * Math.exp(-event.deltaY * WHEEL_ZOOM_RATE), focal));
        return;
      }
      apply((v) => panBy(v, -event.deltaX, -event.deltaY));
    };
    element.addEventListener("wheel", onWheel, { passive: false });
    return () => element.removeEventListener("wheel", onWheel);
  }, [apply, areaRef]);
}

/** One pointer pans (unless drawing has taken it); two pointers pinch-zoom. */
function usePointerPanZoom(
  pointers: RefObject<Map<number, Point>>,
  apply: (next: (current: ReviewView) => ReviewView) => void,
  panEnabled: RefObject<boolean>,
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
      apply((v) => zoomAt(v, v.scale * ratio, focal));
    },
    [apply, panEnabled, pointers],
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
