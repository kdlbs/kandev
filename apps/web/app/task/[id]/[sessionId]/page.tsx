import { StateHydrator } from '@/components/state-hydrator';
import { fetchTask } from '@/lib/http';
import type { Task } from '@/lib/types/http';
import { taskToState } from '@/lib/ssr/mapper';
import TaskPageClient from '../page-client';

export default async function TaskSessionPage({
  params,
}: {
  params: Promise<{ id: string; sessionId: string }>;
}) {
  let initialState: ReturnType<typeof taskToState> | null = null;
  let task: Task | null = null;
  let sessionId: string | null = null;

  try {
    const { id, sessionId: paramSessionId } = await params;
    sessionId = paramSessionId;
    task = await fetchTask(id, { cache: 'no-store' });
    initialState = taskToState(task);
  } catch {
    initialState = null;
    task = null;
    sessionId = null;
  }

  return (
    <>
      {initialState ? <StateHydrator initialState={initialState} /> : null}
      <TaskPageClient task={task} sessionId={sessionId} />
    </>
  );
}
