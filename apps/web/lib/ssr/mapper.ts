import type { AppState, KanbanState } from '@/lib/state/store';
import type { BoardSnapshot, Message, Task } from '@/lib/types/http';

type KanbanTask = KanbanState['tasks'][number];

export function snapshotToState(snapshot: BoardSnapshot): Partial<AppState> {
  const tasks = snapshot.tasks
    .map((task) => {
      const columnId = task.column_id;
      if (!columnId) return null;
      return {
        id: task.id,
        columnId,
        title: task.title,
        description: task.description ?? undefined,
        position: task.position ?? 0,
        state: task.state,
        repositoryId: task.repository_id ?? undefined,
      } as KanbanTask;
    })
    .filter((task): task is KanbanTask => task !== null);

  return {
    kanban: {
      boardId: snapshot.board.id,
      columns: snapshot.columns.map((column) => ({
        id: column.id,
        title: column.name,
        color: column.color ?? 'bg-neutral-400',
        position: column.position,
      })),
      tasks,
    },
  };
}

export function taskToState(
  task: Task,
  messages?: { items: Message[]; hasMore?: boolean; oldestCursor?: string | null }
): Partial<AppState> {
  return {
    tasks: {
      activeTaskId: task.id,
    },
    messages: messages
      ? {
          sessionId: messages.items[0]?.agent_session_id ?? null,
          items: messages.items,
          isLoading: false,
          hasMore: messages.hasMore ?? false,
          oldestCursor: messages.oldestCursor ?? (messages.items[0]?.id ?? null),
        }
      : undefined,
  };
}
