import { updateUserSettings } from "@/lib/api/domains/settings-api";
import type { UISlice } from "./types";

type ImmerSet = (recipe: (draft: UISlice) => void, shouldReplace?: false | undefined) => void;

let sidebarTaskPrefsSync = Promise.resolve();
let sidebarTaskPrefsSyncVersion = 0;

function pruneSubtaskOrder(map: Record<string, string[]>, taskId: string): boolean {
  let changed = false;
  if (taskId in map) {
    delete map[taskId];
    changed = true;
  }
  for (const [parentId, ids] of Object.entries(map)) {
    if (!ids.includes(taskId)) continue;
    const next = ids.filter((id) => id !== taskId);
    if (next.length === 0) delete map[parentId];
    else map[parentId] = next;
    changed = true;
  }
  return changed;
}

function syncSidebarTaskPrefs(prefs: UISlice["sidebarTaskPrefs"], set: ImmerSet) {
  const syncVersion = ++sidebarTaskPrefsSyncVersion;
  const payload = {
    sidebar_task_prefs: {
      pinned_task_ids: [...prefs.pinnedTaskIds],
      ordered_task_ids: [...prefs.orderedTaskIds],
      subtask_order_by_parent_id: Object.fromEntries(
        Object.entries(prefs.subtaskOrderByParentId).map(([key, ids]) => [key, [...ids]]),
      ),
    },
  };
  sidebarTaskPrefsSync = sidebarTaskPrefsSync
    .catch(() => undefined)
    .then(() =>
      updateUserSettings(payload)
        .then(() => {
          set((draft) => {
            if (syncVersion !== sidebarTaskPrefsSyncVersion) return;
            draft.sidebarTaskPrefs.syncError = null;
            draft.sidebarTaskPrefs.syncPending = false;
          });
        })
        .catch((err) => {
          const message = err instanceof Error ? err.message : "Failed to sync sidebar task prefs";
          set((draft) => {
            if (syncVersion !== sidebarTaskPrefsSyncVersion) return;
            draft.sidebarTaskPrefs.syncError = message;
            draft.sidebarTaskPrefs.syncPending = false;
          });
        })
        .then(() => undefined),
    );
}

export function buildSidebarTaskPrefsActions(set: ImmerSet, get: () => UISlice) {
  return {
    clearSidebarTaskPrefsSyncError: () =>
      set((draft) => {
        draft.sidebarTaskPrefs.syncError = null;
      }),
    togglePinnedTask: (taskId: string) => {
      set((draft) => {
        const list = draft.sidebarTaskPrefs.pinnedTaskIds;
        draft.sidebarTaskPrefs.syncPending = true;
        const idx = list.indexOf(taskId);
        if (idx === -1) list.push(taskId);
        else list.splice(idx, 1);
      });
      syncSidebarTaskPrefs(get().sidebarTaskPrefs, set);
    },
    // Pin every id that isn't already pinned (bulk "Pin", not a per-id toggle).
    pinTasks: (taskIds: string[]) => {
      let changed = false;
      set((draft) => {
        const list = draft.sidebarTaskPrefs.pinnedTaskIds;
        for (const id of taskIds) {
          if (!list.includes(id)) {
            list.push(id);
            changed = true;
          }
        }
        if (changed) {
          draft.sidebarTaskPrefs.syncPending = true;
        }
      });
      if (changed) syncSidebarTaskPrefs(get().sidebarTaskPrefs, set);
    },
    // Unpin every matching id in one state update (bulk "Unpin").
    unpinTasks: (taskIds: string[]) => {
      const toRemove = new Set(taskIds);
      let changed = false;
      set((draft) => {
        const list = draft.sidebarTaskPrefs.pinnedTaskIds;
        const next = list.filter((id) => !toRemove.has(id));
        if (next.length === list.length) return;
        changed = true;
        draft.sidebarTaskPrefs.syncPending = true;
        draft.sidebarTaskPrefs.pinnedTaskIds = next;
      });
      if (changed) syncSidebarTaskPrefs(get().sidebarTaskPrefs, set);
    },
    setSidebarTaskOrder: (orderedTaskIds: string[]) => {
      set((draft) => {
        draft.sidebarTaskPrefs.syncPending = true;
        draft.sidebarTaskPrefs.orderedTaskIds = orderedTaskIds;
      });
      syncSidebarTaskPrefs(get().sidebarTaskPrefs, set);
    },
    setSubtaskOrder: (parentTaskId: string, orderedSubtaskIds: string[]) => {
      set((draft) => {
        draft.sidebarTaskPrefs.syncPending = true;
        const map = draft.sidebarTaskPrefs.subtaskOrderByParentId;
        if (orderedSubtaskIds.length === 0) delete map[parentTaskId];
        else map[parentTaskId] = orderedSubtaskIds;
      });
      syncSidebarTaskPrefs(get().sidebarTaskPrefs, set);
    },
    removeTaskFromSidebarPrefs: (taskId: string) => {
      let changed = false;
      set((draft) => {
        const prefs = draft.sidebarTaskPrefs;
        const pinIdx = prefs.pinnedTaskIds.indexOf(taskId);
        if (pinIdx !== -1) {
          changed = true;
          prefs.syncPending = true;
          prefs.pinnedTaskIds.splice(pinIdx, 1);
        }
        const orderIdx = prefs.orderedTaskIds.indexOf(taskId);
        if (orderIdx !== -1) {
          changed = true;
          prefs.syncPending = true;
          prefs.orderedTaskIds.splice(orderIdx, 1);
        }
        if (pruneSubtaskOrder(prefs.subtaskOrderByParentId, taskId)) {
          changed = true;
          prefs.syncPending = true;
        }
      });
      if (changed) syncSidebarTaskPrefs(get().sidebarTaskPrefs, set);
    },
  };
}
