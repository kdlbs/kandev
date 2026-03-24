"use client";

import { useEffect, useRef, useState, type RefObject } from "react";

/**
 * Measures the total width of a container's direct children and compares it
 * to the container's available width. Returns `true` when children overflow.
 *
 * Uses a cached "full width" (measured when expanded) to prevent toggle loops
 * at the collapse/expand boundary. Observes both container resizes and DOM
 * mutations (child additions/removals) to stay in sync when toolbar items
 * appear or disappear.
 */
export function useToolbarCollapsed(containerRef: RefObject<HTMLDivElement | null>): boolean {
  const [isCollapsed, setIsCollapsed] = useState(false);
  const collapsedRef = useRef(false);
  const fullWidthRef = useRef(0);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;

    const childrenWidth = () => {
      let total = 0;
      for (const child of el.children) {
        total += (child as HTMLElement).offsetWidth;
      }
      // Account for gaps between children
      const style = getComputedStyle(el);
      const gap = parseFloat(style.columnGap || "0");
      const childCount = el.children.length;
      if (childCount > 1) total += gap * (childCount - 1);
      return total;
    };

    const check = () => {
      if (!collapsedRef.current) {
        const contentWidth = childrenWidth();
        fullWidthRef.current = contentWidth;
        if (contentWidth > el.clientWidth + 1) {
          collapsedRef.current = true;
          setIsCollapsed(true);
        }
      } else {
        if (el.clientWidth >= fullWidthRef.current) {
          collapsedRef.current = false;
          setIsCollapsed(false);
        }
      }
    };

    const resizeObserver = new ResizeObserver(check);
    resizeObserver.observe(el);

    // Also observe DOM mutations so we remeasure when children change
    // (e.g. isAgentBusy flips and items appear/disappear).
    const mutationObserver = new MutationObserver(check);
    mutationObserver.observe(el, { childList: true, subtree: true });

    return () => {
      resizeObserver.disconnect();
      mutationObserver.disconnect();
    };
  }, [containerRef]);

  return isCollapsed;
}
