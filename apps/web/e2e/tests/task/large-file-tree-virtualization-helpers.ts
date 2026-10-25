import { randomUUID } from "node:crypto";
import path from "node:path";
import { expect, type Locator } from "@playwright/test";
import type { Page } from "@playwright/test";
import { resetSeedRepositoryCheckout, type SeedData } from "../../fixtures/test-base";
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

export function seedLargeFileTree(backend: BackendContext, branch: string): void {
  const git = new GitHelper(
    path.join(backend.tmpDir, "repos", "e2e-repo"),
    makeGitEnv(backend.tmpDir),
  );
  git.exec("git checkout main");
  git.exec("git pull --ff-only origin main");
  git.exec(`git checkout -b ${branch} origin/main`);
  try {
    for (let index = 0; index < LARGE_FILE_TREE_COUNT; index += 1) {
      git.createFile(largeFileTreePath(index), `large tree entry ${index}\n`);
    }
    git.stageAll();
    if (git.exec("git status --short").trim()) {
      git.commit("seed large file tree");
    }
    git.exec(`git push origin ${branch}`);
  } finally {
    git.exec("git checkout main");
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
  // Earlier specs can leave unpublished commits in the worker-shared checkout.
  // Start from the immutable fixture baseline before fast-forwarding its origin.
  resetSeedRepositoryCheckout(seedData, backend.tmpDir);
  const branch = `e2e-large-file-tree-${randomUUID()}`;
  seedLargeFileTree(backend, branch);
  const response = await apiClient.rawRequest(
    "GET",
    `/api/v1/repositories/${seedData.repositoryId}`,
  );
  if (!response.ok) throw new Error("could not read repository before large-tree fixture");
  const repository = (await response.json()) as { default_branch: string };
  await apiClient.updateRepository(seedData.repositoryId, { default_branch: branch });
  try {
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
  } finally {
    await apiClient.updateRepository(seedData.repositoryId, {
      default_branch: repository.default_branch,
    });
  }
}
