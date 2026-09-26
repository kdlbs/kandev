import path from "node:path";
import { expect, type Locator } from "@playwright/test";
import type { Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { BackendContext } from "../../fixtures/backend";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";
import {
  fileTreeGeometryIssues,
  type FileTreeGeometryOptions,
  type FileTreeViewportGeometry,
} from "./file-tree-geometry";

export const LARGE_FILE_TREE_FOLDER = "large-file-tree";
export const LARGE_FILE_TREE_COUNT = 600;

export function largeFileTreePath(index: number): string {
  return `${LARGE_FILE_TREE_FOLDER}/entry-${index.toString().padStart(4, "0")}.txt`;
}

/** Wait for a hidden-panel transition to pass through the browser's layout observers. */
export async function waitForFileTreeLayoutSettle(page: Page): Promise<void> {
  await page.evaluate(
    () =>
      new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))),
  );
}

async function readFileTreeViewportGeometry(viewport: Locator): Promise<FileTreeViewportGeometry> {
  return viewport.evaluate((element) => {
    const viewportRect = element.getBoundingClientRect();
    const firstVirtualRow = element.querySelector<HTMLElement>("[data-index]");
    const treeContainer = firstVirtualRow?.parentElement;
    const rows = Array.from(element.querySelectorAll<HTMLElement>("[data-index]"))
      .map((row) => {
        const rect = row.getBoundingClientRect();
        return {
          bottom: rect.bottom,
          path:
            row.querySelector<HTMLElement>('[data-testid="file-tree-node"]')?.dataset.path ?? null,
          top: rect.top,
        };
      })
      .filter((row) => row.bottom > viewportRect.top && row.top < viewportRect.bottom)
      .sort((left, right) => left.top - right.top);
    return {
      bottom: viewportRect.bottom,
      clientHeight: element.clientHeight,
      rows,
      scrollTop: element.scrollTop,
      top: viewportRect.top,
      treeHeight: treeContainer?.getBoundingClientRect().height ?? 0,
    };
  });
}

/** Return the ordered rows currently intersecting the file-tree viewport. */
export async function visibleFileTreePaths(viewport: Locator): Promise<string[]> {
  const geometry = await readFileTreeViewportGeometry(viewport);
  return geometry.rows.flatMap((row) => (row.path === null ? [] : [row.path]));
}

/** Assert that mounted rows cover the visible viewport without blank intervals. */
export async function expectContiguousVisibleFileTreeRows(
  viewport: Locator,
  options: FileTreeGeometryOptions = {},
): Promise<void> {
  const geometry = await readFileTreeViewportGeometry(viewport);
  const issues = fileTreeGeometryIssues(geometry, options);
  expect(issues, issues.join("; ")).toEqual([]);
}

/** Compare an expected ordered visible window after a panel transition. */
export async function expectVisibleFileTreePaths(
  viewport: Locator,
  expectedPaths: readonly string[],
): Promise<void> {
  expect(await visibleFileTreePaths(viewport)).toEqual(expectedPaths);
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
  git.exec("git checkout main");
  for (let index = 0; index < LARGE_FILE_TREE_COUNT; index += 1) {
    git.createFile(largeFileTreePath(index), `large tree entry ${index}\n`);
  }
  git.stageAll();
  if (git.exec("git status --short").trim()) {
    git.commit("seed large file tree");
  }
  git.exec("git push origin main");
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
  await testPage.reload();
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 45_000 });
  return session;
}
