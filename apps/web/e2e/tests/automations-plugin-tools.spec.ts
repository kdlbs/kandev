import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../fixtures/test-base";
import { installFixturePlugin, PLUGIN_ID, uninstallFixturePlugin } from "../helpers/plugin-fixture";

const LIST_FINDINGS = `kandev_kandev_plugin_e2e_list_findings`;
const COMPLETE_REPORT = `kandev_kandev_plugin_e2e_complete_report`;

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
  await tokenInput.fill("automation-plugin-tools-e2e");
  await saveChanges(testPage);
  await expect(tokenInput).toHaveValue("********", { timeout: 15_000 });
}

test.describe("Private automation plugin tools", () => {
  test.afterEach(async ({ apiClient }) => {
    await uninstallFixturePlugin(apiClient);
  });

  test("selects exact tools, survives reload, invalidates on config change, and runs a hidden report", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(180_000);
    await configureFixturePlugin(testPage);

    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Plugin report automation",
      prompt: `e2e:mcp:kandev:${LIST_FINDINGS}({})\ne2e:mcp:kandev:${COMPLETE_REPORT}({"report":"nightly findings"})`,
      agentProfileId: seedData.agentProfileId,
      executorProfileId: seedData.worktreeExecutorProfileId,
      repositoryMode: "none",
    });
    await testPage.goto(
      `/settings/workspaces/${seedData.workspaceId}/automations/${automation.id}`,
    );
    await expect(testPage.getByTestId("automation-editor")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("automation-plugin-tools")).toBeVisible();

    const readRow = testPage.getByTestId(`plugin-tool-${PLUGIN_ID}-list_findings`);
    const writeRow = testPage.getByTestId(`plugin-tool-${PLUGIN_ID}-complete_report`);
    await expect(readRow).toBeVisible();
    await expect(writeRow).toBeVisible();
    await expect(readRow.getByRole("checkbox")).not.toBeChecked();

    await readRow.click();
    await writeRow.click();
    await expect(readRow.getByRole("checkbox")).toBeChecked();
    await expect(writeRow.getByRole("checkbox")).toBeChecked();
    await saveChanges(testPage);
    await expect(readRow.getByRole("checkbox")).toBeChecked();

    await testPage.reload();
    await expect(testPage.getByTestId("automation-plugin-tools")).toBeVisible({ timeout: 15_000 });
    await expect(
      testPage.getByTestId(`plugin-tool-${PLUGIN_ID}-list_findings`).getByRole("checkbox"),
    ).toBeChecked();
    await expect(
      testPage.getByTestId(`plugin-tool-${PLUGIN_ID}-complete_report`).getByRole("checkbox"),
    ).toBeChecked();

    const reportPath = path.join(
      backend.tmpDir,
      ".kandev",
      "plugins",
      PLUGIN_ID,
      "data",
      "automation-reports.jsonl",
    );
    const findingsPath = path.join(
      backend.tmpDir,
      ".kandev",
      "plugins",
      PLUGIN_ID,
      "data",
      "automation-findings.jsonl",
    );
    const triggered = await apiClient.triggerAutomationManual(automation.id);
    expect(triggered.skipped).not.toBe(true);
    await expect
      .poll(() => (fs.existsSync(findingsPath) ? fs.readFileSync(findingsPath, "utf8") : ""), {
        timeout: 120_000,
      })
      .toContain('"surface":"automation"');
    await expect
      .poll(() => (fs.existsSync(reportPath) ? fs.readFileSync(reportPath, "utf8") : ""), {
        timeout: 120_000,
      })
      .toContain("nightly findings");

    // A configuration change invalidates the approval, even though the
    // installed version and selected tool names did not change.
    await testPage.goto(`/settings/plugins/${PLUGIN_ID}`);
    const greeting = testPage.getByTestId("plugin-config-field-greeting").locator("input");
    await greeting.fill("changed during review");
    await saveChanges(testPage);
    await expect(greeting).toHaveValue("changed during review", { timeout: 15_000 });

    await testPage.goto(
      `/settings/workspaces/${seedData.workspaceId}/automations/${automation.id}`,
    );
    await expect(testPage.getByTestId("automation-plugin-tools-review")).toContainText(
      "Review required",
      { timeout: 15_000 },
    );
    await expect(
      testPage.getByTestId(`plugin-tool-${PLUGIN_ID}-list_findings`).getByRole("checkbox"),
    ).not.toBeChecked();
    await expect(
      testPage.getByTestId(`plugin-tool-${PLUGIN_ID}-complete_report`).getByRole("checkbox"),
    ).not.toBeChecked();

    // Explicit read-only reselection restores only that tool.
    await testPage.getByTestId(`plugin-tool-${PLUGIN_ID}-list_findings`).click();
    await saveChanges(testPage);
    await testPage.reload();
    await expect(
      testPage.getByTestId(`plugin-tool-${PLUGIN_ID}-list_findings`).getByRole("checkbox"),
    ).toBeChecked();
    await expect(
      testPage.getByTestId(`plugin-tool-${PLUGIN_ID}-complete_report`).getByRole("checkbox"),
    ).not.toBeChecked();
  });
});
