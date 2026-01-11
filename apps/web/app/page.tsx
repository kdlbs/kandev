import { KanbanBoard } from '@/components/kanban-board';
import { StateHydrator } from '@/components/state-hydrator';
import { fetchBoardSnapshot, fetchBoards } from '@/lib/ssr/http';
import { snapshotToState } from '@/lib/ssr/mapper';

export default async function Page() {
  try {
    const boards = await fetchBoards();
    const boardId = boards[0]?.id;
    if (!boardId) {
      return <KanbanBoard />;
    }
    const snapshot = await fetchBoardSnapshot(boardId);
    const initialState = snapshotToState(snapshot);
    return (
      <>
        <StateHydrator initialState={initialState} />
        <KanbanBoard />
      </>
    );
  } catch {
    return <KanbanBoard />;
  }
}
