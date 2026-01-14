import { StateHydrator } from '@/components/state-hydrator';
import { fetchTask, listTaskComments } from '@/lib/http';
import type { Task } from '@/lib/types/http';
import { taskToState } from '@/lib/ssr/mapper';
import TaskPageClient from './page-client';

export default async function TaskPage({ params }: { params: Promise<{ id: string }> }) {
  let initialState: ReturnType<typeof taskToState> | null = null;
  let task: Task | null = null;
  let comments: Awaited<ReturnType<typeof listTaskComments>> | null = null;

  try {
    const { id } = await params;
    task = await fetchTask(id, { cache: 'no-store' });
    comments = await listTaskComments(id, { cache: 'no-store' });
    initialState = taskToState(task, comments.comments);
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
