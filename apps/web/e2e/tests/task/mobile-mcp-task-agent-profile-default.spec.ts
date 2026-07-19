import { test, expect } from "../../fixtures/test-base";

test.describe("MCP-created task agent profile default on mobile", () => {
  test("Task Actions choice is touch-usable, viewport-safe, and persists", async ({
    testPage,
    apiClient,
  }) => {
    await testPage.goto("/settings/general");
    const taskActionsLink = testPage.getByRole("link", { name: /Task Actions/ });
    await expect(taskActionsLink).toBeVisible({ timeout: 15_000 });
    await taskActionsLink.tap();

    await expect(testPage).toHaveURL(/\/settings\/general\/task-actions$/);
    await expect(
      testPage.getByRole("heading", { name: "Task Actions", exact: true }),
    ).toBeVisible();
    await expect(
      testPage.getByText(/when an agent creates another task without choosing a profile/i),
    ).toBeVisible();

    const currentTask = testPage.getByRole("radio", { name: "Current task profile" });
    const workspaceDefault = testPage.getByRole("radio", {
      name: "Workspace default profile",
    });
    await expect(currentTask).toBeChecked();

    const choice = testPage.locator('label[for="mcp-task-profile-workspace_default"]');
    const card = testPage
      .locator('[data-slot="card"]')
      .filter({ hasText: "Profile for Tasks Created by Agents" });
    const [choiceBox, cardBox, viewport] = await Promise.all([
      choice.boundingBox(),
      card.boundingBox(),
      testPage.evaluate(() => ({ width: window.innerWidth, height: window.innerHeight })),
    ]);
    expect(choiceBox).not.toBeNull();
    expect(cardBox).not.toBeNull();
    expect(choiceBox!.height).toBeGreaterThanOrEqual(44);
    expect(choiceBox!.x).toBeGreaterThanOrEqual(0);
    expect(choiceBox!.x + choiceBox!.width).toBeLessThanOrEqual(viewport.width);
    expect(cardBox!.x).toBeGreaterThanOrEqual(0);
    expect(cardBox!.x + cardBox!.width).toBeLessThanOrEqual(viewport.width);
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);

    await choice.tap();
    await expect(workspaceDefault).toBeChecked();
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.mcp_task_agent_profile_default)
      .toBe("current_task");
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .tap();
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.mcp_task_agent_profile_default)
      .toBe("workspace_default");

    await testPage.reload();
    await expect(workspaceDefault).toBeChecked();
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
  });
});
