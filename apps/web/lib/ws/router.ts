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
          columns: message.payload.columns.map((column, index) => ({
            id: column.id,
            title: column.title,
            color: column.color ?? 'bg-neutral-400',
            position: column.position ?? index,
          })),
          tasks: message.payload.tasks.map((task) => ({
            id: task.id,
            columnId: task.columnId,
            title: task.title,
            description: task.description,
            position: task.position ?? 0,
            state: task.state,
          })),
        },
      }));
    },
    'task.created': (message: BackendMessageMap['task.created']) => {
      store.setState((state) => {
        if (state.kanban.boardId !== message.payload.board_id) {
          return state;
        }
        const exists = state.kanban.tasks.some((task) => task.id === message.payload.task_id);
        const nextTask = {
          id: message.payload.task_id,
          columnId: message.payload.column_id,
          title: message.payload.title,
          description: message.payload.description,
          position: message.payload.position ?? 0,
          state: message.payload.state,
        };
        return {
          ...state,
          kanban: {
            ...state.kanban,
            tasks: exists
              ? state.kanban.tasks.map((task) => (task.id === nextTask.id ? nextTask : task))
              : [...state.kanban.tasks, nextTask],
          },
        };
      });
    },
    'task.updated': (message: BackendMessageMap['task.updated']) => {
      store.setState((state) => {
        if (state.kanban.boardId !== message.payload.board_id) {
          return state;
        }
        const nextTask = {
          id: message.payload.task_id,
          columnId: message.payload.column_id,
          title: message.payload.title,
          description: message.payload.description,
          position: message.payload.position ?? 0,
          state: message.payload.state,
        };
        return {
          ...state,
          kanban: {
            ...state.kanban,
            tasks: state.kanban.tasks.some((task) => task.id === nextTask.id)
              ? state.kanban.tasks.map((task) => (task.id === nextTask.id ? nextTask : task))
              : [...state.kanban.tasks, nextTask],
          },
        };
      });
    },
    'task.deleted': (message: BackendMessageMap['task.deleted']) => {
      store.setState((state) => ({
        ...state,
        kanban: {
          ...state.kanban,
          tasks: state.kanban.tasks.filter((task) => task.id !== message.payload.task_id),
        },
      }));
    },
    'task.state_changed': (message: BackendMessageMap['task.state_changed']) => {
      store.setState((state) => {
        if (state.kanban.boardId !== message.payload.board_id) {
          return state;
        }
        const nextTask = {
          id: message.payload.task_id,
          columnId: message.payload.column_id,
          title: message.payload.title,
          description: message.payload.description,
          position: message.payload.position ?? 0,
          state: message.payload.state,
        };
        return {
          ...state,
          kanban: {
            ...state.kanban,
            tasks: state.kanban.tasks.some((task) => task.id === nextTask.id)
              ? state.kanban.tasks.map((task) => (task.id === nextTask.id ? nextTask : task))
              : [...state.kanban.tasks, nextTask],
          },
        };
      });
    },
    'agent.updated': (message: BackendMessageMap['agent.updated']) => {
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
    'workspace.created': (message: BackendMessageMap['workspace.created']) => {
      store.setState((state) => {
        const exists = state.workspaces.items.some((item) => item.id === message.payload.id);
        const items = exists
          ? state.workspaces.items.map((item) =>
              item.id === message.payload.id ? { ...item, name: message.payload.name } : item
            )
          : [{ id: message.payload.id, name: message.payload.name }, ...state.workspaces.items];
        const activeId = state.workspaces.activeId ?? message.payload.id;
        return {
          ...state,
          workspaces: {
            items,
            activeId,
          },
        };
      });
    },
    'workspace.updated': (message: BackendMessageMap['workspace.updated']) => {
      store.setState((state) => ({
        ...state,
        workspaces: {
          ...state.workspaces,
          items: state.workspaces.items.map((item) =>
            item.id === message.payload.id ? { ...item, name: message.payload.name } : item
          ),
        },
      }));
    },
    'workspace.deleted': (message: BackendMessageMap['workspace.deleted']) => {
      store.setState((state) => {
        const items = state.workspaces.items.filter((item) => item.id !== message.payload.id);
        const activeId =
          state.workspaces.activeId === message.payload.id ? items[0]?.id ?? null : state.workspaces.activeId;
        const clearBoards = state.workspaces.activeId === message.payload.id;
        return {
          ...state,
          workspaces: {
            items,
            activeId,
          },
          boards: clearBoards ? { items: [], activeId: null } : state.boards,
          kanban: clearBoards ? { boardId: null, columns: [], tasks: [] } : state.kanban,
        };
      });
    },
    'board.created': (message: BackendMessageMap['board.created']) => {
      store.setState((state) => {
        if (state.workspaces.activeId !== message.payload.workspace_id) {
          return state;
        }
        const exists = state.boards.items.some((item) => item.id === message.payload.id);
        if (exists) {
          return state;
        }
        return {
          ...state,
          boards: {
            items: [
              {
                id: message.payload.id,
                workspaceId: message.payload.workspace_id,
                name: message.payload.name,
              },
              ...state.boards.items,
            ],
            activeId: state.boards.activeId ?? message.payload.id,
          },
        };
      });
    },
    'board.updated': (message: BackendMessageMap['board.updated']) => {
      store.setState((state) => ({
        ...state,
        boards: {
          ...state.boards,
          items: state.boards.items.map((item) =>
            item.id === message.payload.id ? { ...item, name: message.payload.name } : item
          ),
        },
      }));
    },
    'board.deleted': (message: BackendMessageMap['board.deleted']) => {
      store.setState((state) => {
        const items = state.boards.items.filter((item) => item.id !== message.payload.id);
        const nextActiveId =
          state.boards.activeId === message.payload.id ? items[0]?.id ?? null : state.boards.activeId;
        return {
          ...state,
          boards: {
            items,
            activeId: nextActiveId,
          },
          kanban:
            state.kanban.boardId === message.payload.id
              ? { boardId: nextActiveId, columns: [], tasks: [] }
              : state.kanban,
        };
      });
    },
    'column.created': (message: BackendMessageMap['column.created']) => {
      store.setState((state) => {
        if (state.kanban.boardId !== message.payload.board_id) {
          return state;
        }
        const nextColumns = [
          ...state.kanban.columns,
          {
            id: message.payload.id,
            title: message.payload.name,
            color: message.payload.color,
            position: message.payload.position,
          },
        ].sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
        return {
          ...state,
          kanban: {
            ...state.kanban,
            columns: nextColumns,
          },
        };
      });
    },
    'column.updated': (message: BackendMessageMap['column.updated']) => {
      store.setState((state) => {
        if (state.kanban.boardId !== message.payload.board_id) {
          return state;
        }
        const exists = state.kanban.columns.some((column) => column.id === message.payload.id);
        const updatedColumns = exists
          ? state.kanban.columns.map((column) =>
              column.id === message.payload.id
                ? {
                    ...column,
                    title: message.payload.name,
                    color: message.payload.color,
                    position: message.payload.position,
                  }
                : column
            )
          : [
              ...state.kanban.columns,
              {
                id: message.payload.id,
                title: message.payload.name,
                color: message.payload.color,
                position: message.payload.position,
              },
            ];
        const nextColumns = updatedColumns.sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
        return {
          ...state,
          kanban: {
            ...state.kanban,
            columns: nextColumns,
          },
        };
      });
    },
    'column.deleted': (message: BackendMessageMap['column.deleted']) => {
      store.setState((state) => {
        if (state.kanban.boardId !== message.payload.board_id) {
          return state;
        }
        const nextColumns = state.kanban.columns.filter((column) => column.id !== message.payload.id);
        return {
          ...state,
          kanban: {
            ...state.kanban,
            columns: nextColumns,
          },
        };
      });
    },
    'system.error': () => {
      // TODO: surface as toast/notification once UI is ready.
    },
  };
}
