import { watchWs } from "../../helpers/causal-waits";
import { test, expect } from "../../fixtures/test-base";
import {
  OPTIONAL_AUTOMATION_PACKAGES,
  saveAutomationMCP,
  verifyAutomationMCPRecovery,
} from "../../helpers/automation-mcp";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

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
    await row.tap();
  }
  await section.evaluate((element) => element.scrollIntoView({ block: "start" }));
  await prCapture.screenshot("automation-mcp-phone", {
    caption: "Automation MCP selection on a phone",
  });
  await saveAutomationMCP(testPage, watcher);
  await testPage.reload();
  for (const id of OPTIONAL_AUTOMATION_PACKAGES)
    await expect(
      section.getByTestId(`automation-mcp-package-${id}`).getByRole("checkbox"),
    ).toBeChecked();
  const diagnostics = section
    .getByTestId("automation-mcp-package-diagnostics")
    .getByRole("checkbox");
  await diagnostics.tap();
  await diagnostics.focus();
  for (const width of [320, 393, 767, 768]) {
    await expect(diagnostics).not.toBeChecked();
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
  await expect(diagnostics).toBeChecked();
});

test.describe("Narrow fine pointer", () => {
  test.use({ viewport: { width: 393, height: 851 }, isMobile: false, hasTouch: false });
  test("keeps phone-sized package controls and keyboard access", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
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
    const checkbox = row.getByRole("checkbox");
    await checkbox.focus();
    await testPage.keyboard.press("Space");
    await expect(checkbox).toBeChecked();
    await saveAutomationMCP(testPage, watcher);
    await assertNoDocumentHorizontalOverflow(testPage, "automation MCP with a narrow fine pointer");
  });
});

test(
  "removes blocked and inherited direct MCP choices without losing inheritance",
  verifyAutomationMCPRecovery,
);
