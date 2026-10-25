"use client";

import { useEffect, useRef, useState } from "react";

/**
 * Tracks whether a row's inline status strip must join the step run's scroll
 * region, re-measuring on resize. Attach `outerRef` to the combined region
 * available to the strip and the step run, and `stripRef` to the strip's own
 * always-natural-width wrapper. The strip's natural width never shrinks, so
 * comparing it against the outer region's rendered width is stage-independent.
 */
export function usePipelineOverflowStage<T extends HTMLElement, S extends HTMLElement>() {
  const outerRef = useRef<T>(null);
  const stripRef = useRef<S>(null);
  const [atTerminus, setAtTerminus] = useState(false);

  useEffect(() => {
    const outer = outerRef.current;
    const strip = stripRef.current;
    if (!outer || !strip) return;

    const update = () => setAtTerminus(strip.scrollWidth > outer.clientWidth);
    update();

    const observer = new ResizeObserver(update);
    observer.observe(outer);
    observer.observe(strip);
    return () => observer.disconnect();
  }, []);

  return { outerRef, stripRef, atTerminus };
}
