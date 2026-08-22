import type {
  FileInfo,
  GitStatusEntry,
  SessionCommit,
} from "@/lib/state/slices/session-runtime/types";

function deriveBranchValues(gitStatus: GitStatusEntry | undefined) {
  const ahead = gitStatus?.ahead ?? 0;
  return {
    branch: gitStatus?.branch ?? null,
    remoteBranch: gitStatus?.remote_branch ?? null,
    headCommit: gitStatus?.head_commit ?? null,
    remoteHeadCommit: gitStatus?.remote_head_commit ?? null,
    ahead,
    behind: gitStatus?.behind ?? 0,
  };
}

function deriveUpstreamValues(gitStatus: GitStatusEntry | undefined, ahead: number) {
  const hasUpstream = Boolean(gitStatus?.remote_branch);
  const remoteAhead = gitStatus?.remote_ahead ?? 0;
  const remoteBehind = gitStatus?.remote_behind ?? 0;
  return {
    remoteAhead,
    remoteBehind,
    pushAhead: hasUpstream ? remoteAhead : ahead,
    pullBehind: hasUpstream ? remoteBehind : 0,
  };
}

function deriveChangeValues(
  unstagedFiles: FileInfo[],
  stagedFiles: FileInfo[],
  commits: SessionCommit[],
) {
  const hasUnstaged = unstagedFiles.length > 0;
  const hasStaged = stagedFiles.length > 0;
  const hasCommits = commits.length > 0;
  return { hasUnstaged, hasStaged, hasCommits, hasChanges: hasUnstaged || hasStaged };
}

export function deriveSessionGitValues(
  gitStatus: GitStatusEntry | undefined,
  hasRepositoryStatuses: boolean,
  unstagedFiles: FileInfo[],
  stagedFiles: FileInfo[],
  commits: SessionCommit[],
) {
  const branch = deriveBranchValues(gitStatus);
  const upstream = deriveUpstreamValues(gitStatus, branch.ahead);
  const status = { ...branch, ...upstream };
  const changes = deriveChangeValues(unstagedFiles, stagedFiles, commits);
  const hasAnything = changes.hasChanges || changes.hasCommits;
  return {
    ...status,
    statusLoaded: Boolean(gitStatus || hasRepositoryStatuses),
    ...changes,
    hasAnything,
    canStageAll: changes.hasUnstaged,
    canCommit: changes.hasStaged,
    canPush: status.pushAhead > 0,
    canPull: status.pullBehind > 0,
    canCreatePR: changes.hasCommits,
  };
}

export function deriveComparisonValues(statuses: GitStatusEntry[]) {
  const comparisonTargets = Array.from(
    new Set(
      statuses
        .map((status) => status.comparison_target?.trim())
        .filter((target): target is string => Boolean(target)),
    ),
  ).sort((a, b) => a.localeCompare(b));
  const unavailableStatus = statuses.find((status) => status.comparison_status === "unavailable");
  return {
    comparisonTargets,
    comparisonUnavailable: Boolean(unavailableStatus),
    comparisonErrorCode: unavailableStatus?.comparison_error_code ?? null,
  };
}
