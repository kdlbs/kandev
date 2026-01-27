import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';

export function registerColumnsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'column.created': (message) => {
      store.setState((state) => {
        if (state.kanban.boardId !== message.payload.board_id) {
          return state;
        }
        const nextSteps = [
          ...state.kanban.steps,
          {
            id: message.payload.id,
            title: message.payload.name,
            color: message.payload.color,
            position: message.payload.position,
            autoStartAgent: message.payload.auto_start_agent ?? false,
          },
        ].sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
        return {
          ...state,
          kanban: {
            ...state.kanban,
            steps: nextSteps,
          },
        };
      });
    },
    'column.updated': (message) => {
      store.setState((state) => {
        if (state.kanban.boardId !== message.payload.board_id) {
          return state;
        }
        const exists = state.kanban.steps.some((step) => step.id === message.payload.id);
        const updatedSteps = exists
          ? state.kanban.steps.map((step) =>
              step.id === message.payload.id
                ? {
                    ...step,
                    title: message.payload.name,
                    color: message.payload.color,
                    position: message.payload.position,
                    autoStartAgent: message.payload.auto_start_agent ?? step.autoStartAgent,
                  }
                : step
            )
          : [
              ...state.kanban.steps,
              {
                id: message.payload.id,
                title: message.payload.name,
                color: message.payload.color,
                position: message.payload.position,
                autoStartAgent: message.payload.auto_start_agent ?? false,
              },
            ];
        const nextSteps = updatedSteps.sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
        return {
          ...state,
          kanban: {
            ...state.kanban,
            steps: nextSteps,
          },
        };
      });
    },
    'column.deleted': (message) => {
      store.setState((state) => {
        if (state.kanban.boardId !== message.payload.board_id) {
          return state;
        }
        const nextSteps = state.kanban.steps.filter((step) => step.id !== message.payload.id);
        return {
          ...state,
          kanban: {
            ...state.kanban,
            steps: nextSteps,
          },
        };
      });
    },
  };
}
