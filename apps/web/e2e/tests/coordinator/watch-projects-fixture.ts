import fs from "node:fs";
import path from "node:path";
import { execSync } from "node:child_process";
import type { ApiClient } from "../../helpers/api-client";
import { makeGitEnv } from "../../helpers/git-helper";

export const PHASE31_ENV = { KANDEV_FEATURES_COORDINATOR_PHASE31: "true" };

/** A workspace repository backed by a fresh local git directory. */
export async function createLocalRepository(
  apiClient: ApiClient,
  backendTmpDir: string,
  workspaceId: string,
  name: string,
): Promise<{ id: string }> {
  const repoDir = path.join(backendTmpDir, "repos", name.toLowerCase().replaceAll(" ", "-"));
  fs.mkdirSync(repoDir, { recursive: true });
  const env = makeGitEnv(backendTmpDir);
  execSync("git init -b main", { cwd: repoDir, env });
  execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env });
  return apiClient.createRepository(workspaceId, repoDir, "main", { name });
}
