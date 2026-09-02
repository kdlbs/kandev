"use client";

import { useEffect, useRef, useState } from "react";

/**
 * Tracks whether a single-line title element is visually truncated at its
 * rendered width, re-measuring on resize. Attach the returned `ref` to the
 * element whose truncation should be measured.
 *
 * The element is clamped with `line-clamp-1` (`-webkit-box` + `-webkit-line-
 * clamp`), which wraps text normally within the box's width and clips
 * vertically, not horizontally: a wrapping multi-word title never grows
 * `scrollWidth` past `clientWidth`, so that comparison never reports
 * truncation for the common case. `scrollHeight` vs `clientHeight` measures
 * the axis line-clamp actually clips.
 */
export function useIsTitleTruncated<T extends HTMLElement>() {
  const ref = useRef<T>(null);
  const [isTruncated, setIsTruncated] = useState(false);

  useEffect(() => {
    const element = ref.current;
    if (!element) return;

    const update = () => setIsTruncated(element.scrollHeight > element.clientHeight);
    update();

    const observer = new ResizeObserver(update);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  return { ref, isTruncated };
}
