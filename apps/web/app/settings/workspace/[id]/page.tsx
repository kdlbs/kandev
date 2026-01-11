import {
  getWorkspaceAction,
  listBoardsAction,
  listColumnsAction,
  listRepositoriesAction,
  listRepositoryScriptsAction,
} from '@/app/actions/workspaces';
import { WorkspaceEditClient } from '@/app/settings/workspace/workspace-edit-client';

export default async function WorkspaceEditPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  let workspace = null;
  let boardsWithColumns: Array<Awaited<ReturnType<typeof listBoardsAction>>['boards'][number] & { columns: Awaited<ReturnType<typeof listColumnsAction>>['columns'] }> = [];
  let repositoriesWithScripts: Array<Awaited<ReturnType<typeof listRepositoriesAction>>['repositories'][number] & { scripts: Awaited<ReturnType<typeof listRepositoryScriptsAction>>['scripts'] }> = [];

  try {
    workspace = await getWorkspaceAction(id);
    const boards = await listBoardsAction(id);
    boardsWithColumns = await Promise.all(
      boards.boards.map(async (board) => {
        const columns = await listColumnsAction(board.id);
        return { ...board, columns: columns.columns };
      })
    );
    const repositories = await listRepositoriesAction(id);
    repositoriesWithScripts = await Promise.all(
      repositories.repositories.map(async (repository) => {
        const scripts = await listRepositoryScriptsAction(repository.id);
        return { ...repository, scripts: scripts.scripts };
      })
    );
  } catch {
    workspace = null;
    boardsWithColumns = [];
    repositoriesWithScripts = [];
  }

  return (
    <WorkspaceEditClient
      workspace={workspace}
      boards={boardsWithColumns}
      repositories={repositoriesWithScripts}
    />
  );
}
