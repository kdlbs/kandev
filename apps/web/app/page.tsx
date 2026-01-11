import { PageClient } from '@/app/page-client';
import { StateHydrator } from '@/components/state-hydrator';
import { fetchBoardSnapshot, fetchBoards, fetchWorkspaces } from '@/lib/ssr/http';
import { snapshotToState } from '@/lib/ssr/mapper';

// Server Component: runs on the server for SSR and data hydration.
export default async function Page() {
  try {
    const workspaces = await fetchWorkspaces();
    const activeWorkspaceId = workspaces.workspaces[0]?.id ?? null;
    let initialState = {
      workspaces: {
        items: workspaces.workspaces.map((workspace) => ({
          id: workspace.id,
          name: workspace.name,
        })),
        activeId: activeWorkspaceId,
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
    const boardId = boardList.boards[0]?.id;
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
