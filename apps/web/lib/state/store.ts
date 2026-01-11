import type { ReactNode } from 'react';
import { createStore } from 'zustand/vanilla';
import { immer } from 'zustand/middleware/immer';

export type KanbanState = {
  boardId: string | null;
  columns: Array<{ id: string; title: string }>;
  tasks: Array<{ id: string; columnId: string; title: string }>;
};

export type WorkspaceState = {
  items: Array<{ id: string; name: string }>;
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
  status: 'disconnected' | 'connecting' | 'connected' | 'error';
  error: string | null;
};

export type AppState = {
  kanban: KanbanState;
  workspaces: WorkspaceState;
  tasks: TaskState;
  agents: AgentState;
  terminal: TerminalState;
  diffs: DiffState;
  connection: ConnectionState;
  hydrate: (state: Partial<AppState>) => void;
  setActiveWorkspace: (workspaceId: string | null) => void;
  setWorkspaces: (workspaces: WorkspaceState['items']) => void;
  setTerminalOutput: (terminalId: string, data: string) => void;
  setConnectionStatus: (status: ConnectionState['status'], error?: string | null) => void;
};

export type AppStore = ReturnType<typeof createAppStore>;

const defaultState: AppState = {
  kanban: { boardId: null, columns: [], tasks: [] },
  workspaces: { items: [], activeId: null },
  tasks: { activeTaskId: null },
  agents: { agents: [] },
  terminal: { terminals: [] },
  diffs: { files: [] },
  connection: { status: 'disconnected', error: null },
  hydrate: () => undefined,
  setActiveWorkspace: () => undefined,
  setWorkspaces: () => undefined,
  setTerminalOutput: () => undefined,
  setConnectionStatus: () => undefined,
};

export function createAppStore(initialState?: Partial<AppState>) {
  return createStore<AppState>()(
    immer((set) => ({
      ...defaultState,
      ...initialState,
      hydrate: (state) =>
        set((draft) => {
          Object.assign(draft, state);
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
    }))
  );
}

export type StoreProviderProps = {
  children: ReactNode;
  initialState?: Partial<AppState>;
};
