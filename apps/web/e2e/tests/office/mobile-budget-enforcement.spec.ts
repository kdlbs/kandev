import { test, expect } from "../../fixtures/office-fixture";
import { officeTopbarTitle } from "../../helpers/office-topbar";
import { waitForHttp } from "../../helpers/causal-waits";

test.describe("Office budget enforcement on mobile", () => {
  test("edits the default ceiling with reachable controls", async ({ testPage }) => {
    await testPage.goto("/office/workspace/costs");
    await expect(officeTopbarTitle(testPage)).toHaveText(/Costs/i, { timeout: 10_000 });
    const defaultLoad = waitForHttp(testPage, "GET", /\/budgets\/default$/);
    await testPage.getByRole("tab", { name: "Budgets" }).tap();
    await defaultLoad;

    const card = testPage.getByText("Default Ceiling").locator("..").locator("..");
    await expect(card.getByText(/\$\d+\.\d{2}/)).toBeVisible();

    const edit = card.getByRole("button", { name: "Edit default ceiling" });
    const editBox = await edit.boundingBox();
    expect(editBox?.width).toBeGreaterThanOrEqual(44);
    expect(editBox?.height).toBeGreaterThanOrEqual(44);
    await edit.tap();

    const input = card.getByRole("spinbutton");
    await expect(input).toHaveAccessibleName("Default Ceiling");
    await input.fill("75.50");

    const save = card.getByRole("button", { name: "Save" });
    const cancel = card.getByRole("button", { name: "Cancel" });
    for (const button of [save, cancel]) {
      const box = await button.boundingBox();
      expect(box?.width).toBeGreaterThanOrEqual(44);
      expect(box?.height).toBeGreaterThanOrEqual(44);
    }

    const saved = waitForHttp(testPage, "PUT", /\/budgets\/default$/, {
      predicate: (response) => response.status() === 200,
    });
    await save.tap();
    await saved;
    await expect(card.getByText("$75.50")).toBeVisible();

    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );
  });
});
