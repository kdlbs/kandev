import { expect, test } from "../../fixtures/test-base";
import {
  assertNoDocumentHorizontalOverflow,
  assertNoDescendantOverflowsRight,
} from "../../helpers/layout-assertions";
import { expandDisplaySettingsGroup } from "../../helpers/display-settings";
import { expectTouchControl } from "../../helpers/control-sizing";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

const PRIORITY_TOKENS = ["critical", "high", "medium", "low"] as const;

async function openMobileMenu(page: import("@playwright/test").Page) {
  await page.getByRole("button", { name: "Open menu" }).tap();
  const card = page.getByTestId("mobile-home-menu-card");
  await card.waitFor({ state: "visible" });
  return card;
}

async function closeMobileMenu(page: import("@playwright/test").Page) {
  const card = page.getByTestId("mobile-home-menu-card");
  await expect(async () => {
    if ((await card.count()) > 0) await page.keyboard.press("Escape");
    await expect(card).toHaveCount(0, { timeout: 1_000 });
  }).toPass({ timeout: 15_000 });
}

test.describe("Mobile display settings groups", () => {
  test.afterEach(async ({ apiClient, seedData }) => {
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      kanban_sort: "created_desc",
      kanban_priority_filter_tokens: [],
    });
  });

  test("uses independent collapsed groups with touch sized controls and one drawer scroll owner", async ({
    testPage,
  }) => {
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    const card = await openMobileMenu(testPage);

    for (const group of ["filters", "sort", "preview"] as const) {
      await expect(card.getByTestId(`mobile-display-settings-${group}-toggle`)).toHaveAttribute(
        "aria-expanded",
        "false",
      );
    }
    await expect(card.getByTestId("mobile-board-sort")).toHaveCount(0);

    await expandDisplaySettingsGroup(testPage, "filters", "mobile");
    await expandDisplaySettingsGroup(testPage, "sort", "mobile");
    await expandDisplaySettingsGroup(testPage, "preview", "mobile");

    await expect(card.getByTestId("mobile-board-sort")).toBeVisible();
    await expect(card.getByTestId("mobile-display-preview-toggle")).toBeVisible();
    for (const token of PRIORITY_TOKENS) {
      const option = card.getByTestId(`mobile-priority-filter-option-${token}`);
      await expect(option).toBeVisible();
      await expectTouchControl(option.locator("xpath=.."));
    }
    await expectTouchControl(card.getByTestId("mobile-display-settings-filters-toggle"));
    await expectTouchControl(card.getByTestId("mobile-display-settings-sort-toggle"));
    await expectTouchControl(card.getByTestId("mobile-display-settings-preview-toggle"));
    await expectTouchControl(card.getByTestId("mobile-board-sort"));
    await expectTouchControl(card.getByTestId("mobile-display-preview-toggle").locator("xpath=.."));

    const scroll = testPage.getByTestId("mobile-home-menu-scroll");
    await expect(scroll).toHaveCSS("overflow-y", "auto");
    await expect
      .poll(() => scroll.evaluate((element) => element.scrollHeight >= element.clientHeight))
      .toBe(true);
    await assertNoDescendantOverflowsRight(card, "mobile display settings drawer");
    await assertNoDocumentHorizontalOverflow(testPage, "mobile display settings");

    await closeMobileMenu(testPage);
    const reopened = await openMobileMenu(testPage);
    await expect(reopened.getByTestId("mobile-display-settings-filters-toggle")).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    await expect(reopened.getByTestId("mobile-board-sort")).toHaveCount(0);
  });

  test("names the mobile workflow and repository filter controls", async ({ testPage }) => {
    await testPage.goto("/tasks");
    const card = await openMobileMenu(testPage);
    await expandDisplaySettingsGroup(testPage, "filters", "mobile");

    await expect(card.getByTestId("mobile-display-workflow-filter")).toHaveAccessibleName(
      "Workflow",
    );
    await expect(card.getByTestId("mobile-display-repository-filter")).toHaveAccessibleName(
      "Repository",
    );
  });

  test("keeps persisted board values visible after closing and reopening the drawer", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      kanban_sort: "priority_desc",
      kanban_priority_filter_tokens: ["critical"],
    });
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await openMobileMenu(testPage);
    await expect(testPage.getByTestId("mobile-display-settings-filters")).toContainText("Critical");
    await expect(testPage.getByTestId("mobile-display-settings-sort")).toContainText("Priority");
    await expandDisplaySettingsGroup(testPage, "filters", "mobile");
    await expect(testPage.getByTestId("mobile-priority-filter-option-critical")).toHaveAttribute(
      "data-state",
      "checked",
    );
    await closeMobileMenu(testPage);

    await openMobileMenu(testPage);
    await expect(testPage.getByTestId("mobile-display-settings-filters-toggle")).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    await expandDisplaySettingsGroup(testPage, "filters", "mobile");
    await expect(testPage.getByTestId("mobile-priority-filter-option-critical")).toHaveAttribute(
      "data-state",
      "checked",
    );
  });

  test("switches from the phone drawer to the tablet sheet at the 768px boundary", async ({
    testPage,
  }) => {
    await testPage.setViewportSize({ width: 767, height: 700 });
    await testPage.goto("/");
    await testPage.getByTestId("kanban-board").waitFor({ state: "visible" });
    await openMobileMenu(testPage);
    await closeMobileMenu(testPage);

    await testPage.setViewportSize({ width: 768, height: 700 });
    await testPage.goto("/");
    await testPage.getByTestId("kanban-board").waitFor({ state: "visible" });
    await testPage.getByRole("button", { name: "Open menu" }).tap();
    await expect(testPage.getByRole("dialog", { name: "Menu" })).toBeVisible();
    await expect(testPage.getByTestId("mobile-home-menu-card")).toHaveCount(0);
    await testPage.keyboard.press("Escape");
  });
});
