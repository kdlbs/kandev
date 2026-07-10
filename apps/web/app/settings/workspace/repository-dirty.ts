import type { Repository, RepositoryScript, WorktreeFile } from "@/lib/types/http";

type RepositoryWithScripts = Repository & { scripts: RepositoryScript[] };

function areWorktreeFilesEqual(
  a: WorktreeFile[] | undefined,
  b: WorktreeFile[] | undefined,
): boolean {
  const left = a ?? [];
  const right = b ?? [];
  if (left.length !== right.length) return false;
  return left.every(
    (file, index) => file.path === right[index].path && file.mode === right[index].mode,
  );
}

// Scalar repository fields compared field-by-field for dirty tracking. Kept as a
// data-driven list so isRepositoryDirty stays under the complexity limit.
const REPOSITORY_SCALAR_FIELDS: (keyof Repository)[] = [
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
];

export function isRepositoryDirty(
  repo: RepositoryWithScripts,
  saved: RepositoryWithScripts | undefined,
): boolean {
  if (!saved) return true;
  if (REPOSITORY_SCALAR_FIELDS.some((field) => repo[field] !== saved[field])) return true;
  return !areWorktreeFilesEqual(repo.worktree_files, saved.worktree_files);
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
