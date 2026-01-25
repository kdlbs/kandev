import { StateHydrator } from '@/components/state-hydrator';
import { readLayoutDefaults } from '@/lib/layout/read-layout-defaults';
import {
  fetchBoardSnapshot,
  fetchTaskSession,
  fetchTask,
  listAgents,
  listAvailableAgents,
  listRepositories,
  listTaskSessionMessages,
  listTaskSessions,
} from '@/lib/api';
import type { ListMessagesResponse, Task } from '@/lib/types/http';
import { snapshotToState, taskToState } from '@/lib/ssr/mapper';
import { TaskPageContent } from '@/components/task/task-page-content';

export default async function SessionPage({
  params,
}: {
  params: Promise<{ sessionId: string }>;
}) {
  let initialState: ReturnType<typeof taskToState> | null = null;
  let task: Task | null = null;
  let sessionId: string | null = null;
  const defaultLayouts = await readLayoutDefaults();

  try {
    const { sessionId: paramSessionId } = await params;
    sessionId = paramSessionId;

    const sessionResponse = await fetchTaskSession(sessionId, { cache: 'no-store' });
    const session = sessionResponse.session;
    if (!session?.task_id) {
      throw new Error('No task_id found for session');
    }

    task = await fetchTask(session.task_id, { cache: 'no-store' });
    const [snapshot, agents, repositories, allSessionsResponse, availableAgentsResponse] = await Promise.all([
      fetchBoardSnapshot(task.board_id, { cache: 'no-store' }),
      listAgents({ cache: 'no-store' }),
      listRepositories(task.workspace_id, { cache: 'no-store' }),
      listTaskSessions(session.task_id, { cache: 'no-store' }),
      listAvailableAgents({ cache: 'no-store' }).catch(() => ({ agents: [] })),
    ]);
    const allSessions = allSessionsResponse.sessions ?? [session];
    const availableAgents = availableAgentsResponse.agents ?? [];
    let messagesResponse: ListMessagesResponse | null = null;
    try {
      // Load most recent messages in descending order, then reverse to show oldest-to-newest
      messagesResponse = await listTaskSessionMessages(
        sessionId,
        { limit: 50, sort: 'desc' },
        { cache: 'no-store' }
      );
    } catch (error) {
      console.warn(
        'Could not load session messages for SSR:',
        error instanceof Error ? error.message : String(error)
      );
    }

    const messages = messagesResponse?.messages ? [...messagesResponse.messages].reverse() : [];
    const taskState = taskToState(task, sessionId, {
      items: messages,
      hasMore: messagesResponse?.has_more ?? false,
      // oldestCursor should be the first (oldest) message ID for lazy loading older messages
      oldestCursor: messages[0]?.id ?? null,
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
      availableAgents: {
        items: availableAgents,
        loaded: true,
        loading: false,
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
      <TaskPageContent
        task={task}
        sessionId={sessionId}
        initialRepositories={initialState?.repositories?.itemsByWorkspaceId?.[task?.workspace_id ?? ''] ?? []}
        defaultLayouts={defaultLayouts}
      />
    </>
  );
}
