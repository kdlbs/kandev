import { StateHydrator } from '@/components/state-hydrator';
import {
  fetchBoardSnapshot,
  fetchTaskSession,
  fetchTask,
  listAgents,
  listRepositories,
  listTaskSessionMessages,
  listTaskSessions,
} from '@/lib/http';
import type { ListMessagesResponse, Task } from '@/lib/types/http';
import { snapshotToState, taskToState } from '@/lib/ssr/mapper';
import TaskPageClient from '@/app/task/[id]/page-client';

export default async function SessionPage({
  params,
}: {
  params: Promise<{ sessionId: string }>;
}) {
  let initialState: ReturnType<typeof taskToState> | null = null;
  let task: Task | null = null;
  let sessionId: string | null = null;

  try {
    const { sessionId: paramSessionId } = await params;
    sessionId = paramSessionId;

    const sessionResponse = await fetchTaskSession(sessionId, { cache: 'no-store' });
    const session = sessionResponse.session;
    if (!session?.task_id) {
      throw new Error('No task_id found for session');
    }

    task = await fetchTask(session.task_id, { cache: 'no-store' });
    const [snapshot, agents, repositories, allSessionsResponse] = await Promise.all([
      fetchBoardSnapshot(task.board_id, { cache: 'no-store' }),
      listAgents({ cache: 'no-store' }),
      listRepositories(task.workspace_id, { cache: 'no-store' }),
      listTaskSessions(session.task_id, { cache: 'no-store' }),
    ]);
    const allSessions = allSessionsResponse.sessions ?? [session];
    let messagesResponse: ListMessagesResponse | null = null;
    try {
      messagesResponse = await listTaskSessionMessages(
        sessionId,
        { limit: 50, sort: 'asc' },
        { cache: 'no-store' }
      );
    } catch (error) {
      console.warn(
        'Could not load session messages for SSR:',
        error instanceof Error ? error.message : String(error)
      );
    }

    const messages = messagesResponse?.messages ?? [];
    const taskState = taskToState(task, sessionId, {
      items: messages,
      hasMore: messagesResponse?.has_more ?? false,
      oldestCursor: messagesResponse?.cursor ?? (messages[0]?.id ?? null),
    });

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
            label: `${profile.agent_display_name} • ${profile.name}`,
            agent_id: agent.id,
          }))
        ),
        version: 0,
      },
      taskSessions: {
        items: Object.fromEntries(allSessions.map((s) => [s.id, s])),
      },
      taskSessionsByTask: {
        itemsByTaskId: {
          [task.id]: allSessions,
        },
        loadingByTaskId: {
          [task.id]: false,
        },
        loadedByTaskId: {
          [task.id]: true,
        },
      },
      worktrees: {
        items: Object.fromEntries(
          allSessions
            .filter((s) => s.worktree_id)
            .map((s) => [
              s.worktree_id,
              {
                id: s.worktree_id!,
                sessionId: s.id,
                repositoryId: s.repository_id ?? undefined,
                path: s.worktree_path ?? undefined,
                branch: s.worktree_branch ?? undefined,
              },
            ])
        ),
      },
      sessionWorktreesBySessionId: {
        itemsBySessionId: Object.fromEntries(
          allSessions
            .filter((s) => s.worktree_id)
            .map((s) => [s.id, [s.worktree_id!]])
        ),
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
  } catch (error) {
    console.warn(
      'Could not SSR session (client will load via WebSocket):',
      error instanceof Error ? error.message : String(error)
    );
    initialState = null;
    task = null;
    sessionId = null;
  }

  return (
    <>
      {initialState ? <StateHydrator initialState={initialState} /> : null}
      <TaskPageClient
        task={task}
        sessionId={sessionId}
        initialRepositories={initialState?.repositories?.itemsByWorkspaceId?.[task?.workspace_id ?? ''] ?? []}
      />
    </>
  );
}
