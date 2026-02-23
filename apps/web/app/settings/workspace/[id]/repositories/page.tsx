import { getWorkspaceAction, listRepositoriesAction } from "@/app/actions/workspaces";
import { WorkspaceRepositoriesClient } from "@/app/settings/workspace/workspace-repositories-client";
import type { Repository, RepositoryScript } from "@/lib/types/http";

type RepositoryWithScripts = Repository & { scripts: RepositoryScript[] };

export default async function WorkspaceRepositoriesPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  let workspace = null;
  let repositoriesWithScripts: RepositoryWithScripts[] = [];

  try {
    const [ws, repoResponse] = await Promise.all([
      getWorkspaceAction(id),
      listRepositoriesAction(id, { includeScripts: true }),
    ]);
    workspace = ws;
    repositoriesWithScripts = repoResponse.repositories.map((repository) => ({
      ...repository,
      scripts: repository.scripts ?? [],
    }));
  } catch {
    workspace = null;
    repositoriesWithScripts = [];
  }

  return (
    <WorkspaceRepositoriesClient workspace={workspace} repositories={repositoriesWithScripts} />
  );
}
