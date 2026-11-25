import { execFileSync } from "node:child_process";

type SeedRepository = { repositoryPath: string; repositoryRemoteURL: string };

/** Forget the old remote's default without deleting its cached target branch. */
export function clearSeedRepositoryRemoteDefault(
  seedData: Pick<SeedRepository, "repositoryPath">,
  env: NodeJS.ProcessEnv,
): void {
  execFileSync(
    "git",
    ["-C", seedData.repositoryPath, "update-ref", "--no-deref", "-d", "refs/remotes/origin/HEAD"],
    { env, stdio: "pipe" },
  );
}

/**
 * Restore the offline origin. Integration cleanup defers tracking-ref refresh
 * until task reset; direct branch-recovery callers refresh by default.
 */
export function restoreSeedRepositoryOrigin(
  seedData: SeedRepository,
  options: { refreshTrackingRefs?: boolean } = {},
) {
  const baseArgs = ["-C", seedData.repositoryPath, "remote"];
  try {
    execFileSync("git", [...baseArgs, "set-url", "origin", seedData.repositoryRemoteURL], {
      stdio: "ignore",
    });
  } catch {
    execFileSync("git", [...baseArgs, "add", "origin", seedData.repositoryRemoteURL], {
      stdio: "ignore",
    });
  }
  if (options.refreshTrackingRefs !== false) {
    execFileSync("git", ["-C", seedData.repositoryPath, "worktree", "prune", "--expire", "now"], {
      stdio: "pipe",
    });
    execFileSync("git", ["-C", seedData.repositoryPath, "fetch", "--no-tags", "origin"], {
      stdio: "pipe",
    });
  }
}
