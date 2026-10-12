// Routing: /t/{taskId}. The mobile- prefix selects the mobile-chrome project.
// Verifies the retired built-in Prompt history option is gone from the phone
// Panels sheet while the sheet and the task navigation keep working.
import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import { SessionPage } from "../../pages/session-page";

const PLUGIN_PANEL_OPTION = "mobile-plugin-panel-option-kandev-plugin-e2e-notes";
const TASK_NAV_ENTRIES = [/^Chat$/, /^Plan$/, /Changes$/, /^Files$/, /^Terminal$/];

test.describe("Prompt history removed from the phone Panels sheet", () => {
  test.afterEach(async ({ apiClient }) => uninstallFixturePlugin(apiClient));

  test("lists the installed plugin panel without a Prompt history option", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    // An enabled mobile plugin panel is what keeps the grouped Panels entry
    // available, so the sheet this test inspects is populated.
    await installFixturePlugin(testPage);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile prompt history removed task",
      seedData.agentProfileId,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const panelsButton = testPage.getByRole("button", { name: "Panels", exact: true });
    await expect(panelsButton).toBeVisible({ timeout: 15_000 });
    await panelsButton.tap();

    // The plugin's option proves the sheet rendered its content. The retired
    // core option's test id is gone from the tree, and its label is matched
    // exactly so the fixture plugin's own differently-titled panel (which
    // shares the "Prompt history" prefix) cannot satisfy this check.
    await expect(testPage.getByTestId(PLUGIN_PANEL_OPTION)).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("mobile-prompt-history-option")).toHaveCount(0);
    await expect(testPage.getByRole("button", { name: "Prompt history", exact: true })).toHaveCount(
      0,
    );

    await testPage.keyboard.press("Escape");
    await expect(testPage.getByRole("dialog", { name: "Panels" })).toHaveCount(0);

    for (const label of TASK_NAV_ENTRIES) {
      await expect(testPage.getByRole("button", { name: label })).toBeVisible();
    }
  });
});
