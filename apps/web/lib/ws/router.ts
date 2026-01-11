import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { BackendMessageMap } from '@/lib/types/backend';

export function registerWsHandlers(store: StoreApi<AppState>) {
  return {
    'kanban.update': (message: BackendMessageMap['kanban.update']) => {
      store.setState((state) => ({
        ...state,
        kanban: {
          boardId: message.payload.boardId,
          columns: message.payload.columns,
          tasks: message.payload.tasks,
        },
      }));
    },
    'task.update': (message: BackendMessageMap['task.update']) => {
      store.setState((state) => ({
        ...state,
        tasks: {
          ...state.tasks,
          activeTaskId: message.payload.taskId,
        },
      }));
    },
    'agent.update': (message: BackendMessageMap['agent.update']) => {
      store.setState((state) => ({
        ...state,
        agents: {
          agents: state.agents.agents.some((agent) => agent.id === message.payload.agentId)
            ? state.agents.agents.map((agent) =>
                agent.id === message.payload.agentId
                  ? { ...agent, status: message.payload.status }
                  : agent
              )
            : [...state.agents.agents, { id: message.payload.agentId, status: message.payload.status }],
        },
      }));
    },
    'terminal.output': (message: BackendMessageMap['terminal.output']) => {
      store.getState().setTerminalOutput(message.payload.terminalId, message.payload.data);
    },
    'diff.update': (message: BackendMessageMap['diff.update']) => {
      store.setState((state) => ({
        ...state,
        diffs: {
          files: message.payload.files,
        },
      }));
    },
    'system.error': () => {
      // TODO: surface as toast/notification once UI is ready.
    },
  };
}
