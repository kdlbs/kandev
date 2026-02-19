import { useCallback, useSyncExternalStore } from "react";

export type ViewMode = "split" | "unified";

const DIFF_VIEW_MODE_KEY = "diff-view-mode";
const DEFAULT_VIEW_MODE: ViewMode = "unified";
const VIEW_MODE_CHANGE_EVENT = "diff-view-mode-change";

function getStoredViewMode(): ViewMode {
  if (typeof window === "undefined") return DEFAULT_VIEW_MODE;
  const stored = localStorage.getItem(DIFF_VIEW_MODE_KEY);
  return stored === "split" || stored === "unified" ? stored : DEFAULT_VIEW_MODE;
}

function setStoredViewMode(mode: ViewMode): void {
  localStorage.setItem(DIFF_VIEW_MODE_KEY, mode);
  window.dispatchEvent(new CustomEvent(VIEW_MODE_CHANGE_EVENT, { detail: mode }));
}

export function useGlobalViewMode(): [ViewMode, (mode: ViewMode) => void] {
  const subscribe = useCallback((callback: () => void) => {
    window.addEventListener(VIEW_MODE_CHANGE_EVENT, callback);
    window.addEventListener("storage", callback);
    return () => {
      window.removeEventListener(VIEW_MODE_CHANGE_EVENT, callback);
      window.removeEventListener("storage", callback);
    };
  }, []);

  const getSnapshot = useCallback(() => getStoredViewMode(), []);
  const getServerSnapshot = useCallback(() => DEFAULT_VIEW_MODE, []);
  const viewMode = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  return [viewMode, setStoredViewMode];
}
