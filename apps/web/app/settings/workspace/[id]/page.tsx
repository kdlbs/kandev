import { getWorkspaceAction, listBoardsAction, listColumnsAction } from '@/app/actions/workspaces';
import { WorkspaceEditClient } from '@/app/settings/workspace/workspace-edit-client';

export default async function WorkspaceEditPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  let workspace = null;
  let boardsWithColumns: Array<Awaited<ReturnType<typeof listBoardsAction>>['boards'][number] & { columns: Awaited<ReturnType<typeof listColumnsAction>>['columns'] }> = [];

  try {
    workspace = await getWorkspaceAction(id);
    const boards = await listBoardsAction(id);
    boardsWithColumns = await Promise.all(
      boards.boards.map(async (board) => {
        const columns = await listColumnsAction(board.id);
        return { ...board, columns: columns.columns };
      })
    );
  } catch {
    workspace = null;
    boardsWithColumns = [];
  }

  return <WorkspaceEditClient workspace={workspace} boards={boardsWithColumns} />;
}
