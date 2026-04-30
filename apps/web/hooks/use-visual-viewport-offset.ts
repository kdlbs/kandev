import { useEffect, useState } from "react";

export type VisualViewportOffset = {
  /** Pixels between the bottom of `visualViewport` and the bottom of the layout viewport. */
  bottomOffset: number;
  /** True when the visual viewport is noticeably shorter than the layout viewport (virtual keyboard open). */
  keyboardOpen: boolean;
  /** Y-coordinate of the visual viewport's bottom edge in the layout viewport — `vv.offsetTop + vv.height`. Use with `position: fixed; top:` to pin an element above the keyboard reliably on iOS even during scrolls. */
  viewportBottom: number;
};

const KEYBOARD_THRESHOLD_PX = 80;

function readOffset(): VisualViewportOffset {
  if (typeof window === "undefined" || !window.visualViewport) {
    return { bottomOffset: 0, keyboardOpen: false, viewportBottom: 0 };
  }
  const vv = window.visualViewport;
  const bottomOffset = Math.max(0, window.innerHeight - vv.height - vv.offsetTop);
  return {
    bottomOffset,
    keyboardOpen: bottomOffset > KEYBOARD_THRESHOLD_PX,
    viewportBottom: vv.offsetTop + vv.height,
  };
}

/**
 * Tracks the visual viewport's bottom offset relative to the layout viewport so
 * floating mobile UI (e.g., terminal key-bar) can dock above the on-screen
 * keyboard. Returns zeros on the server and on browsers without
 * `window.visualViewport`.
 */
export function useVisualViewportOffset(): VisualViewportOffset {
  const [offset, setOffset] = useState<VisualViewportOffset>(() => ({
    bottomOffset: 0,
    keyboardOpen: false,
    viewportBottom: 0,
  }));

  useEffect(() => {
    if (typeof window === "undefined" || !window.visualViewport) return;
    const vv = window.visualViewport;
    const update = () => setOffset(readOffset());
    update();
    vv.addEventListener("resize", update);
    vv.addEventListener("scroll", update);
    window.addEventListener("resize", update);
    return () => {
      vv.removeEventListener("resize", update);
      vv.removeEventListener("scroll", update);
      window.removeEventListener("resize", update);
    };
  }, []);

  return offset;
}
