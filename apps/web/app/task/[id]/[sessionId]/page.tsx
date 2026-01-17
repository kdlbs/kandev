import { StateHydrator } from '@/components/state-hydrator';
import {
  fetchBoardSnapshot,
  fetchTask,
  listAgents,
  listRepositories,
  listTaskSessionMessages,
  listTaskSessions,
} from '@/lib/http';
import type { Task } from '@/lib/types/http';
import { snapshotToState, taskToState } from '@/lib/ssr/mapper';
import TaskPageClient from '../page-client';

export default async function TaskSessionPage({
  params,
}: {
  params: Promise<{ id: string; sessionId: string }>;
}) {
  let initialState: ReturnType<typeof taskToState> | null = null;
  let task: Task | null = null;
  let sessionId: string | null = null;
  let sessionsByTask: Record<string, Awaited<ReturnType<typeof listTaskSessions>>['sessions']> = {};

  try {
    const { id, sessionId: paramSessionId } = await params;
    sessionId = paramSessionId;
    task = await fetchTask(id, { cache: 'no-store' });

    const [snapshot, agents, repositories] = await Promise.all([
      fetchBoardSnapshot(task.board_id, { cache: 'no-store' }),
      listAgents({ cache: 'no-store' }),
      listRepositories(task.workspace_id, { cache: 'no-store' }),
    ]);

    const sessionResults = await Promise.all(
      snapshot.tasks.map((taskItem) =>
        listTaskSessions(taskItem.id, { cache: 'no-store' }).then((result) => [taskItem.id, result.sessions] as const)
      )
    );
    sessionsByTask = Object.fromEntries(sessionResults);

    let taskState = taskToState(task, sessionId);
    try {
      const messagesResponse = await listTaskSessionMessages(
        sessionId,
        { limit: 50, sort: 'asc' },
        { cache: 'no-store' }
      );
      taskState = taskToState(task, sessionId, {
        items: messagesResponse.messages ?? [],
        hasMore: messagesResponse.has_more ?? false,
        oldestCursor: messagesResponse.cursor ?? (messagesResponse.messages?.[0]?.id ?? null),
      });
    } catch (error) {
      console.warn(
        'Could not SSR messages (client will load via WebSocket):',
        error instanceof Error ? error.message : String(error)
      );
    }

    initialState = {
      ...snapshotToState(snapshot),
      ...taskState,
      repositories: {
        itemsByWorkspaceId: {
          [task.workspace_id]: repositories.repositories,
        },
        loadingByWorkspaceId: {
          [task.workspace_id]: false,
        },
        loadedByWorkspaceId: {
          [task.workspace_id]: true,
        },
      },
      agentProfiles: {
        items: agents.agents.flatMap((agent) =>
          agent.profiles.map((profile) => ({
            id: profile.id,
            label: `${agent.name} • ${profile.name}`,
            agent_id: agent.id,
          }))
        ),
        version: 0,
      },
      settingsAgents: {
        items: agents.agents,
      },
      settingsData: {
        agentsLoaded: true,
        executorsLoaded: false,
        environmentsLoaded: false,
      },
    };
  } catch {
    initialState = null;
    task = null;
    sessionId = null;
    sessionsByTask = {};
  }

  return (
    <>
      {initialState ? <StateHydrator initialState={initialState} /> : null}
      <TaskPageClient
        task={task}
        sessionId={sessionId}
        initialSessionsByTask={sessionsByTask}
        initialRepositories={initialState?.repositories?.itemsByWorkspaceId?.[task?.workspace_id ?? ''] ?? []}
        initialAgentProfiles={initialState?.agentProfiles?.items ?? []}
      />
    </>
  );
}
