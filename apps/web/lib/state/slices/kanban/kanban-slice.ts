import type { StateCreator } from 'zustand';
import type { KanbanSlice, KanbanSliceState } from './types';

export const defaultKanbanState: KanbanSliceState = {
  kanban: { boardId: null, columns: [], tasks: [] },
  boards: { items: [], activeId: null },
  tasks: { activeTaskId: null, activeSessionId: null },
};

export const createKanbanSlice: StateCreator<
  KanbanSlice,
  [['zustand/immer', never]],
  [],
  KanbanSlice
> = (set, get) => ({
  ...defaultKanbanState,
  setActiveBoard: (boardId) => {
    if (get().boards.activeId === boardId) {
      return;
    }
    set((draft) => {
      draft.boards.activeId = boardId;
    });
  },
  setBoards: (boards) =>
    set((draft) => {
      draft.boards.items = boards;
      if (!draft.boards.activeId && boards.length) {
        draft.boards.activeId = boards[0].id;
      }
    }),
  setActiveTask: (taskId) =>
    set((draft) => {
      draft.tasks.activeTaskId = taskId;
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
});
