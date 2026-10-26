"use client";

import {
  useCallback,
  useLayoutEffect,
  useRef,
  useState,
  type Dispatch,
  type RefObject,
  type SetStateAction,
} from "react";

const MIN_HEIGHT = 120;
const DEFAULT_INITIAL_MAX_HEIGHT_VH = 50;
const EXPAND_GEOMETRY_TOLERANCE_PX = 2;
const EMPTY_REFS: readonly RefObject<HTMLElement | null>[] = [];

type ClarificationResizeOptions = {
  initialMaxHeightVh?: number;
  allocationRef?: RefObject<HTMLElement | null>;
  anchorRef?: RefObject<HTMLElement | null>;
  reservedSiblingRefs?: readonly RefObject<HTMLElement | null>[];
  reservedHeightPx?: number;
  isPhone?: boolean;
  sizingState?: ClarificationSizingState;
};
type ClarificationSizingState = {
  desktopHeight: number | null;
  phoneHeight: number | null;
  setDesktopHeight: Dispatch<SetStateAction<number | null>>;
  setPhoneHeight: Dispatch<SetStateAction<number | null>>;
};

type VerticalInterval = { top: number; bottom: number };
type HostMeasurement = { hostHeight: number | null; viewportHeight: number };

function viewportBounds(): VerticalInterval {
  if (typeof window === "undefined") return { top: 0, bottom: 800 };
  const visualViewport = window.visualViewport;
  const top = visualViewport?.offsetTop ?? 0;
  return {
    top,
    bottom: top + (visualViewport?.height ?? window.innerHeight),
  };
}

function intersectIntervals(a: VerticalInterval, b: VerticalInterval): VerticalInterval {
  return { top: Math.max(a.top, b.top), bottom: Math.min(a.bottom, b.bottom) };
}

function isScrollableClip(element: HTMLElement): boolean {
  const overflowY = window.getComputedStyle(element).overflowY;
  return (
    overflowY === "auto" || overflowY === "scroll" || overflowY === "hidden" || overflowY === "clip"
  );
}

function clippedHostBounds(
  allocation: HTMLElement | null,
  anchor: HTMLElement | null,
): VerticalInterval {
  const viewport = viewportBounds();
  let bounds = allocation
    ? intersectIntervals(allocation.getBoundingClientRect(), viewport)
    : viewport;

  if (anchor) {
    bounds.top = Math.max(bounds.top, anchor.getBoundingClientRect().top);
  }

  const origin = allocation ?? anchor;
  let parent = origin?.parentElement ?? null;
  while (parent) {
    if (isScrollableClip(parent)) {
      bounds = intersectIntervals(bounds, parent.getBoundingClientRect());
    }
    parent = parent.parentElement;
  }

  return bounds;
}

function reservedIntervals(
  refs: readonly RefObject<HTMLElement | null>[],
  bounds: VerticalInterval,
): VerticalInterval[] {
  return refs.flatMap(({ current }) => {
    if (!current) return [];
    const rect = intersectIntervals(current.getBoundingClientRect(), bounds);
    return rect.bottom > rect.top ? [rect] : [];
  });
}

function unionHeight(intervals: VerticalInterval[]): number {
  const sorted = intervals.sort((a, b) => a.top - b.top);
  let total = 0;
  let current: VerticalInterval | null = null;
  for (const interval of sorted) {
    if (!current) {
      current = interval;
      continue;
    }
    if (interval.top <= current.bottom) {
      current.bottom = Math.max(current.bottom, interval.bottom);
      continue;
    }
    total += current.bottom - current.top;
    current = interval;
  }
  return total + (current ? current.bottom - current.top : 0);
}

function measuredHostHeight(
  allocation: HTMLElement | null,
  anchor: HTMLElement | null,
  reservedRefs: readonly RefObject<HTMLElement | null>[],
  reservedHeightPx: number,
): number | null {
  if (allocation && !allocation.isConnected) return null;
  if (anchor && !anchor.isConnected) return null;
  const bounds = clippedHostBounds(allocation, anchor);
  if (!Number.isFinite(bounds.top) || !Number.isFinite(bounds.bottom)) return null;
  const height = Math.max(0, bounds.bottom - bounds.top);
  const siblings = unionHeight(reservedIntervals(reservedRefs, bounds));
  return Math.max(0, Math.floor(height - siblings - reservedHeightPx));
}

function currentViewportHeight(): number {
  const bounds = viewportBounds();
  return bounds.bottom - bounds.top;
}

function clampHeight(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}

function useMeasuredHostHeight(
  allocationRef: RefObject<HTMLElement | null> | undefined,
  anchorRef: RefObject<HTMLElement | null> | undefined,
  reservedSiblingRefs: readonly RefObject<HTMLElement | null>[],
  reservedHeightPx: number,
): HostMeasurement {
  const [measurement, setMeasurement] = useState<HostMeasurement>(() => ({
    hostHeight: null,
    viewportHeight: currentViewportHeight(),
  }));

  useLayoutEffect(() => {
    const allocation = allocationRef?.current ?? null;
    const anchor = anchorRef?.current ?? null;
    const refs = [
      ...(allocation ? [allocation] : []),
      ...(anchor ? [anchor] : []),
      ...reservedSiblingRefs.flatMap((ref) => (ref.current ? [ref.current] : [])),
    ];
    const clippingParents: HTMLElement[] = [];
    let parent = (allocation ?? anchor)?.parentElement ?? null;
    while (parent) {
      if (isScrollableClip(parent)) clippingParents.push(parent);
      parent = parent.parentElement;
    }
    const observed = [...new Set([...refs, ...clippingParents])];
    let frame = 0;
    const measure = () => {
      const next = measuredHostHeight(
        allocationRef?.current ?? null,
        anchorRef?.current ?? null,
        reservedSiblingRefs,
        reservedHeightPx,
      );
      const nextViewportHeight = currentViewportHeight();
      setMeasurement((current) =>
        current.hostHeight === next && current.viewportHeight === nextViewportHeight
          ? current
          : { hostHeight: next, viewportHeight: nextViewportHeight },
      );
    };
    const scheduleMeasure = () => {
      if (frame !== 0) return;
      frame = window.requestAnimationFrame(() => {
        frame = 0;
        measure();
      });
    };
    const observer =
      typeof ResizeObserver === "undefined" ? null : new ResizeObserver(scheduleMeasure);
    observed.forEach((element) => observer?.observe(element));
    const viewport = window.visualViewport;
    window.addEventListener("resize", scheduleMeasure);
    window.addEventListener("scroll", scheduleMeasure, true);
    viewport?.addEventListener("resize", scheduleMeasure);
    viewport?.addEventListener("scroll", scheduleMeasure);
    measure();

    return () => {
      if (frame !== 0) window.cancelAnimationFrame(frame);
      observer?.disconnect();
      window.removeEventListener("resize", scheduleMeasure);
      window.removeEventListener("scroll", scheduleMeasure, true);
      viewport?.removeEventListener("resize", scheduleMeasure);
      viewport?.removeEventListener("scroll", scheduleMeasure);
    };
  }, [allocationRef, anchorRef, reservedSiblingRefs, reservedHeightPx]);

  return measurement;
}

function useMeasuredPanelHeight(element: HTMLDivElement | null): number | null {
  const [panelHeight, setPanelHeight] = useState<number | null>(null);
  const measure = useCallback(() => {
    if (!element) {
      setPanelHeight(null);
      return;
    }
    const next = element.getBoundingClientRect().height;
    if (!Number.isFinite(next)) return;
    setPanelHeight((current) =>
      current !== null && Math.abs(current - next) < 0.5 ? current : next,
    );
  }, [element]);

  useLayoutEffect(() => {
    measure();
    if (!element || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, [element, measure]);

  return panelHeight;
}

function maxHeightForState(
  desired: number | null,
  host: number | null,
  initialCap: number,
): number {
  if (desired !== null) return host ?? initialCap;
  if (host === null) return initialCap;
  return Math.min(initialCap, host);
}

function useResolvedSizingState(sharedState?: ClarificationSizingState): ClarificationSizingState {
  const [desktopHeight, setDesktopHeight] = useState<number | null>(null);
  const [phoneHeight, setPhoneHeight] = useState<number | null>(null);
  return sharedState ?? { desktopHeight, phoneHeight, setDesktopHeight, setPhoneHeight };
}

function activeSizingState(isPhone: boolean, sizingState: ClarificationSizingState) {
  if (isPhone) {
    return {
      desiredHeight: sizingState.phoneHeight,
      setDesiredHeight: sizingState.setPhoneHeight,
    };
  }
  return {
    desiredHeight: sizingState.desktopHeight,
    setDesiredHeight: sizingState.setDesktopHeight,
  };
}

function effectiveHeightFor(
  desiredHeight: number | null,
  hostHeight: number | null,
  initialCap: number,
): number | null {
  if (desiredHeight === null) return null;
  return Math.min(desiredHeight, hostHeight ?? initialCap);
}

function canExpandToHost(hostHeight: number | null, panelHeight: number): boolean {
  return hostHeight !== null && hostHeight - panelHeight > EXPAND_GEOMETRY_TOLERANCE_PX;
}

function useResizeDrag(
  containerRef: RefObject<HTMLDivElement | null>,
  hostHeight: number | null,
  initialCap: number,
  minHeight: number,
  setDesiredHeight: Dispatch<SetStateAction<number | null>>,
) {
  const isDragging = useRef(false);
  const startY = useRef(0);
  const startHeight = useRef(0);
  const handleMouseDown = useCallback(
    (event: React.MouseEvent) => {
      event.preventDefault();
      const measured = containerRef.current?.getBoundingClientRect().height ?? minHeight;
      isDragging.current = true;
      startY.current = event.clientY;
      startHeight.current = measured;
      document.body.style.cursor = "ns-resize";
      document.body.style.userSelect = "none";
    },
    [containerRef, minHeight],
  );

  useLayoutEffect(() => {
    const handleMouseMove = (event: MouseEvent) => {
      if (!isDragging.current) return;
      const delta = startY.current - event.clientY;
      const max = hostHeight ?? initialCap;
      setDesiredHeight(clampHeight(startHeight.current + delta, Math.min(MIN_HEIGHT, max), max));
    };
    const handleMouseUp = () => {
      if (!isDragging.current) return;
      isDragging.current = false;
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
    };
    document.addEventListener("mousemove", handleMouseMove);
    document.addEventListener("mouseup", handleMouseUp);
    return () => {
      document.removeEventListener("mousemove", handleMouseMove);
      document.removeEventListener("mouseup", handleMouseUp);
      if (isDragging.current) {
        isDragging.current = false;
        document.body.style.cursor = "";
        document.body.style.userSelect = "";
      }
    };
  }, [hostHeight, initialCap, setDesiredHeight]);

  return handleMouseDown;
}

/**
 * Measures the fixed host space available to a pending clarification. Desired
 * heights remain independent from that changing space, so a temporary host or
 * keyboard contraction does not erase the user's selected size.
 */
export function useResizableClarificationOverlay({
  initialMaxHeightVh = DEFAULT_INITIAL_MAX_HEIGHT_VH,
  allocationRef,
  anchorRef,
  reservedSiblingRefs = EMPTY_REFS,
  reservedHeightPx = 0,
  isPhone = false,
  sizingState,
}: ClarificationResizeOptions = {}) {
  const { hostHeight, viewportHeight } = useMeasuredHostHeight(
    allocationRef,
    anchorRef,
    reservedSiblingRefs,
    reservedHeightPx,
  );
  const resolvedSizingState = useResolvedSizingState(sizingState);
  const { setDesktopHeight, setPhoneHeight } = resolvedSizingState;
  const containerElementRef = useRef<HTMLDivElement>(null);
  const [containerElement, setContainerElement] = useState<HTMLDivElement | null>(null);
  const containerRef = useCallback((element: HTMLDivElement | null) => {
    containerElementRef.current = element;
    setContainerElement(element);
  }, []);
  const measuredPanelHeight = useMeasuredPanelHeight(containerElement);

  const { desiredHeight, setDesiredHeight } = activeSizingState(isPhone, resolvedSizingState);
  const initialCap = Math.round((viewportHeight * initialMaxHeightVh) / 100);
  const maxHeight = maxHeightForState(desiredHeight, hostHeight, initialCap);
  const maxHostHeight = hostHeight;
  const minHeight = Math.min(MIN_HEIGHT, hostHeight ?? initialCap);
  const effectiveHeight = effectiveHeightFor(desiredHeight, hostHeight, initialCap);
  const renderedHeight =
    measuredPanelHeight ??
    containerElement?.getBoundingClientRect().height ??
    effectiveHeight ??
    maxHeight;

  const resetHeight = useCallback(() => setDesiredHeight(null), [setDesiredHeight]);
  const resetAllHeights = useCallback(() => {
    setDesktopHeight(null);
    setPhoneHeight(null);
  }, [setDesktopHeight, setPhoneHeight]);
  const expandHeight = useCallback(() => {
    if (hostHeight !== null) setDesiredHeight(hostHeight);
  }, [hostHeight, setDesiredHeight]);
  const handleMouseDown = useResizeDrag(
    containerElementRef,
    hostHeight,
    initialCap,
    minHeight,
    setDesiredHeight,
  );

  return {
    height: desiredHeight,
    effectiveHeight,
    maxHeight,
    maxHostHeight,
    minHeight,
    canExpand: canExpandToHost(hostHeight, renderedHeight),
    canReset: desiredHeight !== null,
    containerRef,
    expandHeight,
    resetHeight,
    resetAllHeights,
    resizeHandleProps: { onMouseDown: handleMouseDown, onDoubleClick: resetHeight },
  };
}
