import { StateHydrator } from '@/components/state-hydrator';
import { fetchTask } from '@/lib/ssr/http';
import { taskToState } from '@/lib/ssr/mapper';
import TaskPageClient from './page-client';

export default async function TaskPage({ params }: { params: Promise<{ id: string }> }) {
  try {
    const { id } = await params;
    const task = await fetchTask(id);
    const initialState = taskToState(task);
    return (
      <>
        <StateHydrator initialState={initialState} />
        <TaskPageClient />
      </>
    );
  } catch {
    return <TaskPageClient />;
  }
}
