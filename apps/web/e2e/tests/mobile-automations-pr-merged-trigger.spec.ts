import { test, expect } from "../fixtures/test-base";
import { AutomationsPage } from "../pages/automations-page";

test.describe("Pull request merged automation on mobile", () => {
  test("edits the trigger with touch and persists the configuration", async ({
    testPage,
    seedData,
  }) => {
    const automationName = "Mobile PR merged trigger";
    const automations = new AutomationsPage(testPage, seedData.workspaceId);

    await automations.gotoNew();
    await automations.nameInput.fill(automationName);

    await automations.selectWorkflow("E2E Workflow");

    await automations.addConditionButton.tap();
    await testPage.getByRole("option", { name: /Pull request merged/i }).tap();

    const triggerSummary = testPage.getByRole("button", { name: /PR merged/i }).first();
    await triggerSummary.tap();

    const allReposSwitch = testPage.getByRole("switch", {
      name: /All repositories allowed/i,
    });
    await expect(allReposSwitch).toBeChecked();
    await allReposSwitch.tap();
    await expect(allReposSwitch).not.toBeChecked();

    const branchInput = testPage.getByLabel(/Base branches/i);
    await expect(branchInput).toHaveAttribute("id", "github-pr-merged-base-branches");
    await branchInput.fill("main, release/*");
    await branchInput.blur();

    await expect(testPage.getByText(/No repositories selected/i)).toBeVisible();
    await expect(automations.saveButton).toBeEnabled({ timeout: 5_000 });

    const settingsScroller = testPage.getByTestId("settings-scroll-container");
    await branchInput.scrollIntoViewIfNeeded();
    const viewport = testPage.viewportSize();
    const branchBox = await branchInput.boundingBox();
    expect(viewport).not.toBeNull();
    expect(branchBox).not.toBeNull();
    expect(branchBox!.x).toBeGreaterThanOrEqual(0);
    expect(branchBox!.x + branchBox!.width).toBeLessThanOrEqual(viewport!.width);
    await expect(settingsScroller).toHaveCSS("overflow-x", /hidden|auto|scroll/);

    await automations.saveButton.tap();
    await expect(testPage).toHaveURL(/automations$/, { timeout: 15_000 });
    await expect(automations.table).toBeVisible({ timeout: 10_000 });

    await automations.table.getByText(automationName, { exact: true }).tap();
    await expect(testPage).toHaveURL(/automations\/[a-f0-9-]+$/, { timeout: 10_000 });
    await expect(automations.editor).toBeVisible();

    await testPage
      .getByRole("button", { name: /PR merged/i })
      .first()
      .tap();
    await expect(
      testPage.getByRole("switch", { name: /All repositories allowed/i }),
    ).not.toBeChecked();
    await expect(testPage.getByLabel(/Base branches/i)).toHaveValue("main, release/*");
    await expect(testPage.getByText(/No repositories selected/i)).toBeVisible();

    const overflow = await testPage.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth,
    }));
    expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth);

    await automations.deleteButton.tap();
    await expect(automations.deleteConfirmation).toBeVisible();
    await expect(automations.deleteConfirmation).toContainText(
      "This will permanently delete Mobile PR merged trigger. This action cannot be undone.",
    );
    await automations.deleteConfirmButton.tap();
    await expect(testPage).toHaveURL(/automations$/, { timeout: 10_000 });
    await expect(testPage.getByText(automationName, { exact: true })).not.toBeVisible();
  });
});
