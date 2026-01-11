import type { AppState } from '@/lib/state/store';
import type { BoardSnapshot, Task } from '@/lib/types/http';

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
      };
    })
    .filter(
      (
        task
      ): task is {
        id: string;
        columnId: string;
        title: string;
        description?: string;
        position: number;
        state?: Task['state'];
      } => Boolean(task)
    );

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

export function taskToState(task: Task): Partial<AppState> {
  return {
    tasks: {
      activeTaskId: task.id,
    },
  };
}
