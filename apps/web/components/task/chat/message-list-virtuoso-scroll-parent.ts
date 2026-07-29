import { useCallback, useEffect, useRef, useState } from "react";
import { createDebugLogger, isDebug } from "@/lib/debug/log";

const debugScrollParent = createDebugLogger("chat:virtuoso:scrollParent");

/** Defer providing scroll parent to Virtuoso until the element has non-zero size. */
export function useVisibleScrollParent() {
  const [scrollParent, setScrollParent] = useState<HTMLDivElement | null>(null);
  const nodeRef = useRef<HTMLDivElement | null>(null);
  const setScrollRef = useCallback((node: HTMLDivElement | null) => {
    nodeRef.current = node;
    if (node && node.offsetHeight > 0) {
      if (isDebug()) {
        debugScrollParent("ref-callback-ready", {
          offsetHeight: node.offsetHeight,
          path: "synchronous",
        });
      }
      setScrollParent(node);
    } else if (isDebug()) {
      debugScrollParent("ref-callback-defer", {
        hasNode: Boolean(node),
        offsetHeight: node?.offsetHeight ?? null,
        reason: !node ? "no-node" : "zero-height",
      });
    }
  }, []);
  useEffect(() => {
    const node = nodeRef.current;
    if (!node || scrollParent) return;
    if (isDebug()) {
      debugScrollParent("ro-attach", {
        initialHeight: node.offsetHeight,
      });
    }
    const ro = new ResizeObserver((entries) => {
      for (const entry of entries) {
        if (entry.contentRect.height > 0) {
          if (isDebug()) {
            debugScrollParent("ro-ready", {
              height: entry.contentRect.height,
            });
          }
          setScrollParent(node);
          ro.disconnect();
          return;
        }
      }
    });
    ro.observe(node);
    return () => ro.disconnect();
  }, [scrollParent]);
  return { scrollParent, setScrollRef };
}
