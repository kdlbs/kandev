import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("Task topbar inline rename", () => {
  test("hints at editing before renaming the task by double-clicking the title", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const originalTitle =
      "Topbar rename original with a deliberately long title that is truncated in the task header";

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      originalTitle,
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

    const title = testPage.locator('[data-testid="task-topbar"] [aria-current="page"]');
    await expect(title).toHaveText(originalTitle, { timeout: 10_000 });

    await title.hover();
    const tooltip = testPage
      .locator('[data-slot="tooltip-content"][data-state]')
      .filter({ hasText: originalTitle });
    await expect(tooltip).toBeVisible();
    await expect(tooltip).toContainText(originalTitle);
    await expect(tooltip).toContainText("Double-click to edit (or press Enter)");

    await title.dblclick();
    const input = testPage.getByTestId("task-title-rename-input");
    await expect(input).toBeVisible();
    await input.fill("Topbar rename updated");
    await input.press("Enter");

    // Title re-renders from the store once the task.updated WS event lands.
    await expect(title).toHaveText("Topbar rename updated", { timeout: 10_000 });

    // Escape cancels without renaming.
    await title.dblclick();
    await input.fill("Should not persist");
    await input.press("Escape");
    await expect(input).not.toBeVisible();
    await expect(title).toHaveText("Topbar rename updated");
  });
});
