import {
  getWorkspaceAction,
  listRepositoriesAction,
  listRepositoryScriptsAction,
} from '@/app/actions/workspaces';
import { WorkspaceRepositoriesClient } from '@/app/settings/workspace/workspace-repositories-client';

export default async function WorkspaceRepositoriesPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  let workspace = null;
  let repositoriesWithScripts: Array<
    Awaited<ReturnType<typeof listRepositoriesAction>>['repositories'][number] & {
      scripts: Awaited<ReturnType<typeof listRepositoryScriptsAction>>['scripts'];
    }
  > = [];

  try {
    workspace = await getWorkspaceAction(id);
    const repositories = await listRepositoriesAction(id);
    repositoriesWithScripts = await Promise.all(
      repositories.repositories.map(async (repository) => {
        const scripts = await listRepositoryScriptsAction(repository.id);
        return { ...repository, scripts: scripts.scripts };
      })
    );
  } catch {
    workspace = null;
    repositoriesWithScripts = [];
  }

  return (
    <WorkspaceRepositoriesClient workspace={workspace} repositories={repositoriesWithScripts} />
  );
}
