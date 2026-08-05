import type { StateCreator } from "zustand";
import type { KanbanSlice, KanbanSliceState } from "./types";

export const defaultKanbanState: KanbanSliceState = {
  kanban: { workflowId: null, steps: [], tasks: [] },
  kanbanMulti: { snapshots: {}, isLoading: false },
  sidebarArchivedTasks: {
    itemsByWorkspaceId: {},
    loadedByWorkspaceId: {},
    loadingByWorkspaceId: {},
    errorByWorkspaceId: {},
  },
  workflows: { items: [], activeId: null },
  workspaceContextGeneration: 0,
  tasks: {
    activeTaskId: null,
    activeSessionId: null,
    pinnedSessionId: null,
    lastSessionByTaskId: {},
  },
};

/* eslint-disable max-lines-per-function -- the slice keeps related task and archived-cache actions together. */
export const createKanbanSlice: StateCreator<
  KanbanSlice,
  [["zustand/immer", never]],
  [],
  KanbanSlice
> = (set) => ({
  ...defaultKanbanState,
  resetKanbanWorkspaceContext: () =>
    set((draft) => {
      const nextGeneration = draft.workspaceContextGeneration + 1;
      Object.assign(draft, defaultKanbanState);
      draft.workspaceContextGeneration = nextGeneration;
    }),
  setActiveWorkflow: (workflowId) =>
    set((draft) => {
      if (draft.workflows.activeId === workflowId) return;
      draft.workflows.activeId = workflowId;
    }),
  setWorkflows: (workflows) =>
    set((draft) => {
      draft.workflows.items = workflows;
    }),
  reorderWorkflowItems: (workflowIds) =>
    set((draft) => {
      const byId = new Map(draft.workflows.items.map((w) => [w.id, w]));
      const reordered = workflowIds
        .map((id) => byId.get(id))
        .filter((w): w is NonNullable<typeof w> => w != null);
      // Append any items not in the reorder list (shouldn't happen, but defensive)
      for (const item of draft.workflows.items) {
        if (!workflowIds.includes(item.id)) reordered.push(item);
      }
      draft.workflows.items = reordered;
    }),
  setActiveTask: (taskId) =>
    set((draft) => {
      draft.tasks.activeTaskId = taskId;
      draft.tasks.activeSessionId = null;
      // New task → drop any pin; the pin only applies within a single task.
      draft.tasks.pinnedSessionId = null;
    }),
  setActiveSession: (taskId, sessionId) =>
    set((draft) => {
      draft.tasks.activeTaskId = taskId;
      draft.tasks.activeSessionId = sessionId;
      // User-initiated selection: pin so WS auto-replace handoff respects it.
      draft.tasks.pinnedSessionId = sessionId;
      draft.tasks.lastSessionByTaskId[taskId] = sessionId;
    }),
  setActiveSessionAuto: (taskId, sessionId) =>
    set((draft) => {
      if (draft.tasks.activeTaskId !== taskId) {
        draft.tasks.pinnedSessionId = null;
      }
      draft.tasks.activeTaskId = taskId;
      draft.tasks.activeSessionId = sessionId;
      draft.tasks.lastSessionByTaskId[taskId] = sessionId;
    }),
  clearActiveSession: () =>
    set((draft) => {
      draft.tasks.activeSessionId = null;
      draft.tasks.pinnedSessionId = null;
    }),
  setWorkflowSnapshot: (workflowId, data) =>
    set((draft) => {
      draft.kanbanMulti.snapshots[workflowId] = data;
    }),
  setKanbanMultiLoading: (loading) =>
    set((draft) => {
      draft.kanbanMulti.isLoading = loading;
    }),
  clearKanbanMulti: () =>
    set((draft) => {
      draft.kanbanMulti.snapshots = {};
      draft.kanbanMulti.isLoading = false;
    }),
  updateMultiTask: (workflowId, task) =>
    set((draft) => {
      const snapshot = draft.kanbanMulti.snapshots[workflowId];
      if (!snapshot) return;
      const idx = snapshot.tasks.findIndex((t) => t.id === task.id);
      if (idx >= 0) {
        snapshot.tasks[idx] = task;
      } else {
        snapshot.tasks.push(task);
      }
    }),
  removeMultiTask: (workflowId, taskId) =>
    set((draft) => {
      const snapshot = draft.kanbanMulti.snapshots[workflowId];
      if (!snapshot) return;
      snapshot.tasks = snapshot.tasks.filter((t) => t.id !== taskId);
    }),
  setSidebarArchivedTasks: (workspaceId, tasks) =>
    set((draft) => {
      const seen = new Set<string>();
      draft.sidebarArchivedTasks.itemsByWorkspaceId[workspaceId] = tasks.filter((task) => {
        if (seen.has(task.id)) return false;
        seen.add(task.id);
        return true;
      });
      draft.sidebarArchivedTasks.loadedByWorkspaceId[workspaceId] = true;
      draft.sidebarArchivedTasks.errorByWorkspaceId[workspaceId] = null;
    }),
  setSidebarArchivedTasksLoading: (workspaceId, loading) =>
    set((draft) => {
      draft.sidebarArchivedTasks.loadingByWorkspaceId[workspaceId] = loading;
    }),
  setSidebarArchivedTasksError: (workspaceId, error) =>
    set((draft) => {
      draft.sidebarArchivedTasks.errorByWorkspaceId[workspaceId] = error;
    }),
  upsertSidebarArchivedTask: (workspaceId, task) =>
    set((draft) => {
      const items = draft.sidebarArchivedTasks.itemsByWorkspaceId[workspaceId] ?? [];
      const index = items.findIndex((item) => item.id === task.id);
      if (index >= 0) items[index] = task;
      else items.push(task);
    }),
  removeSidebarArchivedTask: (taskId, workspaceId) =>
    set((draft) => {
      const workspaceIds = workspaceId
        ? [workspaceId]
        : Object.keys(draft.sidebarArchivedTasks.itemsByWorkspaceId);
      for (const id of workspaceIds) {
        const items = draft.sidebarArchivedTasks.itemsByWorkspaceId[id];
        if (items) {
          draft.sidebarArchivedTasks.itemsByWorkspaceId[id] = items.filter(
            (task) => task.id !== taskId,
          );
        }
      }
    }),
});
/* eslint-enable max-lines-per-function */
