"use client";

import { useCallback } from "react";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useLayoutStore } from "@/lib/state/layout-store";

export type TaskRightPanelsToggleState = {
  isSupported: boolean;
  isReady: boolean;
  isMaximized: boolean;
  isAvailable: boolean;
  rightPanelsVisible: boolean;
  toggleRightPanels: () => void;
};

/**
 * Select the layout owner for the task header's right-panel action.
 *
 * Fine-pointer workbenches use Dockview's environment layout. Coarse-pointer
 * tablet workspaces use the session layout store, even if a Dockview API from
 * a previous desktop render is still present.
 */
export function useTaskRightPanelsToggle(
  effectiveSessionId: string | null | undefined,
): TaskRightPanelsToggleState {
  const { isMobile, isTablet, usesDesktopWorkbench } = useResponsiveBreakpoint();
  const dockviewApi = useDockviewStore((state) => state.api);
  const dockviewVisible = useDockviewStore((state) => state.rightPaneVisible);
  const dockviewAvailable = useDockviewStore((state) => state.rightPaneAvailable);
  const isRestoringLayout = useDockviewStore((state) => state.isRestoringLayout);
  const preMaximizeLayout = useDockviewStore((state) => state.preMaximizeLayout);
  const toggleDockviewRightPanels = useDockviewStore((state) => state.toggleRightPanels);
  const columnsBySessionId = useLayoutStore((state) => state.columnsBySessionId);
  const toggleTabletRightPanel = useLayoutStore((state) => state.toggleRightPanel);

  const isSupported = !isMobile && (isTablet || usesDesktopWorkbench);
  let rightPanelsVisible = dockviewVisible;
  if (isTablet && effectiveSessionId) {
    rightPanelsVisible = columnsBySessionId[effectiveSessionId]?.right ?? true;
  }
  const isMaximized = usesDesktopWorkbench && preMaximizeLayout !== null;
  const isReady = isTablet
    ? Boolean(effectiveSessionId)
    : usesDesktopWorkbench && Boolean(dockviewApi) && !isRestoringLayout && !isMaximized;
  const isAvailable = isTablet ? Boolean(effectiveSessionId) : dockviewAvailable;

  const toggleRightPanels = useCallback(() => {
    if (!isSupported || !isReady || !isAvailable) return;
    if (isTablet) {
      if (effectiveSessionId) toggleTabletRightPanel(effectiveSessionId);
      return;
    }
    toggleDockviewRightPanels();
  }, [
    effectiveSessionId,
    isReady,
    isSupported,
    isAvailable,
    isTablet,
    toggleDockviewRightPanels,
    toggleTabletRightPanel,
  ]);

  return {
    isSupported,
    isReady: isSupported && isReady,
    isMaximized,
    isAvailable,
    rightPanelsVisible,
    toggleRightPanels,
  };
}
