import type { Locator, Page } from "@playwright/test";
import { test, expect } from "../fixtures/test-base";
import { AutomationsPage } from "../pages/automations-page";

/** Swipe up on a scroll container — the touch equivalent of wheel-down at scroll bottom. */
async function swipeUpOnElement(page: Page, element: Locator): Promise<void> {
  const box = await element.boundingBox();
  if (!box) throw new Error("scroll container has no bounding box");

  const cdp = await page.context().newCDPSession(page);
  const centerX = box.x + box.width / 2;
  const startY = box.y + box.height - 20;
  const endY = box.y + 20;

  await cdp.send("Input.dispatchTouchEvent", {
    type: "touchStart",
    touchPoints: [{ x: centerX, y: startY }],
  });
  for (let i = 1; i <= 8; i++) {
    const y = startY + ((endY - startY) * i) / 8;
    await cdp.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [{ x: centerX, y }],
    });
  }
  await cdp.send("Input.dispatchTouchEvent", {
    type: "touchEnd",
    touchPoints: [],
  });
}

test.describe("Automations settings on mobile", () => {
  test("keeps continuity choices visible and contained on a phone", async ({
    testPage,
    seedData,
  }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.gotoNew();

    const workflowSelectorBox = await automations.workflowSelector.boundingBox();
    expect(workflowSelectorBox?.height).toBeGreaterThanOrEqual(44);

    // @covers AC-OFFICE-AUTOMATIONS-SETTINGS-002.3, AC-OFFICE-AUTOMATION-CONTINUITY-001.6
    // Phones stack the options so each label stays readable and each target
    // stays touch-sized.
    const taskChoice = testPage.getByRole("radiogroup", { name: "Task for each run" });
    await expect(taskChoice).toBeVisible();
    const newTask = taskChoice.getByRole("radio", { name: "New task each run" });
    const reuse = taskChoice.getByRole("radio", { name: "One shared task" });
    const newTaskBox = await newTask.boundingBox();
    const reuseBox = await reuse.boundingBox();
    expect(newTaskBox?.height).toBeGreaterThanOrEqual(44);
    expect(reuseBox?.y ?? 0).toBeGreaterThan(newTaskBox?.y ?? 0);
    await expect(testPage.locator("#automation-continuation-description")).toHaveText(
      "Each run starts with its own conversation and files. Runs can overlap.",
    );

    await reuse.tap();
    await expect(reuse).toBeChecked();
    await expect(testPage.locator("#automation-continuation-description")).toHaveText(
      "Every run continues the same task and its files, whatever triggered it. Runs execute one at a time.",
    );
    await expect(testPage.getByRole("spinbutton", { name: "Max concurrent runs" })).toBeDisabled();

    // @covers AC-OFFICE-AUTOMATIONS-SETTINGS-002.5
    // The collapsed Tools row is a touch-sized target that opens the switches.
    const tools = testPage.getByTestId("automation-agent-tools-toggle");
    await tools.scrollIntoViewIfNeeded();
    expect((await tools.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    await expect(testPage.getByTestId("automation-mcp-access")).toHaveCount(0);
    await tools.tap();
    await expect(testPage.getByTestId("automation-mcp-access")).toBeVisible();

    const addRepository = testPage.getByRole("button", { name: "Add repository" });
    const addRepositoryBox = await addRepository.boundingBox();
    expect(addRepositoryBox?.height).toBeGreaterThanOrEqual(44);
    await addRepository.click();
    await expect(testPage.getByTestId("repo-chip-trigger")).toBeVisible();
    await expect(testPage.getByTestId("branch-chip-trigger")).toBeVisible();

    const overflow = await testPage.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth,
    }));
    expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth);
  });

  test("create page does not hand off bottom overscroll to the document", async ({
    testPage,
    seedData,
  }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.gotoNew();

    const settingsScroller = testPage.getByTestId("settings-scroll-container");
    await expect(settingsScroller).toBeVisible();
    await expect(settingsScroller).toHaveCSS("overscroll-behavior-y", "contain");

    await settingsScroller.evaluate((el) => {
      el.scrollTop = el.scrollHeight;
    });
    await swipeUpOnElement(testPage, settingsScroller);

    await expect.poll(() => testPage.evaluate(() => window.scrollY), { timeout: 5_000 }).toBe(0);
  });

  test("delete all in a status view removes only that view's runs on mobile", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    // The delete-all control now lives in the table header's rightmost cell;
    // on a phone viewport that position must stay reachable and scoped to
    // the active status filter.
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Mobile Filtered Delete",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
    });
    await apiClient.seedAutomationRun(automation.id, "skipped");
    await apiClient.seedAutomationRun(automation.id, "skipped");
    await apiClient.seedAutomationRun(automation.id, "succeeded");

    await testPage.goto(
      `/settings/workspaces/${seedData.workspaceId}/automations/${automation.id}`,
    );
    await testPage.getByTestId("automation-editor").waitFor({ state: "visible", timeout: 15_000 });

    const scrollContainer = testPage.getByTestId("settings-scroll-container");
    await scrollContainer.evaluate((el) => (el.scrollTop = el.scrollHeight));

    const recentRunsButton = testPage.locator("button", { hasText: /Recent Runs/ });
    await recentRunsButton.waitFor({ state: "visible", timeout: 10_000 });
    await recentRunsButton.click();

    const tbody = testPage.locator("table tbody");
    await tbody.waitFor({ state: "visible", timeout: 5_000 });
    await expect(tbody.locator("tr")).toHaveCount(3, { timeout: 10_000 });

    // Filter to Skipped, then delete all from the table header.
    await testPage.getByTestId("run-filter-skipped").click();
    await expect(tbody.locator("tr")).toHaveCount(2, { timeout: 5_000 });

    const deleteAll = testPage.locator("table thead").getByTestId("delete-all-runs");
    await expect(deleteAll).toBeVisible();
    await deleteAll.click();
    await expect(
      testPage.getByText(/permanently remove the Skipped runs shown in this view/),
    ).toBeVisible();
    await testPage.getByTestId("delete-all-runs-confirm").click();

    // Only the skipped runs are gone; the succeeded run survives.
    await expect(tbody.locator("tr")).toHaveCount(1, { timeout: 5_000 });
    await testPage.getByTestId("run-filter-all").click();
    await expect(tbody.locator("tr")).toHaveCount(1, { timeout: 5_000 });
  });
});
