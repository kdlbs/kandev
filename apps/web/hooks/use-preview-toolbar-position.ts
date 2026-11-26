"use client";

import { useCallback, useEffect, useLayoutEffect, useState, type RefObject } from "react";
import { startMouseDrag } from "./use-drag-offset";

export const PREVIEW_TOOLBAR_POSITION_KEY = "kandev:preview-annotation-toolbar-position";

/** Place within the free space: 0 is the left or top edge, 1 the right or bottom. */
export type ToolbarPosition = { x: number; y: number };
type Size = { width: number; height: number };

const DEFAULT_POSITION: ToolbarPosition = { x: 0.5, y: 0 };
const MARGIN = 8;
const KEY_STEP = 16;
const ARROWS: Record<string, [number, number]> = {
  ArrowLeft: [-KEY_STEP, 0],
  ArrowRight: [KEY_STEP, 0],
  ArrowUp: [0, -KEY_STEP],
  ArrowDown: [0, KEY_STEP],
};

const clamp01 = (value: number) => Math.min(1, Math.max(0, value));
const freeSpace = (frame: number, toolbar: number) => Math.max(0, frame - toolbar - 2 * MARGIN);

export function toolbarPixels(position: ToolbarPosition, frame: Size, toolbar: Size) {
  return {
    left: MARGIN + position.x * freeSpace(frame.width, toolbar.width),
    top: MARGIN + position.y * freeSpace(frame.height, toolbar.height),
  };
}

export function toolbarFractions(
  pixels: { left: number; top: number },
  frame: Size,
  toolbar: Size,
): ToolbarPosition {
  const fraction = (offset: number, space: number) =>
    space > 0 ? clamp01((offset - MARGIN) / space) : 0;
  return {
    x: fraction(pixels.left, freeSpace(frame.width, toolbar.width)),
    y: fraction(pixels.top, freeSpace(frame.height, toolbar.height)),
  };
}

function readPosition(): ToolbarPosition {
  try {
    const stored = JSON.parse(window.localStorage.getItem(PREVIEW_TOOLBAR_POSITION_KEY) ?? "null");
    if (Number.isFinite(stored?.x) && Number.isFinite(stored?.y)) {
      return { x: clamp01(stored.x), y: clamp01(stored.y) };
    }
  } catch {
    // Unreadable storage keeps the default position.
  }
  return DEFAULT_POSITION;
}

function writePosition(position: ToolbarPosition) {
  try {
    window.localStorage.setItem(PREVIEW_TOOLBAR_POSITION_KEY, JSON.stringify(position));
  } catch {
    // The position still applies for this session.
  }
}

function measure(element: HTMLElement | null): Size {
  return { width: element?.offsetWidth ?? 0, height: element?.offsetHeight ?? 0 };
}

/** Re-renders when the frame or toolbar resizes, so the position stays inside. */
function useResizeTick(
  frame: RefObject<HTMLElement | null>,
  toolbar: RefObject<HTMLElement | null>,
) {
  const [, setTick] = useState(0);
  // Measure once the elements exist, before the first paint, so the toolbar never jumps.
  useLayoutEffect(() => setTick((tick) => tick + 1), []);
  useEffect(() => {
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(() => setTick((tick) => tick + 1));
    if (frame.current) observer.observe(frame.current);
    if (toolbar.current) observer.observe(toolbar.current);
    return () => observer.disconnect();
  }, [frame, toolbar]);
}

/**
 * Where the annotation toolbar sits over the preview frame. The user drags it
 * by its handle, and a double-click on the handle puts it back at the top.
 * The device remembers the place as fractions of the free space, so it stays
 * inside the frame at any size.
 */
export function usePreviewToolbarPosition(
  frame: RefObject<HTMLElement | null>,
  toolbar: RefObject<HTMLElement | null>,
) {
  const [position, setPosition] = useState(readPosition);
  useResizeTick(frame, toolbar);

  const place = useCallback((next: ToolbarPosition, persist: boolean) => {
    setPosition(next);
    if (persist) writePosition(next);
  }, []);
  const fromPixels = useCallback(
    (pixels: { left: number; top: number }) =>
      toolbarFractions(pixels, measure(frame.current), measure(toolbar.current)),
    [frame, toolbar],
  );

  const pixels = toolbarPixels(position, measure(frame.current), measure(toolbar.current));
  const onMouseDown = useCallback(
    (event: React.MouseEvent) => {
      if (event.detail >= 2) {
        place(DEFAULT_POSITION, true);
        return;
      }
      const origin = pixels;
      const at = (dx: number, dy: number) =>
        fromPixels({ left: origin.left + dx, top: origin.top + dy });
      startMouseDrag(event, {
        onMove: (dx, dy) => place(at(dx, dy), false),
        onEnd: (dx, dy) => place(at(dx, dy), true),
      });
    },
    [fromPixels, pixels, place],
  );
  const onKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      const step = ARROWS[event.key];
      if (!step) return;
      event.preventDefault();
      place(fromPixels({ left: pixels.left + step[0], top: pixels.top + step[1] }), true);
    },
    [fromPixels, pixels, place],
  );
  return { style: pixels, handleProps: { onMouseDown, onKeyDown } };
}
