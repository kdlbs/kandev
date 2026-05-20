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

export function useResponsiveBreakpoint(): ResponsiveBreakpoint {
  const [state, setState] = React.useState<ResponsiveBreakpoint>(() =>
    buildResponsiveBreakpoint(DESKTOP_BREAKPOINT, true),
  );

  React.useEffect(() => {
    const updateBreakpoint = () => {
      setState(getCurrentResponsiveBreakpoint());
    };

    // Set initial value
    updateBreakpoint();

    // Listen for resize events
    const mediaQueries = [
      `(max-width: ${MOBILE_BREAKPOINT - 1}px)`,
      `(min-width: ${MOBILE_BREAKPOINT}px) and (max-width: ${COMPACT_DESKTOP_BREAKPOINT - 1}px)`,
      `(min-width: ${COMPACT_DESKTOP_BREAKPOINT}px) and (max-width: ${DESKTOP_BREAKPOINT - 1}px)`,
      `(min-width: ${DESKTOP_BREAKPOINT}px)`,
      "(pointer: fine)",
    ];
    const mediaQueryLists = mediaQueries.map((query) => window.matchMedia(query));

    mediaQueryLists.forEach((mql) => mql.addEventListener("change", updateBreakpoint));

    return () => {
      mediaQueryLists.forEach((mql) => mql.removeEventListener("change", updateBreakpoint));
    };
  }, []);

  return state;
}
