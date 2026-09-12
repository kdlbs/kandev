export type PerRepoStatus = {
  repository_name: string;
  branch: string | null;
  ahead: number;
  behind: number;
  pullBehind?: number;
  hasStaged: boolean;
  hasUnstaged: boolean;
};

export type BranchRow = {
  repoLabel: string | null;
  branch: string;
  baseBranch: string;
  repositoryName: string;
};

export function buildBranchRows(
  perRepoStatus: PerRepoStatus[],
  baseBranchByRepo: Record<string, string> | undefined,
  baseBranchFallback: string,
  repoDisplayName: ((name: string) => string | undefined) | undefined,
): BranchRow[] {
  const named = perRepoStatus.filter((s) => s.repository_name !== "" && s.branch);
  if (named.length <= 1) return [];
  return named.map((s) => ({
    repoLabel: repoDisplayName?.(s.repository_name) || s.repository_name,
    branch: s.branch ?? "",
    baseBranch: baseBranchByRepo?.[s.repository_name] || baseBranchFallback,
    repositoryName: s.repository_name,
  }));
}
