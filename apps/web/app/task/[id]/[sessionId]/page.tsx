import { StateHydrator } from '@/components/state-hydrator';
import { fetchTask, listTaskSessionMessages } from '@/lib/http';
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

    // Load messages for SSR
    try {
      const messagesResponse = await listTaskSessionMessages(sessionId, { limit: 50, sort: 'asc' }, { cache: 'no-store' });
      initialState = taskToState(task, {
        items: messagesResponse.messages ?? [],
        hasMore: messagesResponse.has_more ?? false,
        oldestCursor: messagesResponse.cursor ?? (messagesResponse.messages?.[0]?.id ?? null),
      });
    } catch (error) {
      // SSR failed - client will load messages via WebSocket
      console.warn('Could not SSR messages (client will load via WebSocket):', error instanceof Error ? error.message : String(error));
      initialState = taskToState(task);
    }
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
