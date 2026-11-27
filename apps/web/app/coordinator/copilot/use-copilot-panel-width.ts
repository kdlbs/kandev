import { useCallback, useEffect, useState } from "react";
import { getLocalStorage, setLocalStorage } from "@/lib/local-storage";
import { PREVIEW_PANEL } from "@/lib/settings/constants";

export const COPILOT_WIDTH_STORAGE_KEY = "kandev.coordinatorCopilot.width";

function readStoredWidth(): number {
  const stored = getLocalStorage<number>(COPILOT_WIDTH_STORAGE_KEY, PREVIEW_PANEL.DEFAULT_WIDTH_PX);
  return Number.isFinite(stored) && stored > 0 ? stored : PREVIEW_PANEL.DEFAULT_WIDTH_PX;
}

/**
 * The copilot panel's chosen width, persisted under its own key so resizing it
 * never moves the board preview's width. The value is floored to the panel's
 * minimum at render time, never here.
 */
export function useCopilotPanelWidth() {
  const [widthPx, setWidthPx] = useState<number>(PREVIEW_PANEL.DEFAULT_WIDTH_PX);

  useEffect(() => {
    setWidthPx(readStoredWidth());
  }, []);

  const updateWidth = useCallback((next: number) => {
    setWidthPx(next);
    setLocalStorage(COPILOT_WIDTH_STORAGE_KEY, next);
  }, []);

  return { widthPx, updateWidth };
}
