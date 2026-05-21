import {
  pruneSubtaskOrder,
  setStoredOrderedTaskIds,
  setStoredPinnedTaskIds,
  setStoredSubtaskOrderByParentId,
} from "@/lib/local-storage";
import type { UISlice } from "./types";

type ImmerSet = (recipe: (draft: UISlice) => void, shouldReplace?: false | undefined) => void;

export function buildSidebarTaskPrefsActions(set: ImmerSet) {
  return {
    togglePinnedTask: (taskId: string) =>
      set((draft) => {
        const list = draft.sidebarTaskPrefs.pinnedTaskIds;
        const idx = list.indexOf(taskId);
        if (idx === -1) list.push(taskId);
        else list.splice(idx, 1);
        setStoredPinnedTaskIds(list);
      }),
    setSidebarTaskOrder: (orderedTaskIds: string[]) =>
      set((draft) => {
        draft.sidebarTaskPrefs.orderedTaskIds = orderedTaskIds;
        setStoredOrderedTaskIds(orderedTaskIds);
      }),
    setSubtaskOrder: (parentTaskId: string, orderedSubtaskIds: string[]) =>
      set((draft) => {
        const map = draft.sidebarTaskPrefs.subtaskOrderByParentId;
        if (orderedSubtaskIds.length === 0) delete map[parentTaskId];
        else map[parentTaskId] = orderedSubtaskIds;
        setStoredSubtaskOrderByParentId(map);
      }),
    removeTaskFromSidebarPrefs: (taskId: string) =>
      set((draft) => {
        const prefs = draft.sidebarTaskPrefs;
        const pinIdx = prefs.pinnedTaskIds.indexOf(taskId);
        if (pinIdx !== -1) {
          prefs.pinnedTaskIds.splice(pinIdx, 1);
          setStoredPinnedTaskIds(prefs.pinnedTaskIds);
        }
        const orderIdx = prefs.orderedTaskIds.indexOf(taskId);
        if (orderIdx !== -1) {
          prefs.orderedTaskIds.splice(orderIdx, 1);
          setStoredOrderedTaskIds(prefs.orderedTaskIds);
        }
        if (pruneSubtaskOrder(prefs.subtaskOrderByParentId, taskId)) {
          setStoredSubtaskOrderByParentId(prefs.subtaskOrderByParentId);
        }
      }),
  };
}
