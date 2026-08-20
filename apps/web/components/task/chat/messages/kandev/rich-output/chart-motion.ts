import { useSyncExternalStore } from "react";
import { useOptionalAppStore } from "@/components/state-provider";

const REDUCED_MOTION_QUERY = "(prefers-reduced-motion: reduce)";
const reducedMotionListeners = new Set<() => void>();
let reducedMotionMediaQuery: MediaQueryList | undefined;

function getReducedMotionMediaQuery(): MediaQueryList | undefined {
  if (reducedMotionMediaQuery) return reducedMotionMediaQuery;
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") return undefined;
  reducedMotionMediaQuery = window.matchMedia(REDUCED_MOTION_QUERY);
  return reducedMotionMediaQuery;
}

function notifyReducedMotionListeners() {
  for (const listener of reducedMotionListeners) listener();
}

function subscribeToReducedMotion(listener: () => void): () => void {
  const mediaQuery = getReducedMotionMediaQuery();
  reducedMotionListeners.add(listener);
  if (reducedMotionListeners.size === 1 && mediaQuery) {
    mediaQuery.addEventListener("change", notifyReducedMotionListeners);
  }
  return () => {
    reducedMotionListeners.delete(listener);
    if (reducedMotionListeners.size === 0 && mediaQuery) {
      mediaQuery.removeEventListener("change", notifyReducedMotionListeners);
    }
  };
}

function getReducedMotionSnapshot(): boolean {
  return getReducedMotionMediaQuery()?.matches ?? false;
}

export function useRichOutputChartAnimations(): boolean {
  const enabled = useOptionalAppStore((state) => state.richOutputMotion.enabled, true);
  const reducedMotion = useSyncExternalStore(
    subscribeToReducedMotion,
    getReducedMotionSnapshot,
    () => false,
  );
  return enabled && !reducedMotion;
}
