import { test, expect } from "../../fixtures/test-base";

function taskSwitcherModifier() {
  return process.platform === "darwin" ? "Meta" : "Control";
}

test.describe("Recent task switcher", () => {
  test("default shortcut stays open while held and switches on modifier release", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const first = await apiClient.createTask(seedData.workspaceId, "Recent Switcher Alpha", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const second = await apiClient.createTask(seedData.workspaceId, "Recent Switcher Beta", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    await testPage.goto(`/t/${first.id}`);
    await expect(testPage.getByText("Recent Switcher Alpha").first()).toBeVisible({
      timeout: 15_000,
    });

    await testPage.goto(`/t/${second.id}`);
    await expect(testPage.getByText("Recent Switcher Beta").first()).toBeVisible({
      timeout: 15_000,
    });

    const modifier = taskSwitcherModifier();
    await testPage.keyboard.down(modifier);
    await testPage.keyboard.press("Space");

    const switcher = testPage.getByTestId("recent-task-switcher");
    await expect(switcher).toBeVisible({ timeout: 5_000 });
    await expect(switcher.getByText("Recent Switcher Beta")).toBeVisible();
    await expect(switcher.getByText("Recent Switcher Alpha")).toBeVisible();
    await expect(switcher.getByTestId("recent-task-switcher-badge-status").first()).toBeVisible();
    await expect(
      switcher
        .getByTestId("recent-task-switcher-badge-repository")
        .filter({ hasText: "E2E Repo" })
        .first(),
    ).toBeVisible();
    await expect(
      switcher
        .getByTestId("recent-task-switcher-badge-workflow")
        .filter({ hasText: "E2E Workflow" })
        .first(),
    ).toBeVisible();

    const previousRow = switcher.getByTestId(`recent-task-switcher-item-${first.id}`);
    const currentRow = switcher.getByTestId(`recent-task-switcher-item-${second.id}`);
    await expect(previousRow).toHaveAttribute("data-selected", "true");

    await testPage.keyboard.press("Space");
    await expect(currentRow).toHaveAttribute("data-selected", "true");

    await testPage.keyboard.press("Space");
    await expect(previousRow).toHaveAttribute("data-selected", "true");

    await testPage.keyboard.up(modifier);
    await expect(switcher).not.toBeVisible({ timeout: 5_000 });
    await expect(testPage).toHaveURL(new RegExp(`/t/${first.id}`), { timeout: 10_000 });
  });

  test("escape cancels the held switcher without routing on release", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const first = await apiClient.createTask(seedData.workspaceId, "Recent Switcher Cancel One", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const second = await apiClient.createTask(seedData.workspaceId, "Recent Switcher Cancel Two", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    await testPage.goto(`/t/${first.id}`);
    await expect(testPage.getByText("Recent Switcher Cancel One").first()).toBeVisible({
      timeout: 15_000,
    });
    await testPage.goto(`/t/${second.id}`);
    await expect(testPage.getByText("Recent Switcher Cancel Two").first()).toBeVisible({
      timeout: 15_000,
    });

    const modifier = taskSwitcherModifier();
    await testPage.keyboard.down(modifier);
    await testPage.keyboard.press("Space");

    const switcher = testPage.getByTestId("recent-task-switcher");
    await expect(switcher).toBeVisible({ timeout: 5_000 });

    await testPage.keyboard.press("Escape");
    await expect(switcher).not.toBeVisible({ timeout: 5_000 });

    await testPage.keyboard.up(modifier);
    await expect(testPage).toHaveURL(new RegExp(`/t/${second.id}`));
  });
});
