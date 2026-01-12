import type { ReactNode } from 'react';
import type { TaskState as TaskStatus, Comment } from '@/lib/types/http';
import { createStore } from 'zustand/vanilla';
import { immer } from 'zustand/middleware/immer';

export type KanbanState = {
  boardId: string | null;
  columns: Array<{ id: string; title: string; color: string; position: number }>;
  tasks: Array<{
    id: string;
    columnId: string;
    title: string;
    description?: string;
    position: number;
    state?: TaskStatus;
  }>;
};

export type WorkspaceState = {
  items: Array<{ id: string; name: string }>;
  activeId: string | null;
};

export type BoardState = {
  items: Array<{ id: string; workspaceId: string; name: string }>;
  activeId: string | null;
};

export type TaskState = {
  activeTaskId: string | null;
};

export type AgentState = {
  agents: Array<{ id: string; status: 'idle' | 'running' | 'error' }>;
};

export type TerminalState = {
  terminals: Array<{ id: string; output: string[] }>;
};

export type DiffState = {
  files: Array<{ path: string; status: 'A' | 'M' | 'D'; plus: number; minus: number }>;
};

export type ConnectionState = {
  status: 'disconnected' | 'connecting' | 'connected' | 'error' | 'reconnecting';
  error: string | null;
};

export type CommentsState = {
  taskId: string | null;
  items: Comment[];
  isLoading: boolean;
};

export type AppState = {
  kanban: KanbanState;
  workspaces: WorkspaceState;
  boards: BoardState;
  tasks: TaskState;
  agents: AgentState;
  terminal: TerminalState;
  diffs: DiffState;
  connection: ConnectionState;
  comments: CommentsState;
  hydrate: (state: Partial<AppState>) => void;
  setActiveWorkspace: (workspaceId: string | null) => void;
  setWorkspaces: (workspaces: WorkspaceState['items']) => void;
  setActiveBoard: (boardId: string | null) => void;
  setBoards: (boards: BoardState['items']) => void;
  setTerminalOutput: (terminalId: string, data: string) => void;
  setConnectionStatus: (status: ConnectionState['status'], error?: string | null) => void;
  setComments: (taskId: string, comments: Comment[]) => void;
  addComment: (comment: Comment) => void;
  setCommentsLoading: (loading: boolean) => void;
};

export type AppStore = ReturnType<typeof createAppStore>;

const defaultState: AppState = {
  kanban: { boardId: null, columns: [], tasks: [] },
  workspaces: { items: [], activeId: null },
  boards: { items: [], activeId: null },
  tasks: { activeTaskId: null },
  agents: { agents: [] },
  terminal: { terminals: [] },
  diffs: { files: [] },
  connection: { status: 'disconnected', error: null },
  comments: { taskId: null, items: [], isLoading: false },
  hydrate: () => undefined,
  setActiveWorkspace: () => undefined,
  setWorkspaces: () => undefined,
  setActiveBoard: () => undefined,
  setBoards: () => undefined,
  setTerminalOutput: () => undefined,
  setConnectionStatus: () => undefined,
  setComments: () => undefined,
  addComment: () => undefined,
  setCommentsLoading: () => undefined,
};

function mergeInitialState(initialState?: Partial<AppState>): Omit<AppState, 'hydrate' | 'setActiveWorkspace' | 'setWorkspaces' | 'setActiveBoard' | 'setBoards' | 'setTerminalOutput' | 'setConnectionStatus' | 'setComments' | 'addComment' | 'setCommentsLoading'> {
  if (!initialState) return defaultState;
  return {
    workspaces: { ...defaultState.workspaces, ...initialState.workspaces },
    boards: { ...defaultState.boards, ...initialState.boards },
    kanban: { ...defaultState.kanban, ...initialState.kanban },
    tasks: { ...defaultState.tasks, ...initialState.tasks },
    agents: { ...defaultState.agents, ...initialState.agents },
    terminal: { ...defaultState.terminal, ...initialState.terminal },
    diffs: { ...defaultState.diffs, ...initialState.diffs },
    connection: { ...defaultState.connection, ...initialState.connection },
    comments: { ...defaultState.comments, ...initialState.comments },
  };
}

export function createAppStore(initialState?: Partial<AppState>) {
  const merged = mergeInitialState(initialState);
  return createStore<AppState>()(
    immer((set) => ({
      ...merged,
      hydrate: (state) =>
        set((draft) => {
          // Deep merge to avoid overwriting nested objects with undefined
          if (state.workspaces) Object.assign(draft.workspaces, state.workspaces);
          if (state.boards) Object.assign(draft.boards, state.boards);
          if (state.kanban) Object.assign(draft.kanban, state.kanban);
          if (state.tasks) Object.assign(draft.tasks, state.tasks);
          if (state.agents) Object.assign(draft.agents, state.agents);
          if (state.terminal) Object.assign(draft.terminal, state.terminal);
          if (state.diffs) Object.assign(draft.diffs, state.diffs);
          if (state.connection) Object.assign(draft.connection, state.connection);
          if (state.comments) Object.assign(draft.comments, state.comments);
        }),
      setActiveWorkspace: (workspaceId) =>
        set((draft) => {
          draft.workspaces.activeId = workspaceId;
        }),
      setWorkspaces: (workspaces) =>
        set((draft) => {
          draft.workspaces.items = workspaces;
          if (!draft.workspaces.activeId && workspaces.length) {
            draft.workspaces.activeId = workspaces[0].id;
          }
        }),
      setBoards: (boards) =>
        set((draft) => {
          draft.boards.items = boards;
          if (!draft.boards.activeId && boards.length) {
            draft.boards.activeId = boards[0].id;
          }
        }),
      setActiveBoard: (boardId) =>
        set((draft) => {
          draft.boards.activeId = boardId;
        }),
      setTerminalOutput: (terminalId, data) =>
        set((draft) => {
          const existing = draft.terminal.terminals.find((terminal) => terminal.id === terminalId);
          if (existing) {
            existing.output.push(data);
          } else {
            draft.terminal.terminals.push({ id: terminalId, output: [data] });
          }
        }),
      setConnectionStatus: (status, error = null) =>
        set((draft) => {
          draft.connection.status = status;
          draft.connection.error = error;
        }),
      setComments: (taskId, comments) =>
        set((draft) => {
          draft.comments.taskId = taskId;
          draft.comments.items = comments;
          draft.comments.isLoading = false;
        }),
      addComment: (comment) =>
        set((draft) => {
          // Only add if this comment is for the current task
          if (draft.comments.taskId === comment.task_id) {
            // Check if comment already exists (avoid duplicates)
            const exists = draft.comments.items.some((c) => c.id === comment.id);
            if (!exists) {
              draft.comments.items.push(comment);
            }
          }
        }),
      setCommentsLoading: (loading) =>
        set((draft) => {
          draft.comments.isLoading = loading;
        }),
    }))
  );
}

export type StoreProviderProps = {
  children: ReactNode;
  initialState?: Partial<AppState>;
};
