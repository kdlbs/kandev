import path from "node:path";
import type { Locator } from "@playwright/test";
import type { Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { BackendContext } from "../../fixtures/backend";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

export const LARGE_FILE_TREE_FOLDER = "large-file-tree";
export const LARGE_FILE_TREE_COUNT = 600;

export function largeFileTreePath(index: number): string {
  return `${LARGE_FILE_TREE_FOLDER}/entry-${index.toString().padStart(4, "0")}.txt`;
}

/** Reveal the final seeded row even when later tests have added rows after the fixture folder. */
export async function scrollToLastLargeFile(lastFile: Locator, viewport: Locator): Promise<void> {
  await viewport.evaluate((element) => {
    element.scrollTop = element.scrollHeight;
    element.dispatchEvent(new Event("scroll", { bubbles: true }));
  });
  for (let attempt = 0; attempt < 100; attempt += 1) {
    if ((await lastFile.count()) > 0) return;
    const moved = await viewport.evaluate((element) => {
      const previous = element.scrollTop;
      element.scrollTop = Math.max(0, previous - element.clientHeight);
      element.dispatchEvent(new Event("scroll", { bubbles: true }));
      return element.scrollTop < previous;
    });
    if (!moved) break;
    await viewport.evaluate(
      () => new Promise<void>((resolve) => requestAnimationFrame(() => resolve())),
    );
  }
  throw new Error("last large file row did not enter the virtualized window");
}

export function seedLargeFileTree(backend: BackendContext): void {
  const git = new GitHelper(
    path.join(backend.tmpDir, "repos", "e2e-repo"),
    makeGitEnv(backend.tmpDir),
  );
  for (let index = 0; index < LARGE_FILE_TREE_COUNT; index += 1) {
    git.createFile(largeFileTreePath(index), `large tree entry ${index}\n`);
  }
  git.stageAll();
  if (git.exec("git status --short").trim()) {
    git.commit("seed large file tree");
  }
}

export async function setupLargeFileTreeTask({
  testPage,
  apiClient,
  seedData,
  backend,
  title,
}: {
  testPage: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  backend: BackendContext;
  title: string;
}): Promise<SessionPage> {
  seedLargeFileTree(backend);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 45_000 });
  return session;
}
