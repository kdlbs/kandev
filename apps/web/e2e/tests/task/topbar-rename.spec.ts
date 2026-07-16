import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("Task topbar inline rename", () => {
  test("renames the task by double-clicking the title and pressing Enter", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Topbar rename original",
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
    await expect(title).toHaveText("Topbar rename original", { timeout: 10_000 });

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
