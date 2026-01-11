import type { ReactNode } from 'react';
import { createStore } from 'zustand/vanilla';
import { immer } from 'zustand/middleware/immer';

export type KanbanState = {
  boardId: string | null;
  columns: Array<{ id: string; title: string }>;
  tasks: Array<{ id: string; columnId: string; title: string }>;
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

export type AppState = {
  kanban: KanbanState;
  tasks: TaskState;
  agents: AgentState;
  terminal: TerminalState;
  diffs: DiffState;
  hydrate: (state: Partial<AppState>) => void;
  setTerminalOutput: (terminalId: string, data: string) => void;
};

export type AppStore = ReturnType<typeof createAppStore>;

const defaultState: AppState = {
  kanban: { boardId: null, columns: [], tasks: [] },
  tasks: { activeTaskId: null },
  agents: { agents: [] },
  terminal: { terminals: [] },
  diffs: { files: [] },
  hydrate: () => undefined,
  setTerminalOutput: () => undefined,
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
      setTerminalOutput: (terminalId, data) =>
        set((draft) => {
          const existing = draft.terminal.terminals.find((terminal) => terminal.id === terminalId);
          if (existing) {
            existing.output.push(data);
          } else {
            draft.terminal.terminals.push({ id: terminalId, output: [data] });
          }
        }),
    }))
  );
}

export type StoreProviderProps = {
  children: ReactNode;
  initialState?: Partial<AppState>;
};
