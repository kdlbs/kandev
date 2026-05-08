import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";

/**
 * Active sidebar view merged with any in-flight draft. Used by both desktop
 * and mobile sidebars so a draft (e.g. sort flipped to "custom" by a drag)
 * actually drives `applyView` and not just the sort picker.
 */
export function useEffectiveSidebarView() {
  const sidebarSlice = useAppStore((state) => state.sidebarViews);
  return useMemo(() => {
    const active = sidebarSlice.views.find((v) => v.id === sidebarSlice.activeViewId);
    if (!active) return sidebarSlice.views[0];
    const d = sidebarSlice.draft;
    if (!d || d.baseViewId !== active.id) return active;
    return { ...active, filters: d.filters, sort: d.sort, group: d.group };
  }, [sidebarSlice.views, sidebarSlice.activeViewId, sidebarSlice.draft]);
}
