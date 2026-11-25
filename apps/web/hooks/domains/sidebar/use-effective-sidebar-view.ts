import { selectSidebarViews } from "@/lib/state/slices/ui/sidebar-workspace-state";
import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { DEFAULT_VIEW } from "@/lib/state/slices/ui/sidebar-view-builtins";
import { sidebarViewWithDraft } from "@/lib/state/slices/ui/sidebar-view-disclosure";

/**
 * Active sidebar view merged with any in-flight draft. Used by both desktop
 * and mobile sidebars so a draft (e.g. sort flipped to "custom" by a drag)
 * actually drives `applyView` and not just the sort picker.
 */
export function useEffectiveSidebarView(workspaceId?: string | null) {
  const sidebarSlice = useAppStore((state) => selectSidebarViews(state, workspaceId));
  return useMemo(() => {
    const active = sidebarSlice.views.find((v) => v.id === sidebarSlice.activeViewId);
    if (!active) return sidebarSlice.views[0] ?? DEFAULT_VIEW;
    return sidebarViewWithDraft(active, sidebarSlice.draft);
  }, [sidebarSlice.views, sidebarSlice.activeViewId, sidebarSlice.draft]);
}
