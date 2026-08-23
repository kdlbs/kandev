import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { execFileSync } from "node:child_process";
import { expect, type Locator } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { dwell } from "../../helpers/causal-waits";
import type { SeedData } from "../../fixtures/test-base";
import { makeGitEnv } from "../../helpers/git-helper";

const GIT_PROTOCOL_ARGS = ["-c", "protocol.file.allow=always"];
const GIT_INDEX_LOCK_ATTEMPTS = 3;
const GIT_INDEX_LOCK_RETRY_MS = 300;

async function waitForFiniteAnimations(surface: Locator): Promise<void> {
  await surface.evaluate(async (element) => {
    const animations = element.getAnimations({ subtree: true }).filter((animation) => {
      const iterations = animation.effect?.getComputedTiming().iterations;
      return typeof iterations === "number" && Number.isFinite(iterations);
    });
    await Promise.all(animations.map((animation) => animation.finished.catch(() => undefined)));
  });
}

export async function expectStickyReviewHeaderClearance(
  review: Locator,
  pointer: "mouse" | "touch",
): Promise<void> {
  const rootGroup = review.locator('[data-testid="changes-repo-group"][data-repository-name=""]');
  const repositoryHeader = rootGroup.getByTestId("changes-repo-header");
  const fileHeader = rootGroup.locator(
    '[data-testid="review-file-header"][data-file-path="README.md"]',
  );
  const fileSection = fileHeader.locator("..");
  const disclosure = fileHeader.getByRole("button", {
    name: /^(Collapse|Expand) README\.md$/,
  });

  await expect(repositoryHeader).toContainText("Other changes");
  await expect(fileHeader).toHaveCount(1);
  await expect(disclosure).toHaveCount(1);
  await waitForFiniteAnimations(review);
  await fileSection.evaluate((element) =>
    element.scrollIntoView({ behavior: "auto", block: "start" }),
  );

  await expect
    .poll(
      async () => {
        const [repositoryBox, fileBox] = await Promise.all([
          repositoryHeader.boundingBox(),
          fileHeader.boundingBox(),
        ]);
        const disclosureHit = await disclosure.evaluate((control) => {
          const box = control.getBoundingClientRect();
          const hit = document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2);
          return hit === control || (hit !== null && control.contains(hit));
        });
        const clearance =
          repositoryBox && fileBox
            ? Math.round((fileBox.y - (repositoryBox.y + repositoryBox.height)) * 100) / 100
            : null;
        return {
          clearance,
          headersSeparated: clearance !== null && clearance >= 0,
          disclosureHit,
        };
      },
      { message: "repository header must not cover the current file header or disclosure" },
    )
    .toEqual({
      clearance: expect.any(Number),
      headersSeparated: true,
      disclosureHit: true,
    });

  if (pointer === "touch") {
    await disclosure.tap();
  } else {
    await disclosure.click();
  }
  await expect(disclosure).toHaveAttribute("aria-expanded", "false");
}

export type SubmoduleReviewFixture = {
  taskId: string;
  sessionId: string;
  sourceRoot: string;
  waitForWorktree: (apiClient: ApiClient) => Promise<string>;
  applyNestedChanges: (worktreePath: string) => Promise<void>;
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

/** Retries the short-lived index lock taken by the backend's Git status refresh. */
export async function retryGitIndexLock<T>(operation: () => T): Promise<T> {
  for (let attempt = 0; attempt < GIT_INDEX_LOCK_ATTEMPTS; attempt++) {
    try {
      return operation();
    } catch (error) {
      const isLastAttempt = attempt === GIT_INDEX_LOCK_ATTEMPTS - 1;
      if (!(error instanceof Error) || !error.message.includes("index.lock") || isLastAttempt) {
        throw error;
      }
      await dwell(
        GIT_INDEX_LOCK_RETRY_MS,
        "poll-interval",
        "the backend's periodic Git status refresh can briefly hold the submodule index lock without publishing a completion event",
      );
    }
  }
  throw new Error("Git index lock retry exhausted");
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

async function commit(repoPath: string, env: NodeJS.ProcessEnv, message: string): Promise<void> {
  await retryGitIndexLock(() => runGit(repoPath, ["add", "-A"], env));
  await retryGitIndexLock(() => runGit(repoPath, ["commit", "-m", message], env));
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
    await dwell(
      250,
      "poll-interval",
      "sampling interval for the loop above; no Page exists here, and the worktree appears in a session record the backend writes without publishing anything this helper can subscribe to",
    );
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
  const cleanup = () => fs.rmSync(sourceRoot, { recursive: true, force: true });

  try {
    initializeRepository(innerPath, env, "README.md", "inner base\n");

    initializeRepository(outerPath, env, "README.md", "outer base\n");
    runGit(outerPath, [...GIT_PROTOCOL_ARGS, "submodule", "add", "../inner", "vendor/inner"], env);
    await commit(outerPath, env, "add nested inner submodule");

    initializeRepository(parentPath, env, "README.md", "parent base\n");
    runGit(parentPath, [...GIT_PROTOCOL_ARGS, "submodule", "add", "../outer", "vendor/outer"], env);
    runGit(parentPath, [...GIT_PROTOCOL_ARGS, "submodule", "update", "--init", "--recursive"], env);
    runGit(
      path.join(parentPath, "vendor/outer"),
      [...GIT_PROTOCOL_ARGS, "submodule", "update", "--init", "--recursive"],
      env,
    );
    await commit(parentPath, env, "add outer submodule");

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
      async applyNestedChanges(worktreePath: string) {
        const outerWorktree = path.join(worktreePath, "vendor/outer");
        const innerWorktree = path.join(outerWorktree, "vendor/inner");
        if (!fs.existsSync(innerWorktree)) {
          await retryGitIndexLock(() =>
            runGit(
              worktreePath,
              [...GIT_PROTOCOL_ARGS, "submodule", "update", "--init", "--recursive"],
              env,
            ),
          );
        }
        fs.appendFileSync(
          path.join(worktreePath, "README.md"),
          `parent working-tree change\n${"root review line\n".repeat(80)}`,
        );
        fs.appendFileSync(path.join(outerWorktree, "README.md"), "outer committed change\n");
        await commit(outerWorktree, env, "change outer submodule");
        fs.appendFileSync(path.join(innerWorktree, "README.md"), "inner committed change\n");
        await commit(innerWorktree, env, "change inner submodule");
        await commit(outerWorktree, env, "record inner submodule change");
      },
      cleanup,
    };
  } catch (error) {
    cleanup();
    throw error;
  }
}
