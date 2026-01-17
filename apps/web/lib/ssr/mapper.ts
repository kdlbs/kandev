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
        repositoryId: task.repositories?.[0]?.repository_id ?? undefined,
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
  sessionId?: string | null,
  messages?: { items: Message[]; hasMore?: boolean; oldestCursor?: string | null }
): Partial<AppState> {
  const resolvedSessionId =
    sessionId ?? messages?.items[0]?.session_id ?? null;
  return {
    tasks: {
      activeTaskId: task.id,
      activeSessionId: resolvedSessionId,
    },
    messages:
      resolvedSessionId && messages
        ? {
            bySession: {
              [resolvedSessionId]: messages.items,
            },
            metaBySession: {
              [resolvedSessionId]: {
                isLoading: false,
                hasMore: messages.hasMore ?? false,
                oldestCursor: messages.oldestCursor ?? (messages.items[0]?.id ?? null),
              },
            },
          }
        : undefined,
  };
}
