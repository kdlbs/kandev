import type { TaskState as TaskStatus } from '@/lib/types/http';

export type KanbanState = {
  boardId: string | null;
  steps: Array<{ id: string; title: string; color: string; position: number; autoStartAgent?: boolean }>;
  tasks: Array<{
    id: string;
    workflowStepId: string;
    title: string;
    description?: string;
    position: number;
    state?: TaskStatus;
    repositoryId?: string;
    primarySessionId?: string | null;
  }>;
};

export type BoardState = {
  items: Array<{ id: string; workspaceId: string; name: string }>;
  activeId: string | null;
};

export type TaskState = {
  activeTaskId: string | null;
  activeSessionId: string | null;
};

export type KanbanSliceState = {
  kanban: KanbanState;
  boards: BoardState;
  tasks: TaskState;
};

export type KanbanSliceActions = {
  setActiveBoard: (boardId: string | null) => void;
  setBoards: (boards: BoardState['items']) => void;
  setActiveTask: (taskId: string) => void;
  setActiveSession: (taskId: string, sessionId: string) => void;
  clearActiveSession: () => void;
};

export type KanbanSlice = KanbanSliceState & KanbanSliceActions;
