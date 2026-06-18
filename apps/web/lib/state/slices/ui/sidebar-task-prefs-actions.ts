import {
  pruneSubtaskOrder,
  setStoredOrderedTaskIds,
  setStoredPinnedTaskIds,
  setStoredSubtaskOrderByParentId,
} from "@/lib/local-storage";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import type { UISlice } from "./types";

type ImmerSet = (recipe: (draft: UISlice) => void, shouldReplace?: false | undefined) => void;

function syncSidebarTaskPrefs(prefs: UISlice["sidebarTaskPrefs"]) {
  updateUserSettings({
    sidebar_task_prefs: {
      pinned_task_ids: prefs.pinnedTaskIds,
      ordered_task_ids: prefs.orderedTaskIds,
      subtask_order_by_parent_id: prefs.subtaskOrderByParentId,
    },
  }).catch(() => {});
}

export function buildSidebarTaskPrefsActions(set: ImmerSet, get: () => UISlice) {
  return {
    togglePinnedTask: (taskId: string) => {
      set((draft) => {
        const list = draft.sidebarTaskPrefs.pinnedTaskIds;
        const idx = list.indexOf(taskId);
        if (idx === -1) list.push(taskId);
        else list.splice(idx, 1);
        setStoredPinnedTaskIds(list);
      });
      syncSidebarTaskPrefs(get().sidebarTaskPrefs);
    },
    setSidebarTaskOrder: (orderedTaskIds: string[]) => {
      set((draft) => {
        draft.sidebarTaskPrefs.orderedTaskIds = orderedTaskIds;
        setStoredOrderedTaskIds(orderedTaskIds);
      });
      syncSidebarTaskPrefs(get().sidebarTaskPrefs);
    },
    setSubtaskOrder: (parentTaskId: string, orderedSubtaskIds: string[]) => {
      set((draft) => {
        const map = draft.sidebarTaskPrefs.subtaskOrderByParentId;
        if (orderedSubtaskIds.length === 0) delete map[parentTaskId];
        else map[parentTaskId] = orderedSubtaskIds;
        setStoredSubtaskOrderByParentId(map);
      });
      syncSidebarTaskPrefs(get().sidebarTaskPrefs);
    },
    removeTaskFromSidebarPrefs: (taskId: string) => {
      let changed = false;
      set((draft) => {
        const prefs = draft.sidebarTaskPrefs;
        const pinIdx = prefs.pinnedTaskIds.indexOf(taskId);
        if (pinIdx !== -1) {
          changed = true;
          prefs.pinnedTaskIds.splice(pinIdx, 1);
          setStoredPinnedTaskIds(prefs.pinnedTaskIds);
        }
        const orderIdx = prefs.orderedTaskIds.indexOf(taskId);
        if (orderIdx !== -1) {
          changed = true;
          prefs.orderedTaskIds.splice(orderIdx, 1);
          setStoredOrderedTaskIds(prefs.orderedTaskIds);
        }
        if (pruneSubtaskOrder(prefs.subtaskOrderByParentId, taskId)) {
          changed = true;
          setStoredSubtaskOrderByParentId(prefs.subtaskOrderByParentId);
        }
      });
      if (changed) syncSidebarTaskPrefs(get().sidebarTaskPrefs);
    },
  };
}
