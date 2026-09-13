import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { expandDisplaySettingsGroup } from "../../helpers/display-settings";

async function openDisplaySettings(page: import("@playwright/test").Page) {
  const trigger = page.getByTestId("display-button");
  await trigger.focus();
  await page.keyboard.press("Enter");
  await expect(page.getByTestId("display-settings-content")).toBeVisible();
  return trigger;
}

test.describe("Display settings groups", () => {
  test("starts collapsed, expands independently, and resets after dismiss", async ({
    testPage,
  }) => {
    await testPage.goto("/");
    const trigger = await openDisplaySettings(testPage);

    for (const group of ["filters", "sort", "preview"] as const) {
      await expect(testPage.getByTestId(`display-settings-${group}-toggle`)).toHaveAttribute(
        "aria-expanded",
        "false",
      );
    }
    await expect(testPage.getByTestId("display-workflow-filter")).toHaveCount(0);
    await expect(testPage.getByTestId("display-board-sort")).toHaveCount(0);

    await expandDisplaySettingsGroup(testPage, "filters");
    await expect(testPage.getByTestId("display-workflow-filter")).toBeVisible();
    await expect(testPage.getByTestId("display-settings-sort-toggle")).toHaveAttribute(
      "aria-expanded",
      "false",
    );

    await expandDisplaySettingsGroup(testPage, "sort");
    await expect(testPage.getByTestId("display-board-sort")).toBeVisible();
    await expect(testPage.getByTestId("display-settings-filters-toggle")).toHaveAttribute(
      "aria-expanded",
      "true",
    );

    await testPage.keyboard.press("Escape");
    await expect(testPage.getByTestId("display-settings-content")).toHaveCount(0);
    await expect(trigger).toBeFocused();

    await trigger.click();
    await expect(testPage.getByTestId("display-settings-filters-toggle")).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    await expect(testPage.getByTestId("display-settings-sort-toggle")).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    await expect(testPage.getByTestId("display-workflow-filter")).toHaveCount(0);
  });

  test("supports keyboard activation and keeps the compact panel scrollable", async ({
    testPage,
  }) => {
    await testPage.setViewportSize({ width: 820, height: 360 });
    await testPage.goto("/");
    const trigger = await openDisplaySettings(testPage);

    const filtersToggle = testPage.getByTestId("display-settings-filters-toggle");
    await expect(filtersToggle).toBeFocused();
    await testPage.keyboard.press("Enter");
    await expect(filtersToggle).toHaveAttribute("aria-expanded", "true");

    const workflowFilter = testPage.getByTestId("display-workflow-filter");
    const repositoryFilter = testPage.getByTestId("display-repository-filter");
    const criticalFilter = testPage.getByTestId("display-priority-filter-option-critical");
    await testPage.keyboard.press("Tab");
    await expect(workflowFilter).toBeFocused();
    await testPage.keyboard.press("Tab");
    await expect(repositoryFilter).toBeFocused();
    await testPage.keyboard.press("Tab");
    await expect(criticalFilter).toBeFocused();
    await testPage.keyboard.press("Space");
    await expect(criticalFilter).toHaveAttribute("data-state", "checked");

    for (const token of ["high", "medium", "low"] as const) {
      await testPage.keyboard.press("Tab");
      await expect(testPage.getByTestId(`display-priority-filter-option-${token}`)).toBeFocused();
    }
    await testPage.keyboard.press("Tab");

    const sortToggle = testPage.getByTestId("display-settings-sort-toggle");
    await expect(sortToggle).toBeFocused();
    await testPage.keyboard.press("Enter");
    await expect(sortToggle).toHaveAttribute("aria-expanded", "true");
    const boardSort = testPage.getByTestId("display-board-sort");
    await testPage.keyboard.press("Tab");
    await expect(boardSort).toBeFocused();
    await testPage.keyboard.press("p");
    await expect(boardSort).toContainText("Priority");

    const panel = testPage.getByTestId("display-settings-content");
    await expect
      .poll(() => panel.evaluate((element) => element.scrollHeight > element.clientHeight))
      .toBe(true);
    await assertNoDocumentHorizontalOverflow(testPage, "compact display settings");
    await testPage.keyboard.press("Escape");
    await expect(trigger).toBeFocused();
  });
});
