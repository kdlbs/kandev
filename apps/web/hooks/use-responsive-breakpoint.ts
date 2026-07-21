import * as React from "react";

const MOBILE_BREAKPOINT = 640;
const COMPACT_DESKTOP_BREAKPOINT = 768;
const DESKTOP_BREAKPOINT = 1024;

export type Breakpoint = "mobile" | "tablet" | "compactDesktop" | "desktop";

export type ResponsiveBreakpoint = {
  breakpoint: Breakpoint;
  isMobile: boolean;
  isTablet: boolean;
  isDesktop: boolean;
  isCompactDesktop: boolean;
  isFullDesktop: boolean;
  isFinePointer: boolean;
  usesDesktopWorkbench: boolean;
};

function getBreakpoint(width: number, isFinePointer: boolean): Breakpoint {
  if (width < MOBILE_BREAKPOINT) {
    return "mobile";
  }
  // Fine-pointer devices below 768px stay on the tablet layout; that range is
  // too narrow to host the full workbench even with a mouse.
  if (width >= COMPACT_DESKTOP_BREAKPOINT && width < DESKTOP_BREAKPOINT && isFinePointer) {
    return "compactDesktop";
  }
  if (width < DESKTOP_BREAKPOINT) {
    return "tablet";
  }
  return "desktop";
}

function buildResponsiveBreakpoint(width: number, isFinePointer: boolean): ResponsiveBreakpoint {
  const breakpoint = getBreakpoint(width, isFinePointer);
  const usesDesktopWorkbench = breakpoint === "compactDesktop" || breakpoint === "desktop";
  return {
    breakpoint,
    isMobile: breakpoint === "mobile",
    isTablet: breakpoint === "tablet",
    isDesktop: usesDesktopWorkbench,
    isCompactDesktop: breakpoint === "compactDesktop",
    isFullDesktop: breakpoint === "desktop",
    isFinePointer,
    usesDesktopWorkbench,
  };
}

function getPointerMode(): boolean {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
    return true;
  }
  return window.matchMedia("(pointer: fine)").matches;
}

function getCurrentResponsiveBreakpoint(): ResponsiveBreakpoint {
  if (typeof window === "undefined") {
    return buildResponsiveBreakpoint(DESKTOP_BREAKPOINT, true);
  }
  return buildResponsiveBreakpoint(window.innerWidth, getPointerMode());
}

const SERVER_SNAPSHOT = buildResponsiveBreakpoint(DESKTOP_BREAKPOINT, true);

// Cached snapshot keeps getSnapshot referentially stable across re-renders —
// useSyncExternalStore loops if getSnapshot returns a fresh object every
// call. The cache is freshened only when the underlying viewport actually
// changes (verified by deep-equal against the live readout), so listeners
// firing without an effective change don't trigger spurious renders.
let cachedClientSnapshot: ResponsiveBreakpoint | null = null;

function breakpointsEqual(a: ResponsiveBreakpoint, b: ResponsiveBreakpoint): boolean {
  // breakpoint + isFinePointer fully determine every other field (see
  // buildResponsiveBreakpoint), so equality on those two is sufficient.
  return a.breakpoint === b.breakpoint && a.isFinePointer === b.isFinePointer;
}

function getClientSnapshot(): ResponsiveBreakpoint {
  const fresh = getCurrentResponsiveBreakpoint();
  if (cachedClientSnapshot !== null && breakpointsEqual(cachedClientSnapshot, fresh)) {
    return cachedClientSnapshot;
  }
  cachedClientSnapshot = fresh;
  return fresh;
}

function subscribeBreakpoint(callback: () => void): () => void {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
    return () => {};
  }
  const mediaQueries = [
    `(max-width: ${MOBILE_BREAKPOINT - 1}px)`,
    `(min-width: ${MOBILE_BREAKPOINT}px) and (max-width: ${COMPACT_DESKTOP_BREAKPOINT - 1}px)`,
    `(min-width: ${COMPACT_DESKTOP_BREAKPOINT}px) and (max-width: ${DESKTOP_BREAKPOINT - 1}px)`,
    `(min-width: ${DESKTOP_BREAKPOINT}px)`,
    "(pointer: fine)",
  ];
  const mediaQueryLists = mediaQueries.map((query) => window.matchMedia(query));
  mediaQueryLists.forEach((mql) => mql.addEventListener("change", callback));
  return () => {
    mediaQueryLists.forEach((mql) => mql.removeEventListener("change", callback));
  };
}

// useSyncExternalStore is React 19's canonical pattern for hooks that
// expose browser-only state. It reads the real viewport on the first
// client render — no useEffect lag, no desktop→tablet flash, no race
// where heavyweight desktop-only components mount on narrow viewports
// before the layout switches. SSR keeps the desktop default via the
// third arg, matching the prior behavior for hydration.
export function useResponsiveBreakpoint(): ResponsiveBreakpoint {
  return React.useSyncExternalStore(subscribeBreakpoint, getClientSnapshot, () => SERVER_SNAPSHOT);
}
