import { useMemo } from "react";
import { useSessionGitStatusByRepo } from "./use-session-git-status";
import { useSessionCommits } from "./use-session-commits";

/**
 * Total count of uncommitted file changes + commits for a session, summed
 * across every repo in multi-repo workspaces.
 *
 * `useSessionGitStatus` exposes only the single status that arrived last
 * (documented "last write wins" behaviour), so any badge or title sourced
 * directly from it loses changes from sibling repos as new updates land — the
 * count flickers as repos take turns overwriting each other, and a stale
 * value can persist after the *current* session's actual changes arrive on
 * a different repo's status. Read this hook instead whenever the UI needs a
 * single number representing the session's total changes.
 */
export function useSessionChangesCount(sessionId: string | null): number {
  const statusByRepo = useSessionGitStatusByRepo(sessionId);
  const { commits } = useSessionCommits(sessionId);
  return useMemo(() => {
    let fileCount = 0;
    for (const { status } of statusByRepo) {
      if (status?.files) fileCount += Object.keys(status.files).length;
    }
    return fileCount + commits.length;
  }, [statusByRepo, commits.length]);
}
