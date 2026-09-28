import { test, expect } from "../../fixtures/test-base";

test.describe("MCP-created task agent profile default on mobile", () => {
  test("Creating and opening tasks choice is touch-usable, viewport-safe, and persists", async ({
    testPage,
    apiClient,
  }) => {
    await testPage.goto("/settings");
    const taskBehaviorLink = testPage.getByRole("link", { name: /Task Behavior/ });
    await expect(taskBehaviorLink).toBeVisible({ timeout: 15_000 });
    await taskBehaviorLink.tap();

    await expect(testPage).toHaveURL(/\/settings\/preferences\/task-behavior$/);
    await expect(testPage.getByTestId("task-behavior-creating-title")).toBeVisible();
    await expect(testPage.getByText("create_task_kandev", { exact: true })).not.toBeVisible();
    await testPage.getByRole("button", { name: "About Profile for Tasks Created by Agents" }).tap();
    const info = testPage.getByRole("dialog", { name: "Profile for Tasks Created by Agents" });
    await expect(info).toContainText("create_task_kandev");
    await expect(info).toContainText("spawn_session_kandev");
    await expect(info).toContainText("effective model, mode, and options");
    await expect(info).toContainText("Workflow-selected profiles win first");
    await expect(info).toContainText(
      "skips the creating session and source or parent task profiles",
    );
    await info.getByRole("button", { name: "Close", exact: true }).tap();
    const currentTask = testPage.getByRole("radio", { name: "Creating session profile" });
    const workspaceDefault = testPage.getByRole("radio", {
      name: "Workspace default profile",
    });
    await expect(currentTask).toBeChecked();

    const choice = testPage.locator('label[for="mcp-task-profile-workspace_default"]');
    const card = testPage
      .locator('[data-settings-group-card="true"]')
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
