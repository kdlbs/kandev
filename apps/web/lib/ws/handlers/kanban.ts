import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';

export function registerKanbanHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'kanban.update': (message) => {
      store.setState((state) => ({
        ...state,
        kanban: {
          boardId: message.payload.boardId,
          steps: message.payload.steps.map((step, index) => ({
            id: step.id,
            title: step.title,
            color: step.color ?? 'bg-neutral-400',
            position: step.position ?? index,
            autoStartAgent: step.autoStartAgent ?? false,
          })),
          tasks: message.payload.tasks.map((task) => ({
            id: task.id,
            workflowStepId: task.workflowStepId,
            title: task.title,
            description: task.description,
            position: task.position ?? 0,
            state: task.state,
          })),
        },
      }));
    },
  };
}
