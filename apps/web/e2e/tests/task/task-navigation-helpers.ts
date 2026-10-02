import { randomUUID } from "node:crypto";
import path from "node:path";
import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { BackendContext } from "../../fixtures/backend";
import type { ApiClient } from "../../helpers/api-client";
import { GitHelper, makeGitEnv, createStandardProfile } from "../../helpers/git-helper";
import {
  routeNavigationResponses,
  type NavigationResponseGate,
} from "../../helpers/navigation-response-hold";
import { SessionPage } from "../../pages/session-page";
import { waitForSessionDone, waitForSessionGitHydration } from "../../helpers/session";
import type { AppState } from "../../../lib/state/store";
import type { StoreApi } from "zustand";

export const AVAILABLE = "navigation-available";
export const HELD = "navigation-held";
export const ROOT_FILE = "navigation-root.ts";

async function waitForTreeResponse(
  gate: NavigationResponseGate,
  requestOffset: number,
  sessionId: string,
  path: string,
) {
  await expect
    .poll(
      () =>
        gate.requests
          .slice(requestOffset)
          .some(
            (request) =>
              request.action === "workspace.tree.get" &&
              request.payload.session_id === sessionId &&
              request.payload.path === path &&
              request.deliveredAt !== undefined,
          ),
      { timeout: 15_000, intervals: [100, 250, 500] },
    )
    .toBe(true);
}

export function seedNavigationBranch(backend: BackendContext) {
  const git = new GitHelper(
    path.join(backend.tmpDir, "repos", "e2e-repo"),
    makeGitEnv(backend.tmpDir),
  );
  const branch = `e2e-navigation-${randomUUID()}`;
  const worktreePath = path.join(backend.tmpDir, branch);
  git.exec("git fetch --no-tags origin main");
  git.exec(`git worktree add -b ${branch} "${worktreePath}" origin/main`);
  const branchGit = new GitHelper(worktreePath, makeGitEnv(backend.tmpDir));
  try {
    for (const file of [ROOT_FILE, `${AVAILABLE}/available.ts`, `${HELD}/held.ts`])
      branchGit.createFile(file, "export const navigationFixture = true;\n");
    branchGit.stageAll();
    if (branchGit.exec("git status --short").trim())
      branchGit.commit("seed navigation responsiveness");
    branchGit.exec(`git push origin ${branch}`);
  } finally {
    git.exec(`git worktree remove --force "${worktreePath}"`);
  }
  return branch;
}

export async function seedNavigationTasks(
  api: ApiClient,
  seed: SeedData,
  backend: BackendContext,
  executorProfileId?: string,
) {
  const branch = seedNavigationBranch(backend);
  const profile = await createStandardProfile(api, "navigation-responsiveness");
  const tasks = [];
  for (const suffix of ["A", "B"]) {
    const task = await api.createTaskWithAgent(
      seed.workspaceId,
      `Navigation ${suffix}`,
      profile.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seed.workflowId,
        workflow_step_id: seed.startStepId,
        repositories: [{ repository_id: seed.repositoryId, base_branch: branch }],
        executor_profile_id: executorProfileId ?? seed.worktreeExecutorProfileId,
      },
    );
    tasks.push(task);
    // Local executor sessions share the checkout's Git index.
    await waitForSessionDone(
      api,
      task.id,
      task.session_id!,
      "Navigation fixture turn settled",
      30_000,
    );
  }
  return tasks;
}

export async function exposeNavigationStore(page: Page) {
  await page.addInitScript(() => {
    (window as Window & { __KANDEV_E2E_EXPOSE_STORE__?: boolean }).__KANDEV_E2E_EXPOSE_STORE__ =
      true;
  });
}

export async function saveExpandedPaths(
  page: Page,
  sessionId: string,
  paths: string[],
  mobile = false,
) {
  await page.evaluate(
    ({ sessionId, paths, mobile }) => {
      const state = (
        window as Window & { __KANDEV_E2E_STORE__: StoreApi<AppState> }
      ).__KANDEV_E2E_STORE__.getState();
      const env = mobile ? sessionId : (state.environmentIdBySessionId[sessionId] ?? sessionId);
      const count = state.sessionWorktreesBySessionId.itemsBySessionId[sessionId]?.length ?? 0;
      const refresh = state.workspaceFilesRefresh.bySessionId[sessionId] ?? 0;
      sessionStorage.setItem(
        `kandev.filesPanel.expanded.${env}:${count}:${refresh}`,
        JSON.stringify(paths),
      );
    },
    { sessionId, paths, mobile },
  );
}

export async function showNavigationFiles(page: Page, mobile: boolean) {
  if (mobile) await page.getByRole("button", { name: "Files", exact: true }).tap();
  else {
    const session = new SessionPage(page);
    await session.waitForDockviewReady();
    await expect(page.getByTestId("dockview-task-layout")).toHaveAttribute("aria-busy", "false");
    await session.showSessionContext();
    await session.clickTab("Files");
  }
}

export async function selectNavigationTask(page: Page, title: string, mobile = false) {
  if (mobile) {
    await page.getByTestId("mobile-task-picker-trigger").tap();
    const sheet = page.getByRole("dialog", { name: "Tasks", exact: true });
    await sheet
      .getByTestId("sidebar-task-item")
      .filter({ has: page.getByText(title, { exact: true }) })
      .tap();
    await expect(sheet).not.toBeVisible();
  } else await new SessionPage(page).sidebarTaskItem(title).click();
}

export async function assertProgressiveNavigation(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  backend: BackendContext,
  mobile: boolean,
) {
  const [a, b] = await seedNavigationTasks(api, seed, backend);
  const gate = await routeNavigationResponses(page);
  const initialRequestOffset = gate.requests.length;
  await exposeNavigationStore(page);
  await page.goto(`/t/${a.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle();
  await waitForSessionGitHydration(page, a.session_id!);
  await showNavigationFiles(page, mobile);
  await waitForTreeResponse(gate, initialRequestOffset, a.session_id!, "");
  await expect(session.fileTreeNode(ROOT_FILE)).toBeVisible();
  await saveExpandedPaths(page, a.session_id!, [AVAILABLE, HELD], mobile);
  gate.hold((r) => r.action === "workspace.tree.get" && r.payload.path === HELD);
  const reloadRequestOffset = gate.requests.length;
  await page.reload();
  await waitForSessionGitHydration(page, a.session_id!);
  await showNavigationFiles(page, mobile);
  await expect.poll(() => gate.heldCount()).toBeGreaterThan(0);
  await waitForTreeResponse(gate, reloadRequestOffset, a.session_id!, "");
  await waitForTreeResponse(gate, reloadRequestOffset, a.session_id!, AVAILABLE);
  // This assertion is deliberately before release: an unfinished sibling cannot hide the root.
  await expect(session.fileTreeNode(ROOT_FILE)).toBeVisible();
  await expect(session.fileTreeNode(`${AVAILABLE}/available.ts`)).toBeVisible();
  gate.release("temporary navigation folder failure");
  const status = page.getByTestId("file-tree-refresh-status");
  await expect(status).toContainText("temporary navigation folder failure");
  await expect(session.fileTreeNode(`${AVAILABLE}/available.ts`)).toBeVisible();
  await showNavigationFiles(page, mobile);
  await expect(status).toBeVisible();
  const retry = status.getByRole("button", { name: "Retry", exact: true });
  const retryRequestOffset = gate.requests.length;
  if (mobile) {
    const box = await retry.boundingBox();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    expect(box!.width).toBeGreaterThanOrEqual(44);
    await retry.tap();
  } else await retry.click();
  await waitForTreeResponse(gate, retryRequestOffset, a.session_id!, HELD);
  await expect(session.fileTreeNode(`${HELD}/held.ts`)).toBeVisible();
  await expect(status).toHaveCount(0);
  const taskBRequestOffset = gate.requests.length;
  await selectNavigationTask(page, b.title, mobile);
  await expect(page).toHaveURL(new RegExp(`/t/${b.id}$`));
  await session.waitForChatIdle();
  await waitForSessionGitHydration(page, b.session_id!);
  await showNavigationFiles(page, mobile);
  await waitForTreeResponse(gate, taskBRequestOffset, b.session_id!, "");
  await expect(session.fileTreeNode(ROOT_FILE)).toBeVisible();
  gate.hold(
    (r) =>
      r.action === "workspace.tree.get" &&
      r.payload.session_id === a.session_id &&
      r.payload.path === "",
  );
  await selectNavigationTask(page, a.title, mobile);
  await expect(page).toHaveURL(new RegExp(`/t/${a.id}$`));
  await showNavigationFiles(page, mobile);
  await expect.poll(() => gate.heldCount()).toBeGreaterThan(0);
  await expect(session.fileTreeNode(`${AVAILABLE}/available.ts`)).toBeVisible();
  if (mobile) {
    await session.fileTreeNodeActions(ROOT_FILE).tap();
    await expect(session.fileTreeTouchMenu()).toBeVisible();
    await page.keyboard.press("Escape");
    await session.fileTreeNode(ROOT_FILE).tap();
    await expect(page.getByTestId("mobile-file-viewer-panel")).toBeVisible();
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
  } else {
    await session.fileTreeNode(ROOT_FILE).dblclick();
    await expect(page.getByTestId("preview-tab-file-editor")).toBeVisible();
  }
  gate.release();
}
