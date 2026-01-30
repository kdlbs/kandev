import { StateHydrator } from '@/components/state-hydrator';
import { readLayoutDefaults } from '@/lib/layout/read-layout-defaults';
import {
  fetchBoardSnapshot,
  fetchTaskSession,
  fetchTask,
  listAgents,
  listAvailableAgents,
  listBoards,
  listRepositories,
  listTaskSessionMessages,
  listTaskSessions,
  listWorkspaces,
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
    const [snapshot, agents, repositories, allSessionsResponse, availableAgentsResponse, workspacesResponse, boardsResponse] = await Promise.all([
      fetchBoardSnapshot(task.board_id, { cache: 'no-store' }),
      listAgents({ cache: 'no-store' }),
      listRepositories(task.workspace_id, { cache: 'no-store' }),
      listTaskSessions(session.task_id, { cache: 'no-store' }),
      listAvailableAgents({ cache: 'no-store' }).catch(() => ({ agents: [] })),
      listWorkspaces({ cache: 'no-store' }).catch(() => ({ workspaces: [] })),
      listBoards(task.workspace_id, { cache: 'no-store' }).catch(() => ({ boards: [] })),
    ]);
    const allSessions = allSessionsResponse.sessions ?? [session];
    const availableAgents = availableAgentsResponse.agents ?? [];
    const workspaces = workspacesResponse.workspaces ?? [];
    const boards = boardsResponse.boards ?? [];
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
      workspaces: {
        items: workspaces.map((workspace) => ({
          id: workspace.id,
          name: workspace.name,
          description: workspace.description ?? null,
          owner_id: workspace.owner_id,
          default_executor_id: workspace.default_executor_id ?? null,
          default_environment_id: workspace.default_environment_id ?? null,
          default_agent_profile_id: workspace.default_agent_profile_id ?? null,
          created_at: workspace.created_at,
          updated_at: workspace.updated_at,
        })),
        activeId: task.workspace_id,
      },
      boards: {
        items: boards.map((board) => ({
          id: board.id,
          workspaceId: board.workspace_id,
          name: board.name,
        })),
        activeId: task.board_id,
      },
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
      {initialState ? <StateHydrator initialState={initialState} sessionId={sessionId ?? undefined} /> : null}
      <TaskPageContent
        task={task}
        sessionId={sessionId}
        initialRepositories={initialState?.repositories?.itemsByWorkspaceId?.[task?.workspace_id ?? ''] ?? []}
        defaultLayouts={defaultLayouts}
      />
    </>
  );
}
