import { test, expect } from "../../fixtures/test-base";

test.describe("Configuration Chat on mobile", () => {
  test("launches full-screen and answers a collapsible clarification", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.updateWorkspace(seedData.workspaceId, {
      default_config_agent_profile_id: seedData.agentProfileId,
    });
    await testPage.goto("/settings/agents");
    await testPage.getByRole("button", { name: "Configuration Chat" }).tap();

    const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
    await expect(dialog).toBeVisible({ timeout: 10_000 });
    const viewport = testPage.viewportSize();
    const box = await dialog.boundingBox();
    expect(box).not.toBeNull();
    expect(viewport).not.toBeNull();
    expect(box!.width).toBeGreaterThanOrEqual(viewport!.width * 0.97);
    expect(box!.height).toBeGreaterThanOrEqual(viewport!.height * 0.97);
    await expect(dialog.getByRole("img", { name: "Configuration chat" })).toBeVisible();

    const setup = dialog.getByTestId("config-chat-setup");
    const input = setup.getByPlaceholder("Ask anything about your configuration...");
    await input.fill("/ask-single");
    await setup.getByRole("button", { name: "Start configuration chat" }).tap();

    const clarification = dialog.getByTestId("clarification-overlay-container");
    await expect(clarification).toContainText("Which database", { timeout: 30_000 });
    const collapse = dialog.getByRole("button", { name: "Collapse clarification" });
    await expect(collapse).toBeVisible();
    await collapse.tap();
    await expect(dialog.getByRole("button", { name: "Expand clarification" })).toBeVisible();
    await dialog.getByRole("button", { name: "Expand clarification" }).tap();

    const answer = clarification.getByText("PostgreSQL", { exact: true });
    await expect(answer).toBeVisible();
    const answerBox = await answer.boundingBox();
    expect(answerBox).not.toBeNull();
    expect(answerBox!.x + answerBox!.width).toBeLessThanOrEqual(viewport!.width);
    await answer.tap();
    await expect(clarification).not.toBeVisible({ timeout: 30_000 });
  });
});
