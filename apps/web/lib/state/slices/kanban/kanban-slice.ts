import type { StateCreator } from "zustand";
import type { KanbanSlice, KanbanSliceState } from "./types";

export const defaultKanbanState: KanbanSliceState = {
  kanban: { workflowId: null, steps: [], tasks: [] },
  kanbanMulti: { snapshots: {}, isLoading: false },
  workflows: { items: [], activeId: null },
  tasks: { activeTaskId: null, activeSessionId: null },
};

export const createKanbanSlice: StateCreator<
  KanbanSlice,
  [["zustand/immer", never]],
  [],
  KanbanSlice
> = (set, get) => ({
  ...defaultKanbanState,
  setActiveWorkflow: (workflowId) => {
    if (get().workflows.activeId === workflowId) {
      return;
    }
    set((draft) => {
      draft.workflows.activeId = workflowId;
    });
  },
  setWorkflows: (workflows) =>
    set((draft) => {
      draft.workflows.items = workflows;
      if (!draft.workflows.activeId && workflows.length) {
        draft.workflows.activeId = workflows[0].id;
      }
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
    }),
  setActiveSession: (taskId, sessionId) =>
    set((draft) => {
      draft.tasks.activeTaskId = taskId;
      draft.tasks.activeSessionId = sessionId;
    }),
  clearActiveSession: () =>
    set((draft) => {
      draft.tasks.activeSessionId = null;
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
});
