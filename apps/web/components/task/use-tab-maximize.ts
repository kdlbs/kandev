"use client";

import { useCallback } from "react";
import type { DockviewPanelApi } from "dockview-react";
import { useDockviewStore } from "@/lib/state/dockview-store";

/**
 * Returns an `onDoubleClick` handler that toggles the maximize state for the
 * dockview group containing the given panel. No-ops for the sidebar group
 * (which has no maximize button either) and while the dockview store is in
 * the middle of restoring a layout (guards against rapid double-firing on
 * back-to-back dblclicks during the maximize/restore animation frame).
 *
 * Reads `api.group.id` and store state at call time rather than render time
 * so the toggle remains correct after dockview rebuilds groups via fromJSON.
 */
export function useTabMaximizeOnDoubleClick(
  api: Pick<DockviewPanelApi, "group">,
): (event: React.MouseEvent) => void {
  return useCallback(
    (event) => {
      event.stopPropagation();
      event.preventDefault();
      const groupId = api.group?.id;
      if (!groupId) return;
      const state = useDockviewStore.getState();
      if (state.isRestoringLayout) return;
      if (groupId === state.sidebarGroupId) return;
      if (state.preMaximizeLayout) {
        state.exitMaximizedLayout();
      } else {
        state.maximizeGroup(groupId);
      }
    },
    [api],
  );
}
