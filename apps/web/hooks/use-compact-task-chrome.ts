"use client";

import type { ResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

export function shouldUseCompactTaskChrome(breakpoint: ResponsiveBreakpoint): boolean {
  return breakpoint.isMobile || breakpoint.isTablet;
}

export function useCompactTaskChrome(): boolean {
  return shouldUseCompactTaskChrome(useResponsiveBreakpoint());
}

export function shouldUseTouchDrawer(breakpoint: ResponsiveBreakpoint): boolean {
  return !breakpoint.isFinePointer;
}

export function useTouchDrawer(): boolean {
  return shouldUseTouchDrawer(useResponsiveBreakpoint());
}
