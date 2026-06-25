"use client";

import { type RefObject, useCallback, useLayoutEffect, useState } from "react";
import {
  DEFAULT_SCALE,
  SCALE_STEP,
  MIN_SCALE,
  MAX_SCALE,
  calculateMermaidFitScale,
  getElementContentWidth,
} from "./mermaid-utils";

export function useMermaidViewportWidth(scrollRegionRef: RefObject<HTMLElement | null>): number {
  const [viewportWidth, setViewportWidth] = useState(0);

  const measureViewport = useCallback(() => {
    if (!scrollRegionRef.current) return;
    if (window.getComputedStyle(scrollRegionRef.current).display === "none") return;
    setViewportWidth(getElementContentWidth(scrollRegionRef.current));
  }, [scrollRegionRef]);

  useLayoutEffect(() => {
    measureViewport();
    const element = scrollRegionRef.current;
    if (!element) return;

    if (typeof ResizeObserver === "undefined") {
      window.addEventListener("resize", measureViewport);
      return () => window.removeEventListener("resize", measureViewport);
    }

    const observer = new ResizeObserver(measureViewport);
    observer.observe(element);
    return () => observer.disconnect();
  }, [measureViewport, scrollRegionRef]);

  return viewportWidth;
}

export function useMermaidScale(svgSize: { w: number; h: number } | null, viewportWidth: number) {
  const [scale, setScale] = useState(DEFAULT_SCALE);
  const [manualScale, setManualScale] = useState(false);
  const fitScale = svgSize
    ? calculateMermaidFitScale({ viewportWidth, svgWidth: svgSize.w })
    : DEFAULT_SCALE;

  useLayoutEffect(() => {
    if (!manualScale) {
      setScale(fitScale);
    }
  }, [fitScale, manualScale]);

  const zoomIn = useCallback(() => {
    setManualScale(true);
    setScale((s) => Math.min(s + SCALE_STEP, MAX_SCALE));
  }, []);
  const zoomOut = useCallback(() => {
    setManualScale(true);
    setScale((s) => Math.max(s - SCALE_STEP, MIN_SCALE));
  }, []);
  const zoomReset = useCallback(() => {
    setManualScale(false);
    setScale(fitScale);
  }, [fitScale]);
  const resetAutoScale = useCallback(() => setManualScale(false), []);

  return { scale, zoomIn, zoomOut, zoomReset, resetAutoScale };
}
