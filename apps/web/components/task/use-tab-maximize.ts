"use client";

import { useCallback } from "react";
import { useDockviewStore } from "@/lib/state/dockview-store";

/**
 * Returns a callback that toggles the maximize state for the given dockview
 * group. No-ops for the sidebar group (it has no maximize button either).
 */
export function useToggleGroupMaximize(groupId: string): () => void {
  const isMaximized = useDockviewStore((s) => s.preMaximizeLayout !== null);
  const sidebarGroupId = useDockviewStore((s) => s.sidebarGroupId);
  const maximizeGroup = useDockviewStore((s) => s.maximizeGroup);
  const exitMaximizedLayout = useDockviewStore((s) => s.exitMaximizedLayout);

  return useCallback(() => {
    if (groupId === sidebarGroupId) return;
    if (isMaximized) {
      exitMaximizedLayout();
    } else {
      maximizeGroup(groupId);
    }
  }, [groupId, sidebarGroupId, isMaximized, maximizeGroup, exitMaximizedLayout]);
}
