import { PageClient } from '@/app/page-client';
import { StateHydrator } from '@/components/state-hydrator';
import { fetchBoardSnapshot, fetchUserSettings, listBoards, listRepositories, listWorkspaces, listTaskSessionMessages } from '@/lib/http';
import { snapshotToState } from '@/lib/ssr/mapper';
import type { AppState } from '@/lib/state/store';

// Server Component: runs on the server for SSR and data hydration.
type PageProps = {
  searchParams?: Promise<Record<string, string | string[] | undefined>>;
};

export default async function Page({ searchParams }: PageProps) {
  try {
    const resolvedParams = searchParams ? await searchParams : {};
    const workspaceParam = resolvedParams.workspaceId;
    const boardParam = resolvedParams.boardId;
    const taskIdParam = resolvedParams.taskId;
    const sessionIdParam = resolvedParams.sessionId;
    const workspaceId = Array.isArray(workspaceParam) ? workspaceParam[0] : workspaceParam;
    const boardIdParam = Array.isArray(boardParam) ? boardParam[0] : boardParam;
    const taskId = Array.isArray(taskIdParam) ? taskIdParam[0] : taskIdParam;
    const sessionId = Array.isArray(sessionIdParam) ? sessionIdParam[0] : sessionIdParam;

    const [workspaces, userSettingsResponse] = await Promise.all([
      listWorkspaces({ cache: 'no-store' }),
      fetchUserSettings({ cache: 'no-store' }).catch(() => null),
    ]);
    const userSettings = userSettingsResponse?.settings;
    const settingsWorkspaceId = userSettings?.workspace_id || null;
    const settingsBoardId = userSettings?.board_id || null;
    const settingsRepositoryIds = Array.from(new Set(userSettings?.repository_ids ?? [])).sort();
    const activeWorkspaceId =
      workspaces.workspaces.find((workspace) => workspace.id === workspaceId)?.id ??
      workspaces.workspaces.find((workspace) => workspace.id === settingsWorkspaceId)?.id ??
      workspaces.workspaces[0]?.id ??
      null;
    let initialState: Partial<AppState> = {
      workspaces: {
        items: workspaces.workspaces.map((workspace) => ({
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
        activeId: activeWorkspaceId,
      },
      userSettings: {
        workspaceId: settingsWorkspaceId,
        boardId: settingsBoardId,
        repositoryIds: settingsRepositoryIds,
        preferredShell: userSettings?.preferred_shell || null,
        defaultEditorId: userSettings?.default_editor_id || null,
        loaded: Boolean(userSettings),
      },
    };

    if (!activeWorkspaceId) {
      return (
        <>
          <StateHydrator initialState={initialState} />
          <PageClient />
        </>
      );
    }

    const [boardList, repositoriesResponse] = await Promise.all([
      listBoards(activeWorkspaceId, { cache: 'no-store' }),
      listRepositories(activeWorkspaceId, { cache: 'no-store' }).catch(() => ({ repositories: [] })),
    ]);

    // Use boardIdParam if it exists in this workspace, otherwise fall back
    // Client-side code will handle task selection even if board doesn't match
    const boardId =
      boardList.boards.find((board) => board.id === boardIdParam)?.id ??
      boardList.boards.find((board) => board.id === settingsBoardId)?.id ??
      boardList.boards[0]?.id;
    initialState = {
      ...initialState,
      boards: {
        items: boardList.boards.map((board) => ({
          id: board.id,
          workspaceId: board.workspace_id,
          name: board.name,
        })),
        activeId: boardId ?? null,
      },
      repositories: {
        itemsByWorkspaceId: { [activeWorkspaceId]: repositoriesResponse.repositories },
        loadingByWorkspaceId: { [activeWorkspaceId]: false },
        loadedByWorkspaceId: { [activeWorkspaceId]: true },
      },
    };

    if (!boardId) {
      return (
        <>
          <StateHydrator initialState={initialState} />
          <PageClient />
        </>
      );
    }

    const snapshot = await fetchBoardSnapshot(boardId, { cache: 'no-store' });
    initialState = { ...initialState, ...snapshotToState(snapshot) };

    // Load messages for selected task session if provided (SSR optimization)
    if (taskId && sessionId) {
      try {
        const messagesResponse = await listTaskSessionMessages(sessionId, { limit: 50, sort: 'asc' }, { cache: 'no-store' });
        initialState = {
          ...initialState,
          messages: {
            bySession: {
              [sessionId]: messagesResponse.messages ?? [],
            },
            metaBySession: {
              [sessionId]: {
                isLoading: false,
                hasMore: messagesResponse.has_more ?? false,
                oldestCursor: messagesResponse.cursor ?? (messagesResponse.messages?.[0]?.id ?? null),
              },
            },
          },
        };
      } catch (error) {
        // SSR failed - client will load messages via WebSocket
        console.warn('Could not SSR messages (client will load via WebSocket):', error instanceof Error ? error.message : String(error));
      }
    }

    return (
      <>
        <StateHydrator initialState={initialState} />
        <PageClient initialTaskId={taskId} initialSessionId={sessionId} />
      </>
    );
  } catch {
    return <PageClient />;
  }
}
