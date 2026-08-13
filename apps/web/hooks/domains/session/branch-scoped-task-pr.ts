import { prTaskKey } from "@/components/github/pr-utils";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import type { TaskPR } from "@/lib/types/github";

export type RepositoryGitStatus = {
  repository_name: string;
  status: GitStatusEntry;
};

export type BranchScopedTaskPR = {
  pr: TaskPR;
  repositoryName: string | undefined;
  gitStatus: GitStatusEntry;
};

type ResolveBranchScopedTaskPRsInput = {
  prs: TaskPR[];
  statuses: RepositoryGitStatus[];
  preferredKey?: string | null;
  resolveRepositoryName: (pr: TaskPR) => string | undefined;
};

export function normalizeContributionBranch(branch: string | null | undefined): string {
  return (branch ?? "")
    .trim()
    .replace(/^refs\/heads\//, "")
    .replace(/^refs\/remotes\/[^/]+\//, "");
}

function comparePRRecency(left: TaskPR, right: TaskPR): number {
  const leftTime = Date.parse(left.updated_at || left.created_at || "") || 0;
  const rightTime = Date.parse(right.updated_at || right.created_at || "") || 0;
  return rightTime - leftTime || right.pr_number - left.pr_number;
}

function choosePR(candidates: TaskPR[], preferredKey?: string | null): TaskPR | undefined {
  const openCandidates = candidates.filter((pr) => pr.state === "open");
  const eligible = openCandidates.length > 0 ? openCandidates : candidates;
  const preferred = preferredKey
    ? eligible.find((candidate) => prTaskKey(candidate) === preferredKey)
    : undefined;
  return preferred ?? [...eligible].sort(comparePRRecency)[0];
}

function matchesStatus(
  pr: TaskPR,
  repositoryName: string | undefined,
  entry: RepositoryGitStatus,
  statusCount: number,
): boolean {
  const prBranch = normalizeContributionBranch(pr.head_branch);
  const checkoutBranch = normalizeContributionBranch(entry.status.branch);
  if (!prBranch || !checkoutBranch || prBranch !== checkoutBranch) return false;
  if (repositoryName && repositoryName === entry.repository_name) return true;

  // Agentctl keeps the legacy empty repository key for a single checkout.
  // A missing repository id is also only unambiguous when exactly one live
  // checkout exists. Never use either fallback to fan out across repositories.
  if (statusCount !== 1) return false;
  return entry.repository_name === "" || !repositoryName;
}

export function resolveBranchScopedTaskPRs({
  prs,
  statuses,
  preferredKey,
  resolveRepositoryName,
}: ResolveBranchScopedTaskPRsInput): BranchScopedTaskPR[] {
  const scoped: BranchScopedTaskPR[] = [];
  for (const entry of statuses) {
    const candidates = prs.filter((pr) =>
      matchesStatus(pr, resolveRepositoryName(pr), entry, statuses.length),
    );
    const pr = choosePR(candidates, preferredKey);
    if (!pr) continue;
    scoped.push({
      pr,
      repositoryName: resolveRepositoryName(pr),
      gitStatus: entry.status,
    });
  }
  return scoped;
}

export function selectBranchScopedTaskPR(
  scoped: BranchScopedTaskPR[],
  preferredKey?: string | null,
): BranchScopedTaskPR | undefined {
  if (preferredKey) {
    const preferred = scoped.find((entry) => prTaskKey(entry.pr) === preferredKey);
    if (preferred) return preferred;
  }
  return scoped[0];
}
