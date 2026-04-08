import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

// Regression: Combobox popovers (Repository / Base Branch / Agent Profile)
// rendered inside the task creation Dialog were portaled to document.body,
// which placed them outside react-remove-scroll's allowed subtree so mouse
// wheel events were swallowed and the list could not be scrolled. The fix
// renders these popovers inline (portal={false}) so they live inside the
// Dialog content tree.
test.describe("repository selector scroll inside dialog", () => {
  test("wheel scrolls the repository list and the list is inside the dialog", async ({
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

    // Open the repository selector popover.
    await testPage.getByTestId("repository-selector").click();

    // The popover's cmdk list must live inside the dialog DOM (not portaled
    // to body), otherwise react-remove-scroll eats wheel events.
    const listInDialog = dialog.locator("[cmdk-list]");
    await expect(listInDialog).toBeVisible();

    // Sanity: the list is actually overflowing.
    const { clientHeight, scrollHeight } = await listInDialog.evaluate((el) => ({
      clientHeight: el.clientHeight,
      scrollHeight: el.scrollHeight,
    }));
    expect(scrollHeight).toBeGreaterThan(clientHeight);

    // Force the list to the top so we can reliably detect a downward wheel.
    await listInDialog.evaluate((el) => {
      el.scrollTop = 0;
    });
    expect(await listInDialog.evaluate((el) => el.scrollTop)).toBe(0);

    // Dispatch a real wheel event via the mouse at the center of the list.
    const box = await listInDialog.boundingBox();
    if (!box) throw new Error("repository list has no bounding box");
    await testPage.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    await testPage.mouse.wheel(0, 400);

    await expect
      .poll(() => listInDialog.evaluate((el) => el.scrollTop), { timeout: 2000 })
      .toBeGreaterThan(0);
  });
});
