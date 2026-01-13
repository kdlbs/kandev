import { PageClient } from '@/app/page-client';
import { StateHydrator } from '@/components/state-hydrator';
import { fetchBoardSnapshot, fetchBoards, fetchUserSettings, fetchWorkspaces } from '@/lib/ssr/http';
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
    const workspaceId = Array.isArray(workspaceParam) ? workspaceParam[0] : workspaceParam;
    const boardIdParam = Array.isArray(boardParam) ? boardParam[0] : boardParam;

    const [workspaces, userSettingsResponse] = await Promise.all([
      fetchWorkspaces(),
      fetchUserSettings().catch(() => null),
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
        })),
        activeId: activeWorkspaceId,
      },
      userSettings: {
        workspaceId: settingsWorkspaceId,
        boardId: settingsBoardId,
        repositoryIds: settingsRepositoryIds,
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

    const boardList = await fetchBoards(activeWorkspaceId);
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
    };

    if (!boardId) {
      return (
        <>
          <StateHydrator initialState={initialState} />
          <PageClient />
        </>
      );
    }

    const snapshot = await fetchBoardSnapshot(boardId);
    initialState = { ...initialState, ...snapshotToState(snapshot) };
    return (
      <>
        <StateHydrator initialState={initialState} />
        <PageClient />
      </>
    );
  } catch {
    return <PageClient />;
  }
}
