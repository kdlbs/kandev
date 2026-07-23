"use client";

import { useCallback, useEffect, useRef, useState } from "react";

const SIDEBAR_WIDTH_KEY = "review-dialog-sidebar-width";
const DEFAULT_SIDEBAR_WIDTH = 220;
const MIN_SIDEBAR_WIDTH = 160;
const MAX_SIDEBAR_WIDTH = 600;
// Reserve at least this much horizontal space for the diff pane so a
// previously-saved 600px sidebar can't push the diff list to ~0 on a small
// desktop viewport (e.g. the dialog is 80vw → 512px at the sm breakpoint).
const MIN_DIFF_PANE_WIDTH = 320;

export const REVIEW_SIDEBAR_LIMITS = {
  storageKey: SIDEBAR_WIDTH_KEY,
  defaultWidth: DEFAULT_SIDEBAR_WIDTH,
  minWidth: MIN_SIDEBAR_WIDTH,
  maxWidth: MAX_SIDEBAR_WIDTH,
  minDiffPaneWidth: MIN_DIFF_PANE_WIDTH,
} as const;

export function clampSidebarWidth(w: number, containerWidth?: number): number {
  const effectiveMax = containerWidth
    ? Math.max(MIN_SIDEBAR_WIDTH, Math.min(MAX_SIDEBAR_WIDTH, containerWidth - MIN_DIFF_PANE_WIDTH))
    : MAX_SIDEBAR_WIDTH;
  return Math.max(MIN_SIDEBAR_WIDTH, Math.min(effectiveMax, w));
}

function readStoredWidth(): number {
  if (typeof window === "undefined") return DEFAULT_SIDEBAR_WIDTH;
  try {
    const stored = window.sessionStorage.getItem(SIDEBAR_WIDTH_KEY);
    const parsed = stored ? Number(stored) : NaN;
    return Number.isFinite(parsed) ? clampSidebarWidth(parsed) : DEFAULT_SIDEBAR_WIDTH;
  } catch {
    return DEFAULT_SIDEBAR_WIDTH;
  }
}

function persistWidth(width: number) {
  try {
    window.sessionStorage.setItem(SIDEBAR_WIDTH_KEY, String(width));
  } catch {
    // sessionStorage may be unavailable (private mode, quota); ignore.
  }
}

function restoreBodyStyles() {
  document.body.style.cursor = "";
  document.body.style.userSelect = "";
}

export function useReviewSidebarResize(
  containerRef?: React.RefObject<HTMLElement | null>,
  open = true,
  sourceKey?: string,
) {
  const [width, setWidth] = useState<number>(readStoredWidth);
  const isDragging = useRef(false);
  const startX = useRef(0);
  const startWidth = useRef(0);

  // Re-clamp when the container becomes available or resizes so a stored
  // width carried over from a wider viewport gets pulled down on a narrower
  // one (rather than overflowing the dialog). `open` is in the dep array
  // because the container element only exists once Radix mounts the
  // DialogContent portal — on the closed → open transition, `containerRef`
  // (a stable ref *object*) doesn't change identity, so the effect needs
  // `open` to re-fire and pick up the now-populated `current`. `sourceKey`
  // likewise reattaches the observer when React replaces a keyed container.
  useEffect(() => {
    if (!open) return;
    const el = containerRef?.current;
    if (!el) return;
    const reclamp = () => {
      const cw = el.getBoundingClientRect().width;
      setWidth((w) => clampSidebarWidth(w, cw));
    };
    reclamp();
    if (typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(reclamp);
    ro.observe(el);
    return () => ro.disconnect();
  }, [containerRef, open, sourceKey]);

  const handleMouseDown = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      isDragging.current = true;
      startX.current = e.clientX;
      startWidth.current = width;
      document.body.style.cursor = "col-resize";
      document.body.style.userSelect = "none";
    },
    [width],
  );

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      if (!isDragging.current) return;
      const cw = containerRef?.current?.getBoundingClientRect().width;
      const delta = e.clientX - startX.current;
      setWidth(clampSidebarWidth(startWidth.current + delta, cw));
    };
    const handleMouseUp = () => {
      if (!isDragging.current) return;
      isDragging.current = false;
      restoreBodyStyles();
      setWidth((current) => {
        persistWidth(current);
        return current;
      });
    };
    document.addEventListener("mousemove", handleMouseMove);
    document.addEventListener("mouseup", handleMouseUp);
    return () => {
      document.removeEventListener("mousemove", handleMouseMove);
      document.removeEventListener("mouseup", handleMouseUp);
      if (isDragging.current) {
        isDragging.current = false;
        restoreBodyStyles();
      }
    };
  }, [containerRef]);

  return {
    width,
    resizeHandleProps: { onMouseDown: handleMouseDown },
  };
}
