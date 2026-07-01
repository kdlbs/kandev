import type { StateCreator } from "zustand";
import type { KanbanSlice, KanbanSliceState } from "./types";

export const defaultKanbanState: KanbanSliceState = {
  workflows: { activeId: null },
  tasks: {
    activeTaskId: null,
    activeSessionId: null,
    pinnedSessionId: null,
    lastSessionByTaskId: {},
  },
};

export const createKanbanSlice: StateCreator<
  KanbanSlice,
  [["zustand/immer", never]],
  [],
  KanbanSlice
> = (set) => ({
  ...defaultKanbanState,
  setActiveWorkflow: (workflowId) =>
    set((draft) => {
      if (draft.workflows.activeId === workflowId) return;
      draft.workflows.activeId = workflowId;
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
      draft.tasks.activeTaskId = taskId;
      draft.tasks.activeSessionId = sessionId;
      draft.tasks.lastSessionByTaskId[taskId] = sessionId;
    }),
  clearActiveSession: () =>
    set((draft) => {
      draft.tasks.activeSessionId = null;
      draft.tasks.pinnedSessionId = null;
    }),
});
