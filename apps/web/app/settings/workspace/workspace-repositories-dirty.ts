import type { Repository, RepositoryScript } from "@/lib/types/http";
import { defaultWorktreeBranchTemplate } from "@/lib/worktree-branch-template";

export type RepositoryWithScripts = Repository & { scripts: RepositoryScript[] };

export function cloneRepository(repo: RepositoryWithScripts): RepositoryWithScripts {
  return { ...repo, scripts: repo.scripts.map((script) => ({ ...script })) };
}

// worktree_branch_template is compared separately via branchTemplate() so an
// empty string on either side is treated as the default template value.
const REPOSITORY_DIRTY_FIELDS = [
  "name",
  "source_type",
  "local_path",
  "provider",
  "provider_repo_id",
  "provider_owner",
  "provider_name",
  "default_branch",
  "worktree_branch_prefix",
  "pull_before_worktree",
  "setup_script",
  "cleanup_script",
  "dev_script",
  "copy_files",
  "startup_prompt",
] as const satisfies ReadonlyArray<keyof Repository>;

function branchTemplate(repo: RepositoryWithScripts): string {
  return repo.worktree_branch_template || defaultWorktreeBranchTemplate;
}

export function isRepositoryDirty(
  repo: RepositoryWithScripts,
  saved: RepositoryWithScripts | undefined,
): boolean {
  if (!saved) return true;
  return (
    REPOSITORY_DIRTY_FIELDS.some((field) => repo[field] !== saved[field]) ||
    branchTemplate(repo) !== branchTemplate(saved)
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
