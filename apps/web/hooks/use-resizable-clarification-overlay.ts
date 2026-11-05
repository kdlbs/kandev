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

export type ClarificationPanelSizeMode = "auto" | "manual" | "expanded";
export type ClarificationNormalPanelSize = {
  mode: "auto" | "manual";
  height: number | null;
};
export type ClarificationPanelSize = {
  mode: ClarificationPanelSizeMode;
  height: number | null;
  restore: ClarificationNormalPanelSize | null;
};
export type ClarificationPanelSizes = {
  desktop: ClarificationPanelSize;
  phone: ClarificationPanelSize;
};
export type ClarificationSizingState = ClarificationPanelSizes & {
  setDesktopSize: Dispatch<SetStateAction<ClarificationPanelSize>>;
  setPhoneSize: Dispatch<SetStateAction<ClarificationPanelSize>>;
};

export const DEFAULT_CLARIFICATION_PANEL_SIZE: ClarificationPanelSize = {
  mode: "auto",
  height: null,
  restore: null,
};

export function emptyClarificationPanelSizes(): ClarificationPanelSizes {
  return {
    desktop: DEFAULT_CLARIFICATION_PANEL_SIZE,
    phone: DEFAULT_CLARIFICATION_PANEL_SIZE,
  };
}

function isDefaultPanelSize(size: ClarificationPanelSize): boolean {
  return size.mode === "auto" && size.height === null && size.restore === null;
}

export function isEmptyClarificationPanelSizes(sizes: ClarificationPanelSizes): boolean {
  return isDefaultPanelSize(sizes.desktop) && isDefaultPanelSize(sizes.phone);
}

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

function maxHeightForSize(
  size: ClarificationPanelSize,
  host: number | null,
  initialCap: number,
): number {
  if (size.mode === "auto") {
    return host === null ? initialCap : Math.min(initialCap, host);
  }
  return host ?? initialCap;
}

function heightForSize(
  size: ClarificationPanelSize,
  host: number | null,
  initialCap: number,
): number | null {
  if (size.mode === "expanded") return host ?? initialCap;
  if (size.mode === "manual") return size.height;
  return null;
}

function effectiveHeightFor(
  size: ClarificationPanelSize,
  hostHeight: number | null,
  initialCap: number,
): number | null {
  if (size.mode === "expanded") return hostHeight ?? initialCap;
  if (size.mode === "manual" && size.height !== null) {
    return Math.min(size.height, hostHeight ?? initialCap);
  }
  return null;
}

function normalPanelSize(size: ClarificationPanelSize): ClarificationNormalPanelSize {
  if (size.mode === "manual" && size.height !== null) {
    return { mode: "manual", height: size.height };
  }
  return { mode: "auto", height: null };
}

function expandedPanelSize(size: ClarificationPanelSize): ClarificationPanelSize {
  return { mode: "expanded", height: null, restore: normalPanelSize(size) };
}

function restoredPanelSize(size: ClarificationPanelSize): ClarificationPanelSize {
  if (size.mode !== "expanded") return size;
  const restore = size.restore ?? DEFAULT_CLARIFICATION_PANEL_SIZE;
  return { mode: restore.mode, height: restore.height, restore: null };
}

function useResolvedSizingState(sharedState?: ClarificationSizingState): ClarificationSizingState {
  const [desktop, setDesktopSize] = useState<ClarificationPanelSize>(
    DEFAULT_CLARIFICATION_PANEL_SIZE,
  );
  const [phone, setPhoneSize] = useState<ClarificationPanelSize>(DEFAULT_CLARIFICATION_PANEL_SIZE);
  return sharedState ?? { desktop, phone, setDesktopSize, setPhoneSize };
}

function activeSizingState(isPhone: boolean, sizingState: ClarificationSizingState) {
  if (isPhone) {
    return { size: sizingState.phone, setSize: sizingState.setPhoneSize };
  }
  return { size: sizingState.desktop, setSize: sizingState.setDesktopSize };
}

function canExpandToHost(hostHeight: number | null, panelHeight: number): boolean {
  return hostHeight !== null && hostHeight - panelHeight > EXPAND_GEOMETRY_TOLERANCE_PX;
}

function useResizeDrag(
  containerRef: RefObject<HTMLDivElement | null>,
  hostHeight: number | null,
  initialCap: number,
  minHeight: number,
  setSize: Dispatch<SetStateAction<ClarificationPanelSize>>,
) {
  const isDragging = useRef(false);
  const hasMoved = useRef(false);
  const startY = useRef(0);
  const startHeight = useRef(0);
  const handleMouseDown = useCallback(
    (event: React.MouseEvent) => {
      event.preventDefault();
      const measured = containerRef.current?.getBoundingClientRect().height ?? minHeight;
      isDragging.current = true;
      hasMoved.current = false;
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
      if (delta === 0 && !hasMoved.current) return;
      hasMoved.current = true;
      const max = hostHeight ?? initialCap;
      const height = clampHeight(startHeight.current + delta, Math.min(MIN_HEIGHT, max), max);
      setSize({ mode: "manual", height, restore: null });
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
  }, [hostHeight, initialCap, setSize]);

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
  const { setDesktopSize, setPhoneSize } = resolvedSizingState;
  const containerElementRef = useRef<HTMLDivElement>(null);
  const [containerElement, setContainerElement] = useState<HTMLDivElement | null>(null);
  const containerRef = useCallback((element: HTMLDivElement | null) => {
    containerElementRef.current = element;
    setContainerElement(element);
  }, []);
  const measuredPanelHeight = useMeasuredPanelHeight(containerElement);

  const { size, setSize } = activeSizingState(isPhone, resolvedSizingState);
  const initialCap = Math.round((viewportHeight * initialMaxHeightVh) / 100);
  const maxHeight = maxHeightForSize(size, hostHeight, initialCap);
  const maxHostHeight = hostHeight;
  const minHeight = Math.min(MIN_HEIGHT, hostHeight ?? initialCap);
  const height = heightForSize(size, hostHeight, initialCap);
  const effectiveHeight = effectiveHeightFor(size, hostHeight, initialCap);
  const renderedHeight =
    measuredPanelHeight ??
    containerElement?.getBoundingClientRect().height ??
    effectiveHeight ??
    maxHeight;

  const resetHeight = useCallback(() => setSize(DEFAULT_CLARIFICATION_PANEL_SIZE), [setSize]);
  const resetAllHeights = useCallback(() => {
    setDesktopSize(DEFAULT_CLARIFICATION_PANEL_SIZE);
    setPhoneSize(DEFAULT_CLARIFICATION_PANEL_SIZE);
  }, [setDesktopSize, setPhoneSize]);
  const measureLiveHostHeight = useCallback(
    () =>
      measuredHostHeight(
        allocationRef?.current ?? null,
        anchorRef?.current ?? null,
        reservedSiblingRefs,
        reservedHeightPx,
      ),
    [allocationRef, anchorRef, reservedHeightPx, reservedSiblingRefs],
  );
  const toggleHeight = useCallback(() => {
    setSize((current) => {
      if (current.mode === "expanded") return restoredPanelSize(current);
      const liveHostHeight = measureLiveHostHeight() ?? hostHeight;
      const livePanelHeight =
        containerElementRef.current?.getBoundingClientRect().height ?? renderedHeight;
      if (!canExpandToHost(liveHostHeight, livePanelHeight)) return current;
      return expandedPanelSize(current);
    });
  }, [hostHeight, measureLiveHostHeight, renderedHeight, setSize]);
  const handleMouseDown = useResizeDrag(
    containerElementRef,
    hostHeight,
    initialCap,
    minHeight,
    setSize,
  );

  return {
    expanded: size.mode === "expanded",
    height,
    effectiveHeight,
    maxHeight,
    maxHostHeight,
    minHeight,
    canExpand: size.mode !== "expanded" && canExpandToHost(hostHeight, renderedHeight),
    containerRef,
    toggleHeight,
    resetHeight,
    resetAllHeights,
    resizeHandleProps: { onMouseDown: handleMouseDown, onDoubleClick: resetHeight },
  };
}
