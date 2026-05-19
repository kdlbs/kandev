import type { Repository, RepositoryScript } from "@/lib/types/http";

export type RepositoryWithScripts = Repository & { scripts: RepositoryScript[] };

export function cloneRepository(repo: RepositoryWithScripts): RepositoryWithScripts {
  return { ...repo, scripts: repo.scripts.map((script) => ({ ...script })) };
}

export function isRepositoryDirty(
  repo: RepositoryWithScripts,
  saved: RepositoryWithScripts | undefined,
): boolean {
  if (!saved) return true;
  return (
    repo.name !== saved.name ||
    repo.source_type !== saved.source_type ||
    repo.local_path !== saved.local_path ||
    repo.provider !== saved.provider ||
    repo.provider_repo_id !== saved.provider_repo_id ||
    repo.provider_owner !== saved.provider_owner ||
    repo.provider_name !== saved.provider_name ||
    repo.default_branch !== saved.default_branch ||
    repo.worktree_branch_prefix !== saved.worktree_branch_prefix ||
    repo.pull_before_worktree !== saved.pull_before_worktree ||
    repo.setup_script !== saved.setup_script ||
    repo.cleanup_script !== saved.cleanup_script ||
    repo.dev_script !== saved.dev_script ||
    repo.copy_files !== saved.copy_files
  );
}

export function areRepositoryScriptsDirty(
  repo: RepositoryWithScripts,
  saved: RepositoryWithScripts | undefined,
): boolean {
  if (!saved) return repo.scripts.length > 0;
  if (repo.scripts.length !== saved.scripts.length) return true;
  const savedScripts = new Map(saved.scripts.map((script) => [script.id, script]));
  for (const script of repo.scripts) {
    const savedScript = savedScripts.get(script.id);
    if (
      !savedScript ||
      script.name !== savedScript.name ||
      script.command !== savedScript.command ||
      script.position !== savedScript.position
    )
      return true;
  }
  return false;
}
