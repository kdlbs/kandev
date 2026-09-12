// Routing: /t/{taskId}. The mobile- prefix selects the mobile-chrome project.
import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";
import { SessionPage } from "../../pages/session-page";

const PANEL_OPTION = "mobile-plugin-panel-option-kandev-plugin-e2e-prompt-history-plugin";

test.describe("Mobile prompt-history fixture", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("pages, previews aliases, and navigates through the Host facade", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await installFixturePlugin(testPage);
    await apiClient.createPrompt("daily-mobile", "Mobile saved prompt preview");

    const task = await apiClient.createTask(seedData.workspaceId, "Mobile prompt history parity", {
      description: "mobile fixture initial prompt",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "IDLE",
      agentProfileId: seedData.agentProfileId,
      repositoryId: seedData.repositoryId,
    });
    for (let index = 0; index < 3; index += 1) {
      await apiClient.seedSessionMessage(sessionId, {
        type: "message",
        content: index === 2 ? "mobile latest @daily-mobile" : `mobile older ${index}`,
        authorType: "user",
        newTurn: true,
        turnStartedAt: `2026-09-07T12:0${index}:00Z`,
        turnCompletedAt: `2026-09-07T12:0${index}:01Z`,
      });
    }

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const panelsButton = testPage.getByRole("button", { name: "Panels" });
    await expect(panelsButton).toBeVisible({ timeout: 15_000 });
    expect((await panelsButton.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await panelsButton.tap();
    const option = testPage.getByTestId(PANEL_OPTION);
    await expect(option).toBeVisible({ timeout: 10_000 });
    expect((await option.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await option.tap();

    const panel = testPage.getByTestId("fixture-prompt-history-panel");
    if (!(await panel.isVisible())) {
      await panelsButton.tap();
      await option.tap();
    }
    await expect(panel).toBeVisible({ timeout: 10_000 });
    await expect(panel).toHaveAttribute("data-presentation", "mobile");
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);

    const loadOlder = panel.getByTestId("fixture-prompt-history-load-more");
    await expect(loadOlder).toBeVisible();
    expect((await loadOlder.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await loadOlder.tap();
    await expect(panel.getByTestId("fixture-prompt-history-row")).toHaveCount(3);

    await panel.getByTestId("fixture-prompt-history-row").first().getByRole("button").first().tap();
    const alias = panel.getByTestId("custom-prompt-mention");
    await expect(alias).toHaveAttribute("data-prompt-name", "daily-mobile");
    expect((await alias.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await alias.tap();
    await expect(testPage.getByText("Mobile saved prompt preview")).toBeVisible();
    await testPage.keyboard.press("Escape");

    const openPrompt = panel.getByRole("button", { name: "Open prompt" });
    expect((await openPrompt.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await openPrompt.tap();
    await expect(session.activeChat().getByText("mobile latest", { exact: false })).toBeVisible();
  });
});
