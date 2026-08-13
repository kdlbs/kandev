import { useCallback } from "react";

type ScopedGitOperations<T> = {
  stage: (paths?: string[], repo?: string) => Promise<T>;
  unstage: (paths?: string[], repo?: string) => Promise<T>;
};

export function useScopedStageOperations<T>(
  gitOps: ScopedGitOperations<T>,
  stageAll: () => Promise<T>,
  stageFile: (paths: string[], repo?: string) => Promise<T>,
  unstageAll: () => Promise<T>,
  unstageFile: (paths: string[], repo?: string) => Promise<T>,
) {
  const stage = useCallback(
    (paths?: string[], repo?: string) => {
      if (paths && paths.length > 0) return stageFile(paths, repo);
      if (repo !== undefined) return gitOps.stage(undefined, repo);
      return stageAll();
    },
    [gitOps, stageAll, stageFile],
  );
  const unstage = useCallback(
    (paths?: string[], repo?: string) => {
      if (paths && paths.length > 0) return unstageFile(paths, repo);
      if (repo !== undefined) return gitOps.unstage(undefined, repo);
      return unstageAll();
    },
    [gitOps, unstageAll, unstageFile],
  );
  return { stage, unstage };
}
