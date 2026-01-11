import { StateHydrator } from '@/components/state-hydrator';
import { fetchTask } from '@/lib/ssr/http';
import type { Task } from '@/lib/types/http';
import { taskToState } from '@/lib/ssr/mapper';
import TaskPageClient from './page-client';

export default async function TaskPage({ params }: { params: Promise<{ id: string }> }) {
  let initialState: ReturnType<typeof taskToState> | null = null;
  let task: Task | null = null;

  try {
    const { id } = await params;
    task = await fetchTask(id);
    initialState = taskToState(task);
  } catch {
    initialState = null;
    task = null;
  }

  return (
    <>
      {initialState ? <StateHydrator initialState={initialState} /> : null}
      <TaskPageClient task={task} />
    </>
  );
}
