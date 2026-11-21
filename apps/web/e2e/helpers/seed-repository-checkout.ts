import { execFileSync } from "node:child_process";
import type { SeedData } from "../fixtures/test-base";
import { makeGitEnv } from "./git-helper";

/** Restores the shared seed checkout to the immutable fixture baseline. */
export function resetSeedRepositoryCheckout(seedData: SeedData, tmpDir: string) {
  const env = makeGitEnv(tmpDir);
  execFileSync("git", ["-C", seedData.repositoryPath, "checkout", "-f", "main"], {
    env,
    stdio: "ignore",
  });
  execFileSync(
    "git",
    ["-C", seedData.repositoryPath, "reset", "--hard", seedData.repositoryBaselineOID],
    {
      env,
      stdio: "ignore",
    },
  );
  execFileSync("git", ["-C", seedData.repositoryPath, "clean", "-fd"], {
    env,
    stdio: "ignore",
  });
}
