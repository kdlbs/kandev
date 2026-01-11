import { PageClient } from '@/app/page-client';
import { StateHydrator } from '@/components/state-hydrator';
import { fetchBoardSnapshot, fetchBoards } from '@/lib/ssr/http';
import { snapshotToState } from '@/lib/ssr/mapper';

export default async function Page() {
  try {
    const boards = await fetchBoards();
    const boardId = boards[0]?.id;
    if (!boardId) {
      return <PageClient />;
    }
    const snapshot = await fetchBoardSnapshot(boardId);
    const initialState = snapshotToState(snapshot);
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
