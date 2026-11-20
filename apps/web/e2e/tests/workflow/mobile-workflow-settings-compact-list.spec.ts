import { test, expect } from "../../fixtures/test-base";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

test.describe("Workflow settings compact list on mobile", () => {
  test("uses touch-sized two-line rows that expand by tap and keeps the view choice", async ({
    testPage,
    seedData,
  }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId, { listView: null });

    const card = page.workflowCard(seedData.workflowId);
    const row = page.compactRow(seedData.workflowId);
    const handle = page.dragHandle(seedData.workflowId);
    await expect(row).toBeVisible();
    await expect(handle).toBeVisible();
    expect(await page.listViewToggleGapBelowDivider()).toBeGreaterThanOrEqual(12);

    const [rowBox, handleBox, cardBox] = await Promise.all([
      row.boundingBox(),
      handle.boundingBox(),
      card.boundingBox(),
    ]);
    expect(rowBox!.height).toBeGreaterThanOrEqual(44);
    expect(handleBox!.height).toBeGreaterThanOrEqual(44);
    expect(handleBox!.width).toBeGreaterThanOrEqual(44);
    expect(handleBox!.x + handleBox!.width).toBeLessThanOrEqual(cardBox!.x + cardBox!.width);

    // Two lines: the step list sits below the workflow name.
    const nameBox = await row.getByText("E2E Workflow", { exact: true }).boundingBox();
    const stepBox = await row.getByTestId("workflow-compact-step").first().boundingBox();
    expect(stepBox!.y).toBeGreaterThanOrEqual(nameBox!.y + nameBox!.height - 1);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
    ).toBe(false);

    await row.tap();
    await expect(card).toHaveAttribute("data-expanded", "true");
    await expect(card.locator("input").first()).toBeVisible();
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
    ).toBe(false);
    await row.tap();
    await expect(card).toHaveAttribute("data-expanded", "false");

    const detailed = page.listViewOption("detailed");
    expect((await detailed.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await detailed.tap();
    await expect(row).toHaveCount(0);
    await testPage.reload();
    await expect(page.listViewOption("detailed")).toHaveAttribute("data-state", "on");
    await expect(page.compactRow(seedData.workflowId)).toHaveCount(0);
  });
});
