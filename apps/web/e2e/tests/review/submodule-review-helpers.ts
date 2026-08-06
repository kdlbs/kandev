import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { execFileSync } from "node:child_process";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { makeGitEnv } from "../../helpers/git-helper";

const GIT_PROTOCOL_ARGS = ["-c", "protocol.file.allow=always"];

export type SubmoduleReviewFixture = {
  taskId: string;
  sessionId: string;
  sourceRoot: string;
  waitForWorktree: (apiClient: ApiClient) => Promise<string>;
  applyNestedChanges: (worktreePath: string) => void;
  cleanup: () => void;
};

function runGit(repoPath: string, args: string[], env: NodeJS.ProcessEnv): string {
  return execFileSync("git", args, {
    cwd: repoPath,
    env,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  });
}

function initializeRepository(
  repoPath: string,
  env: NodeJS.ProcessEnv,
  fileName: string,
  content: string,
) {
  fs.mkdirSync(repoPath, { recursive: true });
  runGit(repoPath, ["init", "-b", "main"], env);
  runGit(repoPath, ["config", "protocol.file.allow", "always"], env);
  fs.writeFileSync(path.join(repoPath, fileName), content);
  runGit(repoPath, ["add", fileName], env);
  runGit(repoPath, ["commit", "-m", "initial"], env);
}

export function readGitValue(repoPath: string, args: string[], tempRoot: string): string {
  return runGit(repoPath, args, {
    ...makeGitEnv(tempRoot),
    GIT_ALLOW_PROTOCOL: "file",
  }).trim();
}

function commit(repoPath: string, env: NodeJS.ProcessEnv, message: string): void {
  runGit(repoPath, ["add", "-A"], env);
  runGit(repoPath, ["commit", "-m", message], env);
}

async function waitForWorktreePath(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
): Promise<string> {
  const deadline = Date.now() + 90_000;
  while (Date.now() < deadline) {
    const { sessions } = await apiClient.listTaskSessions(taskId);
    const session = sessions.find((candidate) => candidate.id === sessionId);
    const worktreePath =
      session?.worktree_path ?? session?.workspace_path ?? session?.worktrees?.[0]?.worktree_path;
    if (worktreePath) return worktreePath;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error(`Timed out waiting for the nested-review worktree for ${sessionId}`);
}

export async function createSubmoduleReviewFixture(
  apiClient: ApiClient,
  seedData: SeedData,
  tempRoot: string,
  title: string,
): Promise<SubmoduleReviewFixture> {
  const sourceRoot = path.join(tempRoot, `submodule-review-${crypto.randomUUID()}`);
  const parentPath = path.join(sourceRoot, "parent");
  const outerPath = path.join(sourceRoot, "outer");
  const innerPath = path.join(sourceRoot, "inner");
  const env = { ...makeGitEnv(tempRoot), GIT_ALLOW_PROTOCOL: "file" };

  initializeRepository(innerPath, env, "README.md", "inner base\n");

  initializeRepository(outerPath, env, "README.md", "outer base\n");
  runGit(outerPath, [...GIT_PROTOCOL_ARGS, "submodule", "add", "../inner", "vendor/inner"], env);
  commit(outerPath, env, "add nested inner submodule");

  initializeRepository(parentPath, env, "README.md", "parent base\n");
  runGit(parentPath, [...GIT_PROTOCOL_ARGS, "submodule", "add", "../outer", "vendor/outer"], env);
  runGit(parentPath, [...GIT_PROTOCOL_ARGS, "submodule", "update", "--init", "--recursive"], env);
  runGit(
    path.join(parentPath, "vendor/outer"),
    [...GIT_PROTOCOL_ARGS, "submodule", "update", "--init", "--recursive"],
    env,
  );
  commit(parentPath, env, "add outer submodule");

  const repository = await apiClient.createRepository(seedData.workspaceId, parentPath, "main", {
    name: "nested-submodule-parent",
  });
  const { executors } = await apiClient.listExecutors();
  const directProfile = executors.find(
    (executor) => executor.type === "local" || executor.type === "local_pc",
  )?.profiles?.[0];
  if (!directProfile) throw new Error("Nested submodule fixture needs a direct local executor");
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [repository.id],
      executor_profile_id: directProfile.id,
    },
  );
  if (!task.session_id) throw new Error("Nested submodule fixture did not start a session");

  return {
    taskId: task.id,
    sessionId: task.session_id,
    sourceRoot,
    waitForWorktree: (client) => waitForWorktreePath(client, task.id, task.session_id!),
    applyNestedChanges(worktreePath: string) {
      const outerWorktree = path.join(worktreePath, "vendor/outer");
      const innerWorktree = path.join(outerWorktree, "vendor/inner");
      if (!fs.existsSync(innerWorktree)) {
        runGit(
          worktreePath,
          [...GIT_PROTOCOL_ARGS, "submodule", "update", "--init", "--recursive"],
          env,
        );
      }
      fs.appendFileSync(path.join(worktreePath, "README.md"), "parent working-tree change\n");
      fs.appendFileSync(path.join(outerWorktree, "README.md"), "outer committed change\n");
      commit(outerWorktree, env, "change outer submodule");
      fs.appendFileSync(path.join(innerWorktree, "README.md"), "inner committed change\n");
      commit(innerWorktree, env, "change inner submodule");
      commit(outerWorktree, env, "record inner submodule change");
    },
    cleanup() {
      fs.rmSync(sourceRoot, { recursive: true, force: true });
    },
  };
}
