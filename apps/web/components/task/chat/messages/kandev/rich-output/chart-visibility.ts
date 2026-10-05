import { useEffect, useRef, useState } from "react";

const CHART_PREWARM_MARGIN_PX = 200;
// i18n-exempt: IntersectionObserver geometry, not user-facing copy.
const CHART_PREWARM_MARGIN = `${CHART_PREWARM_MARGIN_PX}px 0px`;

function getScrollRoot(element: HTMLElement): Element | null {
  let ancestor = element.parentElement;
  while (ancestor) {
    const style = window.getComputedStyle(ancestor);
    if (
      /^(auto|scroll|overlay)$/.test(style.overflowY) &&
      ancestor.scrollHeight > ancestor.clientHeight
    ) {
      return ancestor;
    }
    ancestor = ancestor.parentElement;
  }
  return null;
}

function isWithinPrewarmRange(element: HTMLElement, root: Element | null): boolean {
  const rect = element.getBoundingClientRect();
  if (rect.width <= 0 || rect.height <= 0) return false;
  const rootRect = root?.getBoundingClientRect();
  const rootTop = rootRect?.top ?? 0;
  const rootBottom = rootRect?.bottom ?? window.innerHeight;
  const rootLeft = rootRect?.left ?? 0;
  const rootRight = rootRect?.right ?? window.innerWidth;

  return (
    rect.bottom >= rootTop - CHART_PREWARM_MARGIN_PX &&
    rect.top <= rootBottom + CHART_PREWARM_MARGIN_PX &&
    rect.right >= rootLeft &&
    rect.left <= rootRight
  );
}

export function useChartPlotVisibility() {
  const plotRef = useRef<HTMLDivElement | null>(null);
  const canObserveIntersection = typeof IntersectionObserver !== "undefined";
  const [isNearViewport, setIsNearViewport] = useState(!canObserveIntersection);
  const [isDocumentVisible, setIsDocumentVisible] = useState(
    () => typeof document === "undefined" || document.visibilityState !== "hidden",
  );
  const [shouldMountPlot, setShouldMountPlot] = useState(() => !canObserveIntersection);

  useEffect(() => {
    const plot = plotRef.current;
    if (shouldMountPlot || !plot || !canObserveIntersection) return;

    const root = getScrollRoot(plot);
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (!entry.isIntersecting) return;
        setIsNearViewport(true);
        observer.disconnect();
      },
      { root, rootMargin: CHART_PREWARM_MARGIN },
    );
    observer.observe(plot);
    const checkScrollRange = () => {
      if (!isWithinPrewarmRange(plot, root)) return;
      setIsNearViewport(true);
      observer.disconnect();
    };
    document.addEventListener("scroll", checkScrollRange, true);
    window.addEventListener("resize", checkScrollRange);
    checkScrollRange();
    return () => {
      observer.disconnect();
      document.removeEventListener("scroll", checkScrollRange, true);
      window.removeEventListener("resize", checkScrollRange);
    };
  }, [canObserveIntersection, shouldMountPlot]);

  useEffect(() => {
    const handleVisibilityChange = () => {
      setIsDocumentVisible(document.visibilityState !== "hidden");
    };
    document.addEventListener("visibilitychange", handleVisibilityChange);
    handleVisibilityChange();
    return () => document.removeEventListener("visibilitychange", handleVisibilityChange);
  }, []);

  useEffect(() => {
    if (isNearViewport && isDocumentVisible) setShouldMountPlot(true);
  }, [isDocumentVisible, isNearViewport]);

  return { plotRef, shouldMountPlot };
}
