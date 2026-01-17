import { StateHydrator } from '@/components/state-hydrator';
import {
  fetchBoardSnapshot,
  fetchTaskSession,
  fetchTask,
  listAgents,
  listRepositories,
  listTaskSessionMessages,
} from '@/lib/http';
import type { Task } from '@/lib/types/http';
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
    const [snapshot, agents, repositories] = await Promise.all([
      fetchBoardSnapshot(task.board_id, { cache: 'no-store' }),
      listAgents({ cache: 'no-store' }),
      listRepositories(task.workspace_id, { cache: 'no-store' }),
    ]);
    let messagesResponse = { messages: [], has_more: false, cursor: null };
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

    const taskState = taskToState(task, sessionId, {
      items: messagesResponse.messages ?? [],
      hasMore: messagesResponse.has_more ?? false,
      oldestCursor: messagesResponse.cursor ?? (messagesResponse.messages?.[0]?.id ?? null),
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
            label: `${agent.name} • ${profile.name}`,
            agent_id: agent.id,
          }))
        ),
        version: 0,
      },
      taskSessions: {
        items: {
          [session.id]: session,
        },
      },
      taskSessionsByTask: {
        itemsByTaskId: {
          [task.id]: [session],
        },
        loadingByTaskId: {
          [task.id]: false,
        },
        loadedByTaskId: {
          [task.id]: true,
        },
      },
      worktrees: session.worktree_id
        ? {
            items: {
              [session.worktree_id]: {
                id: session.worktree_id,
                sessionId: session.id,
                repositoryId: session.repository_id ?? undefined,
                path: session.worktree_path ?? undefined,
                branch: session.worktree_branch ?? undefined,
              },
            },
          }
        : undefined,
      sessionWorktreesBySessionId: session.worktree_id
        ? {
            itemsBySessionId: {
              [session.id]: [session.worktree_id],
            },
          }
        : undefined,
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
