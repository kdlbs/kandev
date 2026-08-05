import { test, expect } from "../../fixtures/test-base";

const APPEARANCE_URL = "/settings/general/appearance";

test.describe("mobile i18n language switcher", () => {
  test("switches to Chinese, persists through reload, and restores English", async ({
    testPage,
  }) => {
    await testPage.goto(APPEARANCE_URL);

    const select = testPage.getByLabel("Display language");
    await expect(select).toBeVisible({ timeout: 10_000 });
    await select.tap();
    await testPage.getByRole("listbox").getByRole("option", { name: "简体中文" }).tap();

    await expect(testPage.locator("html")).toHaveAttribute("lang", "zh-cn", { timeout: 10_000 });
    await expect(testPage.getByLabel("显示语言")).toBeVisible({ timeout: 10_000 });
    await expect
      .poll(async () => {
        const cookies = await testPage.context().cookies();
        return cookies.find((cookie) => cookie.name === "kandev_locale")?.value;
      })
      .toBe("zh-cn");

    await testPage.reload();
    await expect(testPage.locator("html")).toHaveAttribute("lang", "zh-cn", { timeout: 10_000 });
    await expect(testPage.getByLabel("显示语言")).toBeVisible({ timeout: 10_000 });

    const selectAfter = testPage.getByLabel("显示语言");
    await selectAfter.tap();
    await testPage.getByRole("listbox").getByRole("option", { name: "English" }).tap();
    await expect(testPage.locator("html")).toHaveAttribute("lang", "en", { timeout: 10_000 });
    await expect(testPage.getByLabel("Display language")).toBeVisible({ timeout: 10_000 });
  });
});
