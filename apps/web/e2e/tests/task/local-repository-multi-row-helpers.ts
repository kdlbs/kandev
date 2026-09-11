import path from "node:path";
import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { waitForHttp } from "../../helpers/causal-waits";

export async function exerciseMultiRowCreation(
  page: Page,
  apiClient: ApiClient,
  options: { workspaceId: string; parentPath: string; mobile: boolean },
) {
  const activate = (locator: Locator) => (options.mobile ? locator.tap() : locator.click());
  const name = options.mobile ? "mobile-second-repository" : "desktop-second-repository";
  await page.getByTestId("task-title-input").fill(name);
  await page.getByTestId("task-description-input").fill("/e2e:simple-message");
  const executor = page.getByTestId("executor-profile-selector");
  await activate(executor);
  await activate(page.getByRole("option", { name: /Worktree/i }));
  const originalExecutor = await executor.textContent();
  const firstRow = page.getByTestId("repo-chip").nth(0);
  await expect(firstRow).toHaveAttribute("data-repository-id", /.+/);
  await expect(firstRow.getByTestId("branch-chip-trigger")).toBeEnabled();
  await expect(firstRow.getByTestId("branch-chip-trigger")).not.toHaveText("branch");
  const firstId = await firstRow.getAttribute("data-repository-id");
  expect(firstId).toBeTruthy();
  const firstBranch = await firstRow.getByTestId("branch-chip-trigger").textContent();
  await activate(page.getByTestId("add-repository"));
  const secondRow = page.getByTestId("repo-chip").nth(1);
  await activate(secondRow.getByTestId("repo-chip-trigger"));
  const refresh = page.getByTestId("repo-refresh-button");
  const create = page.getByTestId("create-local-repository-button");
  await expect(refresh).toBeVisible();
  await expect(create).toBeVisible();

  const repositoriesPath = `/api/v1/workspaces/${options.workspaceId}/repositories`;
  let releaseRefresh!: () => void;
  const refreshBarrier = new Promise<void>((resolve) => {
    releaseRefresh = resolve;
  });
  await page.route(`**${repositoriesPath}`, async (route) => {
    if (route.request().method() !== "GET") return route.continue();
    const response = await route.fetch();
    const body = await response.json();
    await refreshBarrier;
    await route.fulfill({
      response,
      json: {
        ...body,
        repositories: [
          ...body.repositories,
          { ...body.repositories[0], id: "refresh-only-option", name: "Refreshed option" },
        ],
      },
    });
  });
  const refreshed = waitForHttp(page, "GET", new RegExp(`${repositoriesPath}$`));
  try {
    await activate(refresh);
    await expect(refresh).toBeDisabled();
    await expect(create).toBeVisible();
    releaseRefresh();
    await refreshed;
    await expect(page.getByRole("option", { name: /Refreshed option/ })).toBeVisible();
  } finally {
    releaseRefresh();
    await refreshed;
    await page.unroute(`**${repositoriesPath}`);
  }
  await expect(firstRow).toHaveAttribute("data-repository-id", firstId!);
  await page.getByPlaceholder("Search repositories...").fill("no-matching-option");
  await expect(refresh).toBeVisible();
  await expect(create).toBeVisible();
  for (const control of [refresh, create]) {
    const box = await control.boundingBox();
    if (options.mobile) {
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.width).toBeGreaterThanOrEqual(44);
    } else {
      expect(box!.height).toBeCloseTo(28, 0);
      expect(box!.width).toBeCloseTo(28, 0);
    }
  }
  await activate(create);
  const surface = page.getByTestId(
    options.mobile ? "create-local-repository-drawer" : "create-local-repository-dialog",
  );
  await expect(surface).toBeVisible();
  await surface.getByRole("textbox", { name: "Repository name" }).fill(name);
  await surface.getByRole("textbox", { name: "Parent directory" }).fill(options.parentPath);
  const initialized = waitForHttp(page, "POST", /\/repositories\/initialize-local$/);
  await activate(surface.getByRole("button", { name: "Create repository" }));
  await initialized;
  await expect(surface).not.toBeVisible();
  await expect(secondRow.getByTestId("repo-chip-trigger")).toContainText(name);
  await expect(secondRow.getByTestId("branch-chip-trigger")).toContainText("main");
  await expect(firstRow).toHaveAttribute("data-repository-id", firstId!);
  await expect(firstRow.getByTestId("branch-chip-trigger")).toHaveText(firstBranch!);
  await expect(executor).toHaveText(originalExecutor!);
  const secondId = await secondRow.getAttribute("data-repository-id");
  await activate(secondRow.getByTestId("repo-chip-trigger"));
  await expect(refresh).toBeVisible();
  await expect(create).toBeVisible();
  await page.keyboard.press("Escape");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
    ),
  ).toBe(false);
  const taskCreated = page.waitForResponse(
    (r) => r.url().endsWith("/api/v1/tasks") && r.request().method() === "POST",
  );
  await activate(page.getByTestId("submit-start-agent"));
  const response = await taskCreated;
  expect(response.ok()).toBe(true);
  const task = await response.json();
  const taskRepositories = (await apiClient.getTask(task.id)).repositories;
  expect(taskRepositories).toHaveLength(2);
  expect(taskRepositories).toEqual(
    expect.arrayContaining([
      expect.objectContaining({ repository_id: firstId }),
      expect.objectContaining({ repository_id: secondId, base_branch: "main" }),
    ]),
  );
  await expect
    .poll(() => apiClient.getTaskEnvironment(task.id))
    .toMatchObject({ executor_type: "worktree" });
  const repositories = await apiClient.rawRequest("GET", repositoriesPath);
  expect((await repositories.json()).repositories).toEqual(
    expect.arrayContaining([
      expect.objectContaining({ id: secondId, local_path: path.join(options.parentPath, name) }),
    ]),
  );
}
