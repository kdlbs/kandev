import type { AppState } from '@/lib/state/store';
import type { BoardSnapshot, Task } from '@/lib/types/http';

export function snapshotToState(snapshot: BoardSnapshot): Partial<AppState> {
  const columnByState = new Map(snapshot.columns.map((column) => [column.state, column.id]));
  const tasks = snapshot.tasks
    .map((task) => {
      const columnId = columnByState.get(task.state);
      if (!columnId) return null;
      return { id: task.id, columnId, title: task.title };
    })
    .filter((task): task is { id: string; columnId: string; title: string } => Boolean(task));

  return {
    kanban: {
      boardId: snapshot.board.id,
      columns: snapshot.columns.map((column) => ({ id: column.id, title: column.name })),
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
