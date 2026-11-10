import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { randomUUID } from "node:crypto";
import { expect, test as base, type SeedData } from "../fixtures/test-base";
import type { BackendContext } from "../fixtures/backend";
import type { ApiClient } from "./api-client";
import type { Page, Route } from "@playwright/test";
import { makeGitEnv } from "./git-helper";
import { watchWs } from "./causal-waits";
import { settledBoundingBox } from "./settled-box";
import { MobileKanbanPage } from "../pages/mobile-kanban-page";

// These scenarios own their data and pages; the standard auto fixture resets shared seeds.
export const test = base.extend({
  integrationCleanup: [
    async ({ backend }, use) => {
      void backend;
      await use();
    },
    { auto: true },
  ],
});

type Inputs = { apiClient: ApiClient; backend: BackendContext; seedData: SeedData };
type Owned = {
  profileId: string;
  workspaceId: string;
  workflowId: string;
  taskId: string;
  steps: string[];
  root: string;
};
const checkout = path.resolve(__dirname, "../../../..");
function git(cwd: string, args: string[], env = process.env) {
  return execFileSync("git", args, { cwd, env, encoding: "utf8" });
}
function fingerprint(cwd: string) {
  return {
    head: git(cwd, ["rev-parse", "HEAD"]),
    status: git(cwd, ["status", "--porcelain=v1", "--untracked-files=all", "-z"]),
  };
}

export async function withRollbackFixture<T>(
  { apiClient, backend, seedData }: Inputs,
  run: (owned: Owned) => Promise<T>,
): Promise<T> {
  const sourceBefore = fingerprint(checkout);
  const seedBefore = fingerprint(seedData.repositoryPath);
  const { settings } = await apiClient.getUserSettings();
  const root = fs.mkdtempSync(path.join(backend.tmpDir, "rollback-"));
  const repoPath = path.join(root, "repo");
  const origin = path.join(root, "origin.git");
  const env = makeGitEnv(root);
  let workspace: { id: string; name: string } | undefined;
  let executorId: string | undefined;
  let profileId: string | undefined;
  let ownedRepoBefore: ReturnType<typeof fingerprint> | undefined;
  try {
    fs.mkdirSync(repoPath);
    git(root, ["init", "--bare", "-b", "main", origin], env);
    git(repoPath, ["init", "-b", "main"], env);
    fs.writeFileSync(path.join(repoPath, "fixture.txt"), "Disposable rollback fixture\n");
    git(repoPath, ["add", "fixture.txt"], env);
    git(repoPath, ["commit", "-m", "test: seed rollback fixture"], env);
    git(repoPath, ["remote", "add", "origin", `file://${origin}`], env);
    git(repoPath, ["push", "origin", "main"], env);
    ownedRepoBefore = fingerprint(repoPath);
    const name = `Rollback ${randomUUID()}`;
    workspace = await apiClient.createWorkspace(name);
    const workflow = await apiClient.createWorkflow(workspace.id, name);
    const steps: string[] = [];
    for (const [index, title] of ["Source", "Optimistic", "Authoritative"].entries()) {
      const step = await apiClient.createWorkflowStep(workflow.id, title, index, {
        is_start_step: index === 0,
      });
      steps.push(step.id);
    }
    const repo = await apiClient.createRepository(workspace.id, repoPath);
    const executor = await apiClient.createExecutor(name, "worktree");
    executorId = executor.id;
    const profile = await apiClient.createExecutorProfile(executor.id, name);
    profileId = profile.id;
    const task = await apiClient.createTask(workspace.id, "Rollback original", {
      workflow_id: workflow.id,
      workflow_step_id: steps[0],
      repository_ids: [repo.id],
      executor_profile_id: profile.id,
    });
    await apiClient.saveUserSettings({
      workspace_id: workspace.id,
      workflow_filter_id: workflow.id,
    });
    return await run({
      profileId: profile.id,
      workspaceId: workspace.id,
      workflowId: workflow.id,
      taskId: task.id,
      steps,
      root,
    });
  } finally {
    const cleanupResults = await Promise.allSettled([
      apiClient.saveUserSettings({
        workspace_id: settings.workspace_id,
        workflow_filter_id: settings.workflow_filter_id,
      }),
      workspace ? apiClient.deleteWorkspace(workspace.id, workspace.name) : Promise.resolve(),
    ]);
    const profileCleanup = profileId
      ? await Promise.allSettled([apiClient.deleteExecutorProfile(profileId)])
      : [];
    const executorCleanup = executorId
      ? await Promise.allSettled([apiClient.deleteExecutor(executorId)])
      : [];
    const [ownedRepoAfter] = await Promise.allSettled([
      Promise.resolve().then(() => (ownedRepoBefore ? fingerprint(repoPath) : undefined)),
    ]);
    fs.rmSync(root, { recursive: true, force: true });
    if (ownedRepoAfter.status === "rejected") throw ownedRepoAfter.reason;
    expect(ownedRepoAfter.value).toEqual(ownedRepoBefore);
    expect(fs.existsSync(root)).toBe(false);
    expect(fingerprint(seedData.repositoryPath)).toEqual(seedBefore);
    expect(fingerprint(checkout)).toEqual(sourceBefore);
    for (const result of [...cleanupResults, ...profileCleanup, ...executorCleanup]) {
      if (result.status === "rejected") throw result.reason;
    }
  }
}

async function showStep(page: Page, stepIndex: number, mobile: boolean) {
  if (!mobile) return;
  await page.getByTestId("mobile-board-navigator").tap();
  await page.getByTestId(`column-tab-${stepIndex}`).tap();
  await expect(page.getByTestId("mobile-board-navigator-drawer")).toHaveCount(0);
}

async function startMove(page: Page, owned: Owned, mobile: boolean, targetIndex = 1) {
  const card = page.getByTestId(`task-card-${owned.taskId}`);
  await expect(card).toHaveCount(1);
  await expect(card).toBeVisible();
  if (mobile) {
    await card.getByRole("button", { name: "More options" }).tap();
    await page.getByTestId("task-context-move-to").tap();
    await page.getByTestId(`task-context-step-${owned.steps[targetIndex]}`).tap();
    return;
  }
  const from = await settledBoundingBox(card);
  const x = from.x + from.width / 2;
  const y = from.y + from.height / 2;
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.mouse.move(x + 12, y, { steps: 3 });
  const to = await settledBoundingBox(
    page
      .getByTestId(`kanban-column-${owned.steps[targetIndex]}`)
      .getByRole("heading", { level: 2 }),
  );
  await page.mouse.move(to.x + to.width / 2, to.y + to.height / 2, { steps: 12 });
  await page.mouse.up();
}

export async function proveRollback(
  inputs: Inputs & { page: Page },
  mobile: boolean,
  sameTarget: boolean,
) {
  const { page, backend, apiClient } = inputs;
  await withRollbackFixture(inputs, async (owned) => {
    const ws = watchWs(page);
    await page.addInitScript(
      ({ port }) => {
        localStorage.setItem("kandev.onboarding.completed", "true");
        window.__KANDEV_API_PORT = port;
      },
      { port: String(backend.port) },
    );
    await page.goto(`${backend.frontendUrl}/?workflowId=${owned.workflowId}`);
    await expect(page.getByTestId("kanban-board")).toBeVisible();
    if (mobile) await expect(new MobileKanbanPage(page).mobileKanbanLayout()).toBeVisible();
    const movePath = `**/api/v1/tasks/${owned.taskId}/move`;
    let held: Route | undefined;
    const routeHandler = async (route: Route) => {
      held = route;
    };
    await page.route(movePath, routeHandler);
    try {
      await startMove(page, owned, mobile);
      await expect.poll(() => Boolean(held)).toBe(true);
      expect(held!.request().postDataJSON()).toEqual({
        workflow_id: owned.workflowId,
        workflow_step_id: owned.steps[1],
      });
      await showStep(page, 1, mobile);
      await expect(
        page
          .getByTestId(`kanban-column-${owned.steps[1]}`)
          .getByTestId(`task-card-${owned.taskId}`),
      ).toBeVisible();
      const authoritativeIndex = sameTarget ? 1 : 2;
      const authoritativeStep = owned.steps[authoritativeIndex];
      const rename = await apiClient.rawRequest("PATCH", `/api/v1/tasks/${owned.taskId}`, {
        title: "Remote title",
      });
      expect(rename.ok).toBe(true);
      const authoritativeUpdate = ws.waitForEvent("task.updated", {
        where: (payload) =>
          payload.task_id === owned.taskId &&
          payload.workflow_step_id === authoritativeStep &&
          payload.title === "Remote title",
      });
      await apiClient.moveTask(owned.taskId, owned.workflowId, authoritativeStep);
      await authoritativeUpdate;
      await showStep(page, authoritativeIndex, mobile);
      const authoritativeCard = page
        .getByTestId(`kanban-column-${authoritativeStep}`)
        .getByTestId(`task-card-${owned.taskId}`);
      await expect(authoritativeCard).toContainText("Remote title");
      const errorResponse = page.waitForResponse(
        (response) =>
          response.url().endsWith(`/tasks/${owned.taskId}/move`) && response.status() === 500,
      );
      await held!.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ error: "Old move rejected" }),
      });
      held = undefined;
      await errorResponse;
      await expect(
        page.getByText("The task could not be moved. Try again.", { exact: true }),
      ).toBeVisible();
      const dismiss = page.getByRole("button", { name: "Dismiss", exact: true });
      if (mobile) await dismiss.tap();
      else await dismiss.click();
      await expect(page.getByRole("alertdialog")).toHaveCount(0);
      await expect(authoritativeCard).toContainText("Remote title");
      await expect(authoritativeCard).toBeInViewport();
      const persisted = await apiClient.getTask(owned.taskId);
      expect(persisted.workflow_step_id).toBe(authoritativeStep);
      expect(persisted.title).toBe("Remote title");
      await expect(
        page
          .getByTestId(`kanban-column-${owned.steps[0]}`)
          .getByTestId(`task-card-${owned.taskId}`),
      ).toHaveCount(0);
      if (mobile)
        await page.screenshot({
          path: test.info().outputPath("phone-authoritative-placement.png"),
        });
      if (mobile)
        expect(
          await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
        ).toBe(true);
    } finally {
      if (held) await held.fulfill({ status: 500, contentType: "application/json", body: "{}" });
      await page.unroute(movePath, routeHandler);
      await page.goto("about:blank");
    }
  });
}

export async function proveRejectedMoveChain(
  inputs: Inputs & { page: Page },
  mobile: boolean,
  newerFirst: boolean,
) {
  const { page, backend, apiClient } = inputs;
  await withRollbackFixture(inputs, async (owned) => {
    await page.addInitScript(
      ({ port }) => {
        localStorage.setItem("kandev.onboarding.completed", "true");
        window.__KANDEV_API_PORT = port;
      },
      { port: String(backend.port) },
    );
    await page.goto(`${backend.frontendUrl}/?workflowId=${owned.workflowId}`);
    await expect(page.getByTestId("kanban-board")).toBeVisible();
    const held: Array<Route | undefined> = [];
    const movePath = `**/api/v1/tasks/${owned.taskId}/move`;
    const routeHandler = async (route: Route) => {
      held.push(route);
    };
    await page.route(movePath, routeHandler);
    try {
      await startMove(page, owned, mobile);
      await expect.poll(() => held.length).toBe(1);
      await showStep(page, 1, mobile);
      await expect(
        page
          .getByTestId(`kanban-column-${owned.steps[1]}`)
          .getByTestId(`task-card-${owned.taskId}`),
      ).toBeVisible();
      await startMove(page, owned, mobile, 2);
      await expect.poll(() => held.length).toBe(2);
      for (const [index, route] of held.entries())
        expect(route!.request().postDataJSON()).toEqual({
          workflow_id: owned.workflowId,
          workflow_step_id: owned.steps[index + 1],
        });
      for (const index of newerFirst ? [1, 0] : [0, 1]) {
        const route = held[index]!;
        const failed = page.waitForResponse(
          (response) =>
            response.url().endsWith(`/tasks/${owned.taskId}/move`) &&
            response.status() === 500 &&
            response.request().postDataJSON().workflow_step_id === owned.steps[index + 1],
        );
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ error: "Move rejected" }),
        });
        held[index] = undefined;
        await failed;
        await expect(page.getByRole("alertdialog")).toBeVisible();
      }
      const dismiss = page.getByRole("button", { name: "Dismiss", exact: true });
      if (mobile) await dismiss.tap();
      else await dismiss.click();
      await expect(page.getByRole("alertdialog")).toHaveCount(0);
      await showStep(page, 0, mobile);
      const restored = page
        .getByTestId(`kanban-column-${owned.steps[0]}`)
        .getByTestId(`task-card-${owned.taskId}`);
      await expect(restored).toBeVisible();
      await expect(restored).toBeInViewport();
      expect((await apiClient.getTask(owned.taskId)).workflow_step_id).toBe(owned.steps[0]);
    } catch (error) {
      await page.screenshot({ path: test.info().outputPath("rollback-chain-before-cleanup.png") });
      throw error;
    } finally {
      for (const route of held)
        if (route)
          await route.fulfill({ status: 500, contentType: "application/json", body: "{}" });
      await page.unroute(movePath, routeHandler);
      await page.goto("about:blank");
    }
  });
}
