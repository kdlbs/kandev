import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import type { Locator, Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { useRegularMode } from "../../helpers/regular-mode";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

useRegularMode();

type RepositoryRecord = { id: string; local_path: string; default_branch: string };

async function persistedRepository(
  apiClient: ApiClient,
  workspaceId: string,
  repositoryPath: string,
): Promise<RepositoryRecord | undefined> {
  const response = await apiClient.rawRequest(
    "GET",
    `/api/v1/workspaces/${workspaceId}/repositories`,
  );
  expect(response.ok).toBe(true);
  const body = (await response.json()) as { repositories: RepositoryRecord[] };
  return body.repositories.find((repository) => repository.local_path === repositoryPath);
}

async function openCreateTask(page: Page): Promise<void> {
  const mobile = new MobileKanbanPage(page);
  await mobile.goto();
  await mobile.mobileFab.click();
  await expect(page.getByTestId("create-task-dialog")).toBeVisible();
}

async function openCreationDrawer(page: Page): Promise<Locator> {
  const trigger = page.getByTestId("repo-chip-trigger");
  await trigger.click();
  const search = page.getByPlaceholder("Search repositories...");
  const refresh = page.getByTestId("repo-refresh-button");
  const action = page.getByTestId("create-local-repository-button");
  await expect(search).toBeVisible();
  await expect(refresh).toBeVisible();
  await expect(action).toBeVisible();
  const searchControl = search.locator("..");
  const [searchControlBox, refreshBox, actionBox] = await Promise.all([
    searchControl.boundingBox(),
    refresh.boundingBox(),
    action.boundingBox(),
  ]);
  expect(searchControlBox).not.toBeNull();
  expect(refreshBox).not.toBeNull();
  expect(actionBox).not.toBeNull();
  expect(searchControlBox!.width).toBeGreaterThan(refreshBox!.width + actionBox!.width);
  const refreshGap = refreshBox!.x - (searchControlBox!.x + searchControlBox!.width);
  expect(refreshGap).toBeGreaterThanOrEqual(0);
  expect(refreshGap).toBeLessThanOrEqual(8);
  expect(refreshBox!.height).toBeGreaterThanOrEqual(44);
  expect(actionBox!.height).toBeGreaterThanOrEqual(44);
  await action.click();
  const drawer = page.getByTestId("create-local-repository-drawer");
  await expect(drawer).toBeVisible();
  return drawer;
}

async function expectDrawerGeometry(page: Page, drawer: Locator): Promise<void> {
  const viewport = page.viewportSize();
  expect(viewport).not.toBeNull();
  await expect
    .poll(async () => {
      const box = await drawer.boundingBox();
      return !!box && box.y >= 0 && box.y + box.height <= viewport!.height;
    })
    .toBe(true);
  const drawerBox = await drawer.boundingBox();
  expect(drawerBox).not.toBeNull();
  expect(drawerBox!.x).toBeGreaterThanOrEqual(0);
  expect(drawerBox!.x + drawerBox!.width).toBeLessThanOrEqual(viewport!.width);
  expect(drawerBox!.y).toBeGreaterThanOrEqual(0);
  expect(drawerBox!.y + drawerBox!.height).toBeLessThanOrEqual(viewport!.height);

  const scrollOwners = await drawer.locator("*").evaluateAll((elements) =>
    elements
      .filter((element) => ["auto", "scroll"].includes(getComputedStyle(element).overflowY))
      .map((element) => ({
        clientHeight: element.clientHeight,
        scrollHeight: element.scrollHeight,
      })),
  );
  expect(scrollOwners).toHaveLength(1);
  expect(scrollOwners[0].scrollHeight).toBeGreaterThan(scrollOwners[0].clientHeight);

  const entryBox = await drawer.getByTestId("folder-picker-entry").first().boundingBox();
  const createButton = drawer.getByRole("button", { name: "Create repository" });
  const [createBox, footerBox] = await Promise.all([
    createButton.boundingBox(),
    createButton.locator("..").boundingBox(),
  ]);
  expect(entryBox).not.toBeNull();
  expect(createBox).not.toBeNull();
  expect(footerBox).not.toBeNull();
  expect(entryBox!.height).toBeGreaterThanOrEqual(44);
  expect(createBox!.height).toBeGreaterThanOrEqual(44);
  expect(footerBox!.y + footerBox!.height).toBeLessThanOrEqual(drawerBox!.y + drawerBox!.height);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
    ),
  ).toBe(false);
}

function expectUnbornMainRepository(repositoryPath: string): void {
  const symbolicRef = spawnSync("git", ["symbolic-ref", "--short", "HEAD"], {
    cwd: repositoryPath,
    encoding: "utf8",
  });
  expect(symbolicRef.error).toBeUndefined();
  expect(symbolicRef.status).toBe(0);
  expect(String(symbolicRef.stdout).trim()).toBe("main");

  const head = spawnSync("git", ["rev-parse", "--verify", "HEAD"], { cwd: repositoryPath });
  expect(head.error).toBeUndefined();
  expect(head.status).not.toBe(0);
}

test.describe("Create task with a new local repository on mobile", () => {
  test("creates and selects the repository in a contained, scrollable drawer", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repositoryName = "mobile-unborn-main";
    const parentName = "mobile-created-parent";
    const parentPath = path.join(backend.tmpDir, parentName);
    const repositoryPath = path.join(parentPath, repositoryName);
    for (let index = 0; index < 18; index += 1) {
      fs.mkdirSync(path.join(backend.tmpDir, `mobile-parent-${String(index).padStart(2, "0")}`));
    }
    const { executors } = await apiClient.listExecutors();
    const directExecutor = executors.find((executor) =>
      ["local", "local_pc"].includes(executor.type),
    );
    const directProfile = directExecutor?.profiles?.[0];
    expect(
      directProfile,
      "a direct local executor profile is required by the fixture",
    ).toBeDefined();

    await openCreateTask(testPage);
    await testPage.getByTestId("task-title-input").fill("Mobile task on a new repository");
    await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");
    let drawer = await openCreationDrawer(testPage);
    await expect(testPage.getByRole("textbox", { name: "Repository name" })).toBeFocused();
    await testPage.getByRole("textbox", { name: "Repository name" }).fill("dismissed-name");
    await testPage.keyboard.press("Escape");
    await expect(drawer).not.toBeVisible();
    await expect(testPage.getByTestId("repo-chip-trigger")).toBeFocused();
    expect(fs.existsSync(path.join(backend.tmpDir, "dismissed-name"))).toBe(false);
    await expect(testPage.getByTestId("task-title-input")).toHaveValue(
      "Mobile task on a new repository",
    );

    drawer = await openCreationDrawer(testPage);
    await expectDrawerGeometry(testPage, drawer);
    const parentInput = drawer.getByRole("textbox", { name: "Parent directory" });
    await parentInput.fill(backend.tmpDir);
    await parentInput.press("Enter");
    await drawer.getByRole("button", { name: "New folder" }).click();
    await drawer.getByRole("textbox", { name: "New folder name" }).fill(parentName);
    await drawer.getByRole("button", { name: "Create folder" }).click();
    await expect(parentInput).toHaveValue(parentPath);
    const nameInput = testPage.getByRole("textbox", { name: "Repository name" });
    await nameInput.fill(repositoryName);
    await expect(testPage.getByTitle(repositoryPath)).toBeVisible();
    await drawer.getByRole("button", { name: "Create repository" }).click();
    await expect(drawer).not.toBeVisible();

    await expect(testPage.getByTestId("repo-chip-trigger")).toContainText(repositoryName);
    await expect(testPage.getByTestId("branch-chip-trigger").first()).toContainText("main");
    await expect(testPage.getByTestId("executor-profile-selector")).toContainText(
      directExecutor!.name,
    );
    await expect(testPage.getByTestId("submit-start-agent")).toBeEnabled({ timeout: 30_000 });
    const taskResponsePromise = testPage.waitForResponse(
      (response) =>
        response.url().endsWith("/api/v1/tasks") && response.request().method() === "POST",
    );
    await testPage.getByTestId("submit-start-agent").click();
    const taskResponse = await taskResponsePromise;
    expect(taskResponse.ok()).toBe(true);
    const createdTask = (await taskResponse.json()) as { id: string };
    await expect(testPage.getByTestId("create-task-dialog")).not.toBeVisible();

    const repository = await persistedRepository(apiClient, seedData.workspaceId, repositoryPath);
    expect(repository).toMatchObject({ local_path: repositoryPath, default_branch: "main" });
    const taskId = createdTask.id;
    expect((await apiClient.getTask(taskId)).repositories).toEqual(
      expect.arrayContaining([expect.objectContaining({ repository_id: repository!.id })]),
    );
    await expect
      .poll(async () => await apiClient.getTaskEnvironment(taskId), { timeout: 20_000 })
      .toMatchObject({
        executor_type: expect.stringMatching(/^(local|local_pc)$/),
        executor_profile_id: directProfile!.id,
      });
    expect(fs.statSync(path.join(repositoryPath, ".git")).isDirectory()).toBe(true);
    expectUnbornMainRepository(repositoryPath);
  });
});
