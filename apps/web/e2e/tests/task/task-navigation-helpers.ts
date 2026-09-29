import path from "node:path";
import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { BackendContext } from "../../fixtures/backend";
import type { ApiClient } from "../../helpers/api-client";
import { GitHelper, makeGitEnv, createStandardProfile } from "../../helpers/git-helper";
import { routeNavigationResponses } from "../../helpers/navigation-response-hold";
import { SessionPage } from "../../pages/session-page";
import type { AppState } from "../../../lib/state/store";
import type { StoreApi } from "zustand";

export const AVAILABLE = "navigation-available";
export const HELD = "navigation-held";
export const ROOT_FILE = "navigation-root.ts";

export async function seedNavigationTasks(
  api: ApiClient,
  seed: SeedData,
  backend: BackendContext,
  executorProfileId?: string,
) {
  const git = new GitHelper(
    path.join(backend.tmpDir, "repos", "e2e-repo"),
    makeGitEnv(backend.tmpDir),
  );
  for (const file of [ROOT_FILE, `${AVAILABLE}/available.ts`, `${HELD}/held.ts`])
    git.createFile(file, "export const navigationFixture = true;\n");
  git.stageAll();
  if (git.exec("git status --short").trim()) git.commit("seed navigation responsiveness");
  if (executorProfileId) git.exec("git push origin main");
  const profile = await createStandardProfile(api, "navigation-responsiveness");
  const tasks = [];
  for (const suffix of ["A", "B"]) {
    tasks.push(
      await api.createTaskWithAgent(seed.workspaceId, `Navigation ${suffix}`, profile.id, {
        description: "/e2e:simple-message",
        workflow_id: seed.workflowId,
        workflow_step_id: seed.startStepId,
        repository_ids: [seed.repositoryId],
        executor_profile_id: executorProfileId,
      }),
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
  else await new SessionPage(page).clickTab("Files");
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
  await exposeNavigationStore(page);
  await page.goto(`/t/${a.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle();
  await showNavigationFiles(page, mobile);
  await expect(session.fileTreeNode(ROOT_FILE)).toBeVisible();
  await saveExpandedPaths(page, a.session_id!, [AVAILABLE, HELD], mobile);
  gate.hold((r) => r.action === "workspace.tree.get" && r.payload.path === HELD);
  await page.reload();
  await showNavigationFiles(page, mobile);
  await expect.poll(() => gate.heldCount()).toBeGreaterThan(0);
  // This assertion is deliberately before release: an unfinished sibling cannot hide the root.
  await expect(session.fileTreeNode(ROOT_FILE)).toBeVisible();
  await expect(session.fileTreeNode(`${AVAILABLE}/available.ts`)).toBeVisible();
  gate.release("temporary navigation folder failure");
  const status = page.getByTestId("file-tree-refresh-status");
  await expect(status).toContainText("temporary navigation folder failure");
  await expect(session.fileTreeNode(`${AVAILABLE}/available.ts`)).toBeVisible();
  const retry = status.getByRole("button", { name: "Retry", exact: true });
  if (mobile) {
    const box = await retry.boundingBox();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    expect(box!.width).toBeGreaterThanOrEqual(44);
    await retry.tap();
  } else await retry.click();
  await expect(session.fileTreeNode(`${HELD}/held.ts`)).toBeVisible();
  await expect(status).toHaveCount(0);
  await selectNavigationTask(page, b.title, mobile);
  await expect(page).toHaveURL(new RegExp(`/t/${b.id}$`));
  await session.waitForChatIdle();
  await showNavigationFiles(page, mobile);
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
