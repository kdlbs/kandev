import { useEffect, useState, type CSSProperties, type RefObject } from "react";
import {
  resolveVisualViewportPosition,
  useVisualViewportOffset,
} from "@/hooks/use-visual-viewport-offset";

export type DockContainerBounds = { left: number; width: number; bottom: number };

export type DockedBarPosition = { top: number; left?: number; width?: number } | { bottom: string };

/**
 * Places a bottom bar at the visible bottom edge of its container, or directly
 * above the software keyboard when the keyboard covers that edge. Without a
 * container, the bar follows the visual viewport.
 */
export function resolveDockedBarPosition({
  keyboardOpen,
  viewportBottom,
  barHeight,
  baseBottomOffset,
  container,
}: {
  keyboardOpen: boolean;
  viewportBottom: number;
  barHeight: number;
  baseBottomOffset?: string;
  container?: DockContainerBounds | null;
}): DockedBarPosition {
  if (container) {
    // Clamp to the visual viewport so a host taller than the screen keeps the bar visible.
    const visibleBottom =
      viewportBottom > 0 ? Math.min(container.bottom, viewportBottom) : container.bottom;
    return { top: visibleBottom - barHeight, left: container.left, width: container.width };
  }
  if (keyboardOpen) return { top: viewportBottom - barHeight };
  const fallback = resolveVisualViewportPosition({
    keyboardOpen,
    viewportBottom,
    barHeight,
    baseBottomOffset,
  });
  return { bottom: String(fallback.bottom) };
}

/**
 * Converts a docked position into inline styles. `origin` is the client-space
 * corner of a positioned ancestor when the bar is absolutely positioned inside
 * it instead of fixed to the viewport.
 */
export function dockedBarStyle(
  position: DockedBarPosition,
  origin?: { left: number; top: number },
): CSSProperties {
  if ("bottom" in position) return { bottom: position.bottom, left: 0, right: 0 };
  const top = position.top - (origin?.top ?? 0);
  if (position.left === undefined) return { top, bottom: "auto", left: 0, right: 0 };
  return {
    top,
    bottom: "auto",
    left: position.left - (origin?.left ?? 0),
    right: "auto",
    width: position.width,
  };
}

function useContainerBounds(containerRef?: RefObject<HTMLElement | null>) {
  const [bounds, setBounds] = useState<DockContainerBounds | null>(null);

  useEffect(() => {
    const container = containerRef?.current;
    if (!container) return;

    const update = () => {
      const { left, width, bottom } = container.getBoundingClientRect();
      setBounds((current) =>
        current?.left === left && current.width === width && current.bottom === bottom
          ? current
          : { left, width, bottom },
      );
    };
    update();

    const resizeObserver =
      typeof ResizeObserver === "undefined" ? null : new ResizeObserver(update);
    resizeObserver?.observe(container);
    window.addEventListener("resize", update);
    window.addEventListener("scroll", update, true);
    window.visualViewport?.addEventListener("resize", update);
    window.visualViewport?.addEventListener("scroll", update);
    return () => {
      resizeObserver?.disconnect();
      window.removeEventListener("resize", update);
      window.removeEventListener("scroll", update, true);
      window.visualViewport?.removeEventListener("resize", update);
      window.visualViewport?.removeEventListener("scroll", update);
    };
  }, [containerRef]);

  return bounds;
}

export function useDockedBarPosition({
  containerRef,
  barHeight,
  baseBottomOffset,
}: {
  containerRef?: RefObject<HTMLElement | null>;
  barHeight: number;
  baseBottomOffset?: string;
}): DockedBarPosition {
  const { keyboardOpen, viewportBottom } = useVisualViewportOffset();
  const container = useContainerBounds(containerRef);
  return resolveDockedBarPosition({
    keyboardOpen,
    viewportBottom,
    barHeight,
    baseBottomOffset,
    container,
  });
}
