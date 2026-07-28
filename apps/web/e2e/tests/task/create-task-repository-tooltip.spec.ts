import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { expect, test } from "../../fixtures/test-base";
import { makeGitEnv } from "../../helpers/git-helper";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";

useRegularMode();

test.describe("Create task repository tooltip", () => {
  test("contains a long path and waits for a deliberate re-hover after picker selection", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const longRepoDir = path.join(backend.tmpDir, "repos", "x".repeat(180));
    fs.mkdirSync(longRepoDir, { recursive: true });
    const gitEnv = makeGitEnv(backend.tmpDir);
    execSync("git init -b main", { cwd: longRepoDir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: longRepoDir, env: gitEnv });
    const repository = await apiClient.createRepository(seedData.workspaceId, longRepoDir, "main", {
      name: "Long path repository",
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();

    const dialog = testPage.getByTestId("create-task-dialog");
    const trigger = dialog.getByTestId("repo-chip-trigger").first();
    await trigger.hover();
    await trigger.click();
    await testPage.getByPlaceholder("Search repositories...").fill("Long path repository");
    await trigger.hover();
    await testPage.keyboard.press("ArrowDown");
    await testPage.keyboard.press("Enter");
    await expect(dialog.getByTestId("repo-chip").first()).toHaveAttribute(
      "data-repository-id",
      repository.id,
    );

    const tooltip = testPage.getByRole("tooltip").filter({ hasText: longRepoDir });
    expect(await trigger.evaluate((element) => element.matches(":hover"))).toBe(true);
    await testPage.waitForTimeout(300);
    await expect(tooltip).toBeHidden();

    await testPage.mouse.move(0, 0);
    await trigger.hover();
    await expect(tooltip).toBeVisible();

    const [tooltipBox, viewport] = await Promise.all([
      tooltip.boundingBox(),
      testPage.evaluate(() => ({ width: window.innerWidth, height: window.innerHeight })),
    ]);
    expect(tooltipBox).not.toBeNull();
    expect(tooltipBox!.x).toBeGreaterThanOrEqual(0);
    expect(tooltipBox!.y).toBeGreaterThanOrEqual(0);
    expect(tooltipBox!.x + tooltipBox!.width).toBeLessThanOrEqual(viewport.width);
    expect(tooltipBox!.y + tooltipBox!.height).toBeLessThanOrEqual(viewport.height);
    await assertNoDocumentHorizontalOverflow(testPage, "long repository tooltip");
    await prCapture.screenshot("desktop-long-repository-tooltip", {
      caption:
        "New Task keeps a deliberately long repository path wrapped inside the desktop viewport.",
    });
  });

  test("opens on the first later mouse hover after picker selection", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repositoryDir = path.join(backend.tmpDir, "repos", "mouse-tooltip-repository");
    fs.mkdirSync(repositoryDir, { recursive: true });
    const gitEnv = makeGitEnv(backend.tmpDir);
    execSync("git init -b main", { cwd: repositoryDir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: repositoryDir, env: gitEnv });
    const repository = await apiClient.createRepository(
      seedData.workspaceId,
      repositoryDir,
      "main",
      {
        name: "Mouse tooltip repository",
      },
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();

    const dialog = testPage.getByTestId("create-task-dialog");
    const trigger = dialog.getByTestId("repo-chip-trigger").first();
    await trigger.hover();
    await trigger.click();
    await testPage.getByPlaceholder("Search repositories...").fill("Mouse tooltip repository");
    await testPage.getByRole("option", { name: "Mouse tooltip repository" }).click();
    await expect(dialog.getByTestId("repo-chip").first()).toHaveAttribute(
      "data-repository-id",
      repository.id,
    );

    await testPage.waitForTimeout(300);
    await testPage.mouse.move(0, 0);
    await trigger.hover();
    await expect(testPage.getByRole("tooltip").filter({ hasText: repositoryDir })).toBeVisible();
  });
});
