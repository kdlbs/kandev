import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

import { test, expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";

// Regression: task creation selectors portal out of the clipped
// form subtree, but remain inside the dialog so modal scroll locking still
// permits wheel scrolling.

// Exercises the regular task-create dialog (New Task in the sidebar); run with office off.
useRegularMode();

async function expectWheelScrollsListInsideDialog(testPage: Page) {
  const list = testPage.locator("[cmdk-list]").first();
  await expect(list).toBeVisible();
  await expect
    .poll(() => list.evaluate((el) => Boolean(el.closest('[data-testid="create-task-dialog"]'))))
    .toBe(true);

  const { clientHeight, scrollHeight } = await list.evaluate((el) => ({
    clientHeight: el.clientHeight,
    scrollHeight: el.scrollHeight,
  }));
  expect(scrollHeight).toBeGreaterThan(clientHeight);

  await list.evaluate((el) => {
    el.scrollTop = 0;
  });
  expect(await list.evaluate((el) => el.scrollTop)).toBe(0);

  const box = await list.boundingBox();
  if (!box) throw new Error("selector list has no bounding box");
  await testPage.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await testPage.mouse.wheel(0, 400);

  await expect
    .poll(() => list.evaluate((el) => el.scrollTop), { timeout: 2000 })
    .toBeGreaterThan(0);
}

test.describe("create task selector scroll", () => {
  test("wheel scrolls the repository list portaled inside the dialog", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    // Seed enough repositories so the list overflows max-h-72 (~288px).
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    for (let i = 0; i < 20; i++) {
      const repoDir = path.join(backend.tmpDir, "repos", `scroll-repo-${i}`);
      fs.mkdirSync(repoDir, { recursive: true });
      execSync("git init -b main", { cwd: repoDir, env: gitEnv });
      execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env: gitEnv });
      await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
        name: `scroll-repo-${i}`,
      });
    }

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // Open the repository selector popover via the chip-row trigger
    // (the dialog now uses the multi-repo chip layout, not the legacy
    // single repository-selector combobox).
    await testPage.getByTestId("repo-chip-trigger").first().click();
    await expectWheelScrollsListInsideDialog(testPage);
  });

  test("wheel scrolls the branch list portaled inside the dialog", async ({
    testPage,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    for (let i = 0; i < 20; i++) {
      execSync(`git branch -f scroll-branch-${i} main`, { cwd: repoDir, env: gitEnv });
    }

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    await testPage.getByTestId("repo-chip-trigger").first().click();
    await testPage.getByRole("option", { name: /E2E Repo/ }).click();

    const branchSelector = testPage.getByTestId("branch-chip-trigger").first();
    await expect(branchSelector).toBeEnabled({ timeout: 5_000 });
    await branchSelector.click();
    await expectWheelScrollsListInsideDialog(testPage);
  });

  test("wheel scrolls the agent profile list portaled inside the dialog", async ({
    testPage,
    apiClient,
  }) => {
    const { agents } = await apiClient.listAgents();
    const agentId = agents[0]?.id;
    if (!agentId) throw new Error("no agent available in test fixtures");
    for (let i = 0; i < 20; i++) {
      await apiClient.createAgentProfile(agentId, `Scroll Agent Profile ${i}`, {
        model: "mock-fast",
      });
    }

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    await testPage.getByTestId("agent-profile-selector").click();
    await expectWheelScrollsListInsideDialog(testPage);
  });
});
