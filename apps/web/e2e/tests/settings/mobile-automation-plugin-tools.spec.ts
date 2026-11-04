import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  installFixturePlugin,
  PLUGIN_ID,
  uninstallFixturePlugin,
} from "../../helpers/plugin-fixture";

async function saveChanges(testPage: Parameters<typeof installFixturePlugin>[0]): Promise<void> {
  const saveBar = testPage.getByTestId("settings-floating-save");
  await saveBar.getByRole("button", { name: "Save changes" }).click();
  await expect(saveBar).toHaveAttribute("data-status", "saved", { timeout: 15_000 });
}

async function configureFixturePlugin(
  testPage: Parameters<typeof installFixturePlugin>[0],
): Promise<void> {
  await installFixturePlugin(testPage);
  await testPage.goto(`/settings/plugins/${PLUGIN_ID}`);
  const tokenInput = testPage.getByTestId("plugin-config-field-api_token").locator("input");
  await tokenInput.fill("automation-plugin-tools-mobile");
  await saveChanges(testPage);
  await expect(tokenInput).toHaveValue("********", { timeout: 15_000 });
}

test.describe("Mobile automation plugin tools", () => {
  test.afterEach(async ({ apiClient }) => {
    await uninstallFixturePlugin(apiClient);
  });

  test("selects, saves, reviews, and clears tools with 44-pixel rows", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await configureFixturePlugin(testPage);
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Mobile plugin tools",
      repositoryMode: "none",
    });
    await testPage.goto(
      `/settings/workspaces/${seedData.workspaceId}/automations/${automation.id}`,
    );
    await expect(testPage.getByTestId("automation-editor")).toBeVisible({ timeout: 15_000 });

    const readRow = testPage.getByTestId(`plugin-tool-${PLUGIN_ID}-list_findings`);
    const writeRow = testPage.getByTestId(`plugin-tool-${PLUGIN_ID}-complete_report`);
    await expect(readRow).toBeVisible();
    for (const row of [readRow, writeRow]) {
      const box = await row.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
    }
    await assertNoDocumentHorizontalOverflow(testPage, "mobile automation plugin tools");

    await readRow.tap();
    await writeRow.tap();
    await saveChanges(testPage);
    await testPage.reload();
    await expect(
      testPage.getByTestId(`plugin-tool-${PLUGIN_ID}-list_findings`).getByRole("checkbox"),
    ).toBeChecked({ timeout: 15_000 });
    await expect(
      testPage.getByTestId(`plugin-tool-${PLUGIN_ID}-complete_report`).getByRole("checkbox"),
    ).toBeChecked();

    await testPage.getByTestId(`plugin-tools-clear-${PLUGIN_ID}`).tap();
    await saveChanges(testPage);
    await testPage.reload();
    await expect(
      testPage.getByTestId(`plugin-tool-${PLUGIN_ID}-list_findings`).getByRole("checkbox"),
    ).not.toBeChecked({ timeout: 15_000 });
    await expect(
      testPage.getByTestId(`plugin-tool-${PLUGIN_ID}-complete_report`).getByRole("checkbox"),
    ).not.toBeChecked();
    await assertNoDocumentHorizontalOverflow(testPage, "mobile automation plugin tools cleared");
  });
});
