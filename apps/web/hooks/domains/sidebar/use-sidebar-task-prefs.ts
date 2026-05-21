import { useCallback } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { mergeGroupOrder } from "@/lib/sidebar/apply-view";

/**
 * Read sidebar pin/order prefs and expose drag/pin handlers.
 *
 * `handleReorderGroup` records the new order *and* flips the active sort to
 * "Custom" via the view draft — read draft-first so subsequent drags don't
 * try to re-set custom on top of an already-custom draft. Both desktop and
 * mobile sidebars use this hook to stay in sync.
 *
 * `handleReorderSubtasks` is intentionally scoped to one parent: it writes to
 * `subtaskOrderByParentId` only, so the root-task sort is never disturbed.
 */
export function useSidebarTaskPrefs() {
  const store = useAppStoreApi();
  const pinnedTaskIds = useAppStore((s) => s.sidebarTaskPrefs.pinnedTaskIds);
  const orderedTaskIds = useAppStore((s) => s.sidebarTaskPrefs.orderedTaskIds);
  const subtaskOrderByParentId = useAppStore((s) => s.sidebarTaskPrefs.subtaskOrderByParentId);
  const togglePinnedTask = useAppStore((s) => s.togglePinnedTask);
  const setSidebarTaskOrder = useAppStore((s) => s.setSidebarTaskOrder);
  const setSubtaskOrder = useAppStore((s) => s.setSubtaskOrder);
  const updateSidebarDraft = useAppStore((s) => s.updateSidebarDraft);

  const handleReorderGroup = useCallback(
    (groupTaskIds: string[]) => {
      const state = store.getState();
      const current = state.sidebarTaskPrefs.orderedTaskIds;
      setSidebarTaskOrder(mergeGroupOrder(current, groupTaskIds));
      const sliceState = state.sidebarViews;
      const baseSort =
        sliceState.draft?.sort ??
        sliceState.views.find((v) => v.id === sliceState.activeViewId)?.sort;
      if (!baseSort || baseSort.key === "custom") return;
      updateSidebarDraft({ sort: { key: "custom", direction: baseSort.direction } });
    },
    [store, setSidebarTaskOrder, updateSidebarDraft],
  );

  const handleReorderSubtasks = useCallback(
    (parentTaskId: string, orderedSubtaskIds: string[]) => {
      setSubtaskOrder(parentTaskId, orderedSubtaskIds);
    },
    [setSubtaskOrder],
  );

  return {
    pinnedTaskIds,
    orderedTaskIds,
    subtaskOrderByParentId,
    togglePinnedTask,
    handleReorderGroup,
    handleReorderSubtasks,
  };
}
