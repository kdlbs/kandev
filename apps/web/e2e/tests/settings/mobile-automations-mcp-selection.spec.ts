import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { watchWs } from "../../helpers/causal-waits";
import { test, expect } from "../../fixtures/test-base";
import {
  OPTIONAL_AUTOMATION_PACKAGES,
  chooseAutomationPackage,
  verifyAutomationMCPDeleteWarning,
  saveAutomationMCP,
  verifyAutomationMCPRecovery,
  verifyAutomationMCPRepositoryInheritance,
} from "../../helpers/automation-mcp";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

// @covers AC-OFFICE-AUTOMATION-MCP-001.2, AC-OFFICE-AUTOMATION-MCP-004.4
test("matches MCP inheritance to the saved repository for a loaded automation", async ({
  testPage,
  apiClient,
  seedData,
  backend,
  prCapture,
}) => {
  await verifyAutomationMCPRepositoryInheritance({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
    touch: true,
  });
});

// @covers AC-OFFICE-AUTOMATION-MCP-004.4, AC-OFFICE-AUTOMATION-MCP-004.6, AC-OFFICE-AUTOMATION-MCP-004.8, AC-OFFICE-AUTOMATION-MCP-004.9, AC-OFFICE-AUTOMATION-MCP-004.10
test("selects and saves every optional package on a phone with reachable controls", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const automation = await apiClient.seedAutomation({
    workspaceId: seedData.workspaceId,
    name: "Phone MCP selection",
    repositoryMode: "none",
  });
  const watcher = watchWs(testPage);
  await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/automations/${automation.id}`);
  const section = testPage.getByTestId("automation-mcp-access");
  for (const id of OPTIONAL_AUTOMATION_PACKAGES) {
    const row = section.getByTestId(`automation-mcp-package-${id}`);
    await expect(row).toBeVisible();
    expect((await row.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await chooseAutomationPackage(testPage, id, true, true);
  }
  await section.evaluate((element) => element.scrollIntoView({ block: "start" }));
  if (prCapture.capturing) await waitForFiniteAnimations(testPage.locator("body"));
  await prCapture.screenshot("automation-mcp-phone", {
    caption: "Automation MCP selection on a phone",
  });
  await section.getByTestId("automation-mcp-package-configuration").tap();
  const detail = section.getByTestId("automation-mcp-package-detail");
  expect((await detail.getByRole("combobox").boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await detail.evaluate((element) => element.scrollIntoView({ block: "start" }));
  if (prCapture.capturing) await waitForFiniteAnimations(testPage.locator("body"));
  await prCapture.screenshot("automation-mcp-package-detail-phone", {
    caption: "Automation package detail with touch-sized controls",
  });
  await detail.getByRole("button", { name: "Back to packages", exact: true }).tap();
  await saveAutomationMCP(testPage, watcher);
  await testPage.reload();
  for (const id of OPTIONAL_AUTOMATION_PACKAGES)
    await expect(
      section.getByTestId(`automation-mcp-package-${id}`).locator('[data-slot="badge"]'),
    ).toHaveText("On");
  const diagnostics = section.getByTestId("automation-mcp-package-diagnostics");
  await chooseAutomationPackage(testPage, "diagnostics", false, true);
  for (const width of [320, 393, 767, 768]) {
    await expect(diagnostics.locator('[data-slot="badge"]')).toHaveText("Off");
    await testPage.setViewportSize({ width, height: 851 });
    await expect(diagnostics).toBeFocused();
    await assertNoDocumentHorizontalOverflow(testPage, `automation MCP at ${width}px`);
    expect(
      (await section.getByTestId("automation-mcp-package-configuration").boundingBox())!.height,
    ).toBeGreaterThanOrEqual(44);
  }
  await testPage
    .getByTestId("settings-floating-save")
    .getByRole("button", { name: "Reset", exact: true })
    .tap();
  await expect(diagnostics.locator('[data-slot="badge"]')).toHaveText("On");
});

test.describe("Narrow fine pointer", () => {
  test("keeps phone-sized package controls and keyboard access", async ({
    desktopTestPage: testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 393, height: 851 });
    expect(await testPage.evaluate(() => matchMedia("(pointer: fine)").matches)).toBe(true);
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Fine pointer MCP choices",
      repositoryMode: "none",
    });
    const watcher = watchWs(testPage);
    await testPage.goto(
      `/settings/workspaces/${seedData.workspaceId}/automations/${automation.id}`,
    );
    const row = testPage.getByTestId("automation-mcp-package-configuration");
    await expect(row).toBeVisible();
    expect((await row.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await row.focus();
    await testPage.keyboard.press("Enter");
    const detail = testPage.getByTestId("automation-mcp-package-detail");
    const back = detail.getByRole("button", { name: "Back to packages", exact: true });
    await expect(back).toBeFocused();
    expect((await back.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await testPage.keyboard.press("Tab");
    const choice = detail.getByRole("combobox");
    await expect(choice).toBeFocused();
    await expect(choice).toHaveAccessibleName("Configuration MCP access");
    expect((await choice.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await testPage.keyboard.press("Enter");
    await testPage.getByRole("option", { name: "On", exact: true }).click();
    await detail.getByRole("button", { name: /registered tools/ }).click();
    await expect(detail.getByText("list_shared_prompts_kandev", { exact: true })).toBeVisible();
    await back.click();
    await expect(row).toBeFocused();
    await expect(row.locator('[data-slot="badge"]')).toHaveText("On");
    await saveAutomationMCP(testPage, watcher);
    await assertNoDocumentHorizontalOverflow(testPage, "automation MCP with a narrow fine pointer");
  });
});

test(
  "removes blocked and inherited direct MCP choices without losing inheritance",
  verifyAutomationMCPRecovery,
);

test("warns before deleting a server selected by an automation", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  await verifyAutomationMCPDeleteWarning({ testPage, apiClient, seedData, prCapture });
});
