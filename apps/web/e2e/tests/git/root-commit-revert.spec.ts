import { test, expect } from "../../fixtures/test-base";
import { openHistoryRegression } from "./changes-history-regression-helpers";
import { expectMenu, longPressCommit, ROOT_ROW, setParent } from "./root-commit-revert-helpers";

test.describe("Root commit revert availability", () => {
  test.describe.configure({ timeout: 120_000 });

  // @covers AC-WORKSPACES-ROOT-REVERT-001.1 AC-WORKSPACES-ROOT-REVERT-001.2
  test("desktop context menu and hover follow known ancestry", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await openHistoryRegression(testPage, apiClient, seedData, false);
    await setParent(testPage, "");
    const row = testPage.getByTestId(ROOT_ROW);
    await row.hover();
    await expect(row.getByRole("button", { name: "Revert commit" })).toHaveCount(0);
    await expect(row.getByRole("button", { name: "Amend commit message" })).toBeVisible();
    await row.click({ button: "right" });
    await expectMenu(testPage, false, () =>
      prCapture.screenshot("root-commit-desktop", { caption: "Root commit actions on desktop" }),
    );
    await setParent(testPage, "parent-sha");
    await row.hover();
    await expect(row.getByRole("button", { name: "Revert commit" })).toBeVisible();
    await row.click({ button: "right" });
    await expectMenu(testPage, true);
  });

  test("coarse desktop long-press follows known ancestry", async ({
    coarseDesktopTestPage: page,
    apiClient,
    seedData,
  }) => {
    await openHistoryRegression(page, apiClient, seedData, false);
    await setParent(page, "");
    await longPressCommit(page);
    await expectMenu(page, false);
    await setParent(page, "parent-sha");
    await longPressCommit(page);
    await expectMenu(page, true);
  });
});
