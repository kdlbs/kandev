import { useEffect, type RefObject } from "react";
import type { DockviewApi } from "dockview-react";
import { panelPortalManager } from "@/lib/layout/panel-portal-manager";
import { useDockviewStore } from "@/lib/state/dockview-store";

export function useCompactDockviewDefault(compact: boolean) {
  const setDefaultPreset = useDockviewStore((s) => s.setDefaultPreset);
  useEffect(() => {
    setDefaultPreset(compact ? "compact" : "default");
    return () => setDefaultPreset("default");
  }, [compact, setDefaultPreset]);
}

export function useDockviewUnmountCleanup(
  saveTimerRef: RefObject<ReturnType<typeof setTimeout> | null>,
  readyDisposersRef: RefObject<Array<() => void>>,
  ownedApiRef: RefObject<DockviewApi | null>,
) {
  useEffect(() => {
    const timerRef = saveTimerRef;
    const disposersRef = readyDisposersRef;
    return () => {
      for (const dispose of disposersRef.current.splice(0)) dispose();
      if (timerRef.current) clearTimeout(timerRef.current);
      const owner = ownedApiRef.current;
      const state = useDockviewStore.getState();
      if (owner && state.api === owner) {
        panelPortalManager.releaseAll();
        state.setApi(null);
      }
    };
  }, [ownedApiRef, readyDisposersRef, saveTimerRef]);
}
