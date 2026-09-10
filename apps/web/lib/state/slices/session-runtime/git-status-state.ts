import type { FileChangeFacet, FileInfo, GitStatusEntry, SessionRuntimeSliceState } from "./types";
import { createDebugLogger, isDebug } from "@/lib/debug/log";

const debugGit = createDebugLogger("git-status:store");

/** Compute total additions/deletions across all files. */
function computeFileStats(files: Record<string, FileInfo> | undefined): {
  additions: number;
  deletions: number;
} {
  if (!files) return { additions: 0, deletions: 0 };
  let additions = 0;
  let deletions = 0;
  for (const f of Object.values(files)) {
    additions += f.additions || 0;
    deletions += f.deletions || 0;
  }
  return { additions, deletions };
}

function sameStringList(existing: string[] | undefined, incoming: string[] | undefined): boolean {
  const a = existing ?? [];
  const b = incoming ?? [];
  if (a.length !== b.length) return false;
  const sortedA = [...a].sort();
  const sortedB = [...b].sort();
  return sortedA.every((value, index) => value === sortedB[index]);
}

const COMPARABLE_FILE_FIELDS = [
  "path",
  "status",
  "staged",
  "additions",
  "deletions",
  "old_path",
  "diff",
  "diff_skip_reason",
  "staged_change",
  "unstaged_change",
  "repository_name",
  "is_submodule",
] as const;

function comparableChangeFacet(facet: FileChangeFacet | undefined): string {
  if (!facet) return "";
  return [
    facet.status,
    facet.additions ?? 0,
    facet.deletions ?? 0,
    facet.old_path ?? "",
    facet.diff ?? "",
    facet.diff_skip_reason ?? "",
  ].join("\0");
}

function comparableFileInfo(file: FileInfo) {
  return {
    path: file.path,
    status: file.status,
    staged: file.staged,
    additions: file.additions ?? 0,
    deletions: file.deletions ?? 0,
    old_path: file.old_path ?? "",
    diff: file.diff ?? "",
    diff_skip_reason: file.diff_skip_reason ?? "",
    staged_change: comparableChangeFacet(file.staged_change),
    unstaged_change: comparableChangeFacet(file.unstaged_change),
    repository_name: file.repository_name ?? "",
    is_submodule: file.is_submodule ?? false,
  };
}

function sameFileInfo(existing: FileInfo | undefined, incoming: FileInfo | undefined): boolean {
  if (!existing || !incoming) return existing === incoming;
  const a = comparableFileInfo(existing);
  const b = comparableFileInfo(incoming);
  return COMPARABLE_FILE_FIELDS.every((field) => a[field] === b[field]);
}

function sameFiles(
  existingFiles: Record<string, FileInfo> | undefined,
  newFiles: Record<string, FileInfo> | undefined,
): boolean {
  if (!existingFiles || !newFiles) return existingFiles === newFiles;
  const existingFileKeys = Object.keys(existingFiles).sort();
  const newFileKeys = Object.keys(newFiles).sort();
  if (existingFileKeys.length !== newFileKeys.length) return false;
  for (let i = 0; i < existingFileKeys.length; i += 1) {
    const key = existingFileKeys[i];
    if (key !== newFileKeys[i]) return false;
    if (!sameFileInfo(existingFiles[key], newFiles[key])) return false;
  }
  return true;
}

function hasBranchSummaryChanged(existing: GitStatusEntry, incoming: GitStatusEntry): boolean {
  return (
    hasBranchReferencesChanged(existing, incoming) ||
    hasComparisonSummaryChanged(existing, incoming) ||
    existing.ahead !== incoming.ahead ||
    existing.behind !== incoming.behind ||
    existing.remote_ahead !== incoming.remote_ahead ||
    existing.remote_behind !== incoming.remote_behind ||
    existing.remote_head_commit !== incoming.remote_head_commit ||
    (existing.repository_name ?? "") !== (incoming.repository_name ?? "") ||
    existing.is_submodule !== incoming.is_submodule ||
    existing.branch_additions !== incoming.branch_additions ||
    existing.branch_deletions !== incoming.branch_deletions
  );
}

function hasBranchReferencesChanged(existing: GitStatusEntry, incoming: GitStatusEntry): boolean {
  return (
    existing.branch !== incoming.branch ||
    existing.remote_branch !== incoming.remote_branch ||
    existing.head_commit !== incoming.head_commit ||
    existing.base_commit !== incoming.base_commit
  );
}

function hasComparisonSummaryChanged(existing: GitStatusEntry, incoming: GitStatusEntry): boolean {
  return (
    existing.comparison_target !== incoming.comparison_target ||
    existing.comparison_status !== incoming.comparison_status ||
    existing.comparison_error_code !== incoming.comparison_error_code
  );
}

function hasFileListsChanged(existing: GitStatusEntry, incoming: GitStatusEntry): boolean {
  return (
    !sameStringList(existing.modified, incoming.modified) ||
    !sameStringList(existing.added, incoming.added) ||
    !sameStringList(existing.deleted, incoming.deleted) ||
    !sameStringList(existing.untracked, incoming.untracked) ||
    !sameStringList(existing.renamed, incoming.renamed)
  );
}

function hasFileStatsChanged(existing: GitStatusEntry, incoming: GitStatusEntry): boolean {
  // Fast early-exit: aggregate totals differ → sameFiles would also return false,
  // but this avoids the per-file deep comparison when the gross numbers differ.
  const existingTotal = computeFileStats(existing.files);
  const newTotal = computeFileStats(incoming.files);
  return (
    existingTotal.additions !== newTotal.additions || existingTotal.deletions !== newTotal.deletions
  );
}

/** Compare two git status entries to determine if a meaningful change occurred. */
export function hasGitStatusChanged(existing: GitStatusEntry, incoming: GitStatusEntry): boolean {
  // The backend also emits fresh snapshots for focus/startup/poll events. Those
  // can carry a new timestamp for identical git data, so timestamp alone must
  // not force a store update or diff-cache invalidation.
  return (
    hasBranchSummaryChanged(existing, incoming) ||
    hasFileListsChanged(existing, incoming) ||
    hasFileStatsChanged(existing, incoming) ||
    !sameFiles(existing.files, incoming.files)
  );
}

function gitStatusTimestamp(timestamp: string | null | undefined): number | null {
  if (!timestamp) return null;
  const parsed = Date.parse(timestamp);
  return Number.isFinite(parsed) ? parsed : null;
}

function acceptsGitStatusTimestamp(
  existing: GitStatusEntry | undefined,
  incoming: GitStatusEntry,
): boolean {
  const existingTimestamp = gitStatusTimestamp(existing?.timestamp);
  const incomingTimestamp = gitStatusTimestamp(incoming.timestamp);
  if (existingTimestamp !== null && incomingTimestamp === null) return false;
  if (
    existingTimestamp !== null &&
    incomingTimestamp !== null &&
    incomingTimestamp < existingTimestamp
  ) {
    return false;
  }
  return true;
}

function advancesGitStatusTimestamp(existing: GitStatusEntry, incoming: GitStatusEntry): boolean {
  const existingTimestamp = gitStatusTimestamp(existing.timestamp);
  const incomingTimestamp = gitStatusTimestamp(incoming.timestamp);
  return (
    incomingTimestamp !== null &&
    (existingTimestamp === null || incomingTimestamp > existingTimestamp)
  );
}

type GitStatusUpdateContext = {
  state: SessionRuntimeSliceState;
  taskEnvironmentId: string;
  envKey: string;
  repoName: string;
  repoMap: Record<string, GitStatusEntry>;
  existing: GitStatusEntry | undefined;
  incoming: GitStatusEntry;
  changed: boolean;
  timestampAdvanced: boolean;
};

function logRejectedGitStatus(
  taskEnvironmentId: string,
  envKey: string,
  repoName: string,
  existing: GitStatusEntry | undefined,
  incoming: GitStatusEntry,
) {
  if (!isDebug()) return;
  debugGit("setGitStatus", {
    taskEnvironmentId,
    envKey,
    repoName,
    prevTimestamp: existing?.timestamp,
    nextTimestamp: incoming.timestamp,
    orderingRejected: true,
  });
}

function logAcceptedGitStatus(context: GitStatusUpdateContext) {
  if (!isDebug()) return;
  debugGit("setGitStatus", {
    taskEnvironmentId: context.taskEnvironmentId,
    envKey: context.envKey,
    repoName: context.repoName,
    prevFileCount: Object.keys(context.existing?.files ?? {}).length,
    nextFileCount: Object.keys(context.incoming.files ?? {}).length,
    prevRepoKeys: Object.keys(context.repoMap),
    willMutate: context.changed,
    prevTimestamp: context.existing?.timestamp,
    nextTimestamp: context.incoming.timestamp,
    orderingRejected: false,
    timestampAdvanced: context.timestampAdvanced,
  });
}

function updateRepoStatus(context: GitStatusUpdateContext) {
  if (context.changed) {
    context.repoMap[context.repoName] = context.incoming;
  } else if (context.existing && context.timestampAdvanced) {
    // Preserve the deep-content identity and cumulative diff cache while
    // advancing the ordering watermark for a duplicate status snapshot.
    context.existing.timestamp = context.incoming.timestamp;
  }
}

function mirrorLegacyGitStatus(context: GitStatusUpdateContext): boolean {
  if (context.repoName !== "") {
    // Multi-repo: only mirror into the legacy map when this repo's entry changed.
    if (context.changed) context.state.gitStatus.byEnvironmentId[context.envKey] = context.incoming;
    else if (
      context.timestampAdvanced &&
      context.state.gitStatus.byEnvironmentId[context.envKey]?.repository_name === context.repoName
    ) {
      context.state.gitStatus.byEnvironmentId[context.envKey].timestamp =
        context.incoming.timestamp;
    }
    return context.changed;
  }

  // The empty-repo entry and byEnvironmentId track together (written and cleared
  // as a pair), so existingRepo presence/equality matches the env entry — reuse
  // changed instead of comparing diffs again.
  const legacyChanged = !context.state.gitStatus.byEnvironmentId[context.envKey] || context.changed;
  if (legacyChanged) context.state.gitStatus.byEnvironmentId[context.envKey] = context.incoming;
  else if (context.timestampAdvanced && context.state.gitStatus.byEnvironmentId[context.envKey]) {
    context.state.gitStatus.byEnvironmentId[context.envKey].timestamp = context.incoming.timestamp;
  }
  return legacyChanged;
}

/** Write a git-status update into the env/per-repo maps, skipping writes when
 * nothing meaningfully changed. Returns whether git state changed so the WS
 * handler can invalidate derived caches without repeating the deep comparison. */
export function applyGitStatus(
  state: SessionRuntimeSliceState,
  taskEnvironmentId: string,
  gitStatus: GitStatusEntry,
): boolean {
  if (!taskEnvironmentId) return false;
  const envKey = taskEnvironmentId;
  const repoName = gitStatus.repository_name ?? "";
  const repoMap = (state.gitStatus.byEnvironmentRepo[envKey] ??= {});
  const existingRepo = repoMap[repoName];
  if (!acceptsGitStatusTimestamp(existingRepo, gitStatus)) {
    logRejectedGitStatus(taskEnvironmentId, envKey, repoName, existingRepo, gitStatus);
    return false;
  }

  const timestampAdvanced = existingRepo
    ? advancesGitStatusTimestamp(existingRepo, gitStatus)
    : true;
  const changed = !existingRepo || hasGitStatusChanged(existingRepo, gitStatus);
  const context: GitStatusUpdateContext = {
    state,
    taskEnvironmentId,
    envKey,
    repoName,
    repoMap,
    existing: existingRepo,
    incoming: gitStatus,
    changed,
    timestampAdvanced,
  };
  logAcceptedGitStatus(context);
  updateRepoStatus(context);
  return mirrorLegacyGitStatus(context);
}
